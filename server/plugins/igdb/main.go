package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// IPC protocol types.

type Request struct {
	ID     string          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type Response struct {
	ID     string `json:"id"`
	Result any    `json:"result,omitempty"`
	Error  *Error `json:"error,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Plugin request/response types.

type lookupParams struct {
	Games  []gameQuery    `json:"games"`
	Config map[string]any `json:"config"`
}

type gameQuery struct {
	Index     int    `json:"index"`
	Title     string `json:"title"`
	Platform  string `json:"platform"`
	RootPath  string `json:"root_path"`
	GroupKind string `json:"group_kind"`
}

type lookupResult struct {
	Index      int    `json:"index"`
	Title      string `json:"title,omitempty"`
	Platform   string `json:"platform,omitempty"`
	ExternalID string `json:"external_id"`
	URL        string `json:"url,omitempty"`
}

// IGDB API types.

type igdbGame struct {
	ID               int          `json:"id"`
	Name             string       `json:"name"`
	Slug             string       `json:"slug"`
	Summary          string       `json:"summary"`
	FirstReleaseDate int64        `json:"first_release_date"`
	Platforms        []int        `json:"platforms"`
	Genres           []igdbNamed  `json:"genres"`
	URL              string       `json:"url"`
}

type igdbNamed struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type twitchTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// Config for credentials.

type igdbConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// Auth state.

type authState struct {
	mu          sync.Mutex
	cfg         igdbConfig
	accessToken string
	expiresAt   time.Time
}

var auth authState

// Our platform → IGDB platform ID(s).
var platformMap = map[string][]int{
	"windows_pc": {6},
	"ms_dos":     {13},
	"arcade":     {52},
	"gba":        {24},
	"ps1":        {7},
	"ps2":        {8},
	"ps3":        {9},
	"psp":        {38},
	"xbox_360":   {36},
	"scummvm":    {13, 6},
}

const (
	twitchTokenURL = "https://id.twitch.tv/oauth2/token"
	igdbBaseURL    = "https://api.igdb.com/v4"
	configFile     = "config.json"
)

// Rate limiter: 4 requests per second.
var rateLimiter = time.NewTicker(250 * time.Millisecond)

// --- Title normalization (shared logic with LaunchBox) ---

var (
	versionSuffixRE = regexp.MustCompile(`[\s._]+v?\d+(\.\d+)+([\s._]+[a-z]{2,3})*\s*$`)
	nonAlphaNumRE   = regexp.MustCompile(`[^a-z0-9\s]+`)
	multiSpaceRE    = regexp.MustCompile(`\s{2,}`)
)

func normalizeTitle(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_ ", " ")
	s = versionSuffixRE.ReplaceAllString(s, "")
	s = nonAlphaNumRE.ReplaceAllString(s, " ")
	s = multiSpaceRE.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func tokenize(s string) map[string]bool {
	words := strings.Fields(normalizeTitle(s))
	tokens := make(map[string]bool, len(words))
	for _, w := range words {
		tokens[w] = true
	}
	return tokens
}

func jaccardSimilarity(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	intersection := 0
	for k := range a {
		if b[k] {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// --- Auth ---

func loadConfig() (*igdbConfig, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	var cfg igdbConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("config.json must contain client_id and client_secret")
	}
	return &cfg, nil
}

func authenticate(cfg *igdbConfig) (string, time.Time, error) {
	resp, err := http.PostForm(twitchTokenURL, url.Values{
		"client_id":     {cfg.ClientID},
		"client_secret": {cfg.ClientSecret},
		"grant_type":    {"client_credentials"},
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("token request: status %d: %s", resp.StatusCode, string(body))
	}

	var tok twitchTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return "", time.Time{}, fmt.Errorf("decode token: %w", err)
	}

	expiresAt := time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second).Add(-5 * time.Minute)
	return tok.AccessToken, expiresAt, nil
}

func (a *authState) getToken() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.accessToken != "" && time.Now().Before(a.expiresAt) {
		return a.accessToken, nil
	}

	token, expires, err := authenticate(&a.cfg)
	if err != nil {
		return "", err
	}
	a.accessToken = token
	a.expiresAt = expires
	log.Printf("authenticated with Twitch (expires %s)", expires.Format(time.RFC3339))
	return a.accessToken, nil
}

// --- Init ---

func handleInit() (any, *Error) {
	cfg, err := loadConfig()
	if err != nil {
		log.Printf("WARNING: %v — IGDB plugin will not be functional", err)
		return map[string]any{"status": "not_configured", "reason": err.Error()}, nil
	}

	auth.cfg = *cfg

	token, _, err := authenticate(cfg)
	if err != nil {
		return nil, &Error{Code: "AUTH_FAILED", Message: err.Error()}
	}

	auth.mu.Lock()
	auth.accessToken = token
	auth.expiresAt = time.Now().Add(55 * 24 * time.Hour)
	auth.mu.Unlock()

	log.Println("IGDB plugin initialized and authenticated")
	return map[string]any{"status": "ok"}, nil
}

// --- IGDB API ---

func igdbQuery(endpoint, query string) ([]byte, error) {
	token, err := auth.getToken()
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	<-rateLimiter.C

	req, err := http.NewRequest("POST", igdbBaseURL+"/"+endpoint, strings.NewReader(query))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-ID", auth.cfg.ClientID)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "text/plain")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 401 {
		auth.mu.Lock()
		auth.accessToken = ""
		auth.mu.Unlock()
		return nil, fmt.Errorf("unauthorized (token expired?)")
	}
	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("rate limited")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("IGDB API status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

const igdbFields = "name,slug,url,first_release_date,platforms,genres.name"

// buildSearchQueries returns an ordered list of IGDB queries to try,
// from most specific to least specific.
func buildSearchQueries(title string, igdbPlatformIDs []int) []string {
	escaped := strings.ReplaceAll(title, `"`, `\"`)
	var queries []string

	// Pass 1: search + platform filter.
	if len(igdbPlatformIDs) > 0 {
		queries = append(queries, fmt.Sprintf(
			`search "%s"; fields %s; where %s; limit 10;`,
			escaped, igdbFields, platformFilterExpr(igdbPlatformIDs),
		))
	}

	// Pass 2: search without platform filter.
	queries = append(queries, fmt.Sprintf(
		`search "%s"; fields %s; limit 10;`,
		escaped, igdbFields,
	))

	// Pass 3: name~ wildcard match.
	if len(igdbPlatformIDs) > 0 {
		queries = append(queries, fmt.Sprintf(
			`fields %s; where name ~ *"%s"* & %s; limit 10;`,
			igdbFields, escaped, platformFilterExpr(igdbPlatformIDs),
		))
	} else {
		queries = append(queries, fmt.Sprintf(
			`fields %s; where name ~ *"%s"*; limit 10;`,
			igdbFields, escaped,
		))
	}

	return queries
}

func runQuery(query string) ([]igdbGame, error) {
	data, err := igdbQuery("games", query)
	if err != nil {
		return nil, err
	}
	var games []igdbGame
	if err := json.Unmarshal(data, &games); err != nil {
		return nil, fmt.Errorf("decode IGDB response: %w", err)
	}
	return games, nil
}

func platformFilterExpr(ids []int) string {
	if len(ids) == 1 {
		return fmt.Sprintf("platforms = %d", ids[0])
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%d", id)
	}
	return fmt.Sprintf("platforms = (%s)", strings.Join(parts, ","))
}

func filterByPlatform(games []igdbGame, wanted []int) []igdbGame {
	var out []igdbGame
	for _, g := range games {
		if gamePlatformMatches(g.Platforms, wanted) {
			out = append(out, g)
		}
	}
	return out
}

func gamePlatformMatches(gamePlatforms []int, wanted []int) bool {
	for _, gp := range gamePlatforms {
		for _, wp := range wanted {
			if gp == wp {
				return true
			}
		}
	}
	return false
}

// --- Lookup ---

func handleLookup(params lookupParams) (any, *Error) {
	if auth.cfg.ClientID == "" {
		return map[string]any{"results": []any{}}, nil
	}

	var results []lookupResult
	for _, q := range params.Games {
		r, err := matchGame(q)
		if err != nil {
			log.Printf("IGDB lookup error for %q: %v", q.Title, err)
			continue
		}
		if r != nil {
			results = append(results, *r)
		}
	}

	return map[string]any{"results": results}, nil
}

const minMatchScore = 0.7
const goodMatchScore = 0.9

func matchGame(q gameQuery) (*lookupResult, error) {
	igdbPlatforms := platformMap[q.Platform]
	cleanedTitle := normalizeTitle(q.Title)
	queries := buildSearchQueries(cleanedTitle, igdbPlatforms)
	queryTokens := tokenize(q.Title)
	normalizedQuery := cleanedTitle

	var overallBest *igdbGame
	overallBestScore := -1.0

	for _, query := range queries {
		games, err := runQuery(query)
		if err != nil {
			return nil, err
		}
		if len(games) == 0 {
			continue
		}

		for i := range games {
			score := scoreCandidate(normalizedQuery, queryTokens, &games[i])
			if score > overallBestScore {
				overallBestScore = score
				overallBest = &games[i]
			}
		}

		if overallBestScore >= goodMatchScore {
			break
		}
	}

	if overallBest == nil || overallBestScore < minMatchScore {
		return nil, nil
	}

	igdbURL := overallBest.URL
	if igdbURL == "" {
		igdbURL = fmt.Sprintf("https://www.igdb.com/games/%s", overallBest.Slug)
	}

	return &lookupResult{
		Index:      q.Index,
		Title:      overallBest.Name,
		ExternalID: fmt.Sprintf("%d", overallBest.ID),
		URL:        igdbURL,
	}, nil
}

func scoreCandidate(normalizedQuery string, queryTokens map[string]bool, g *igdbGame) float64 {
	if normalizeTitle(g.Name) == normalizedQuery {
		return 1.0
	}
	return jaccardSimilarity(queryTokens, tokenize(g.Name))
}

// --- Main ---

func main() {
	log.SetOutput(os.Stderr)
	log.Println("IGDB metadata plugin started")

	for {
		var length uint32
		if err := binary.Read(os.Stdin, binary.BigEndian, &length); err != nil {
			if err == io.EOF {
				return
			}
			log.Fatalf("read length: %v", err)
		}

		payload := make([]byte, length)
		if _, err := io.ReadFull(os.Stdin, payload); err != nil {
			log.Fatalf("read payload: %v", err)
		}

		var req Request
		if err := json.Unmarshal(payload, &req); err != nil {
			log.Printf("unmarshal request: %v", err)
			continue
		}

		var resp Response
		resp.ID = req.ID

		switch req.Method {
		case "plugin.init":
			result, errObj := handleInit()
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}

		case "plugin.info":
			resp.Result = map[string]any{
				"plugin_id":      "metadata-igdb",
				"plugin_version": "1.0.0",
				"capabilities":   []string{"metadata"},
			}

		case "plugin.check_config":
			var params struct {
				Config map[string]any `json:"config"`
			}
			if err := json.Unmarshal(req.Params, &params); err == nil {
				cid, _ := params.Config["client_id"].(string)
				csec, _ := params.Config["client_secret"].(string)
				if cid != "" && csec != "" {
					_, _, err := authenticate(&igdbConfig{ClientID: cid, ClientSecret: csec})
					if err != nil {
						resp.Result = map[string]any{"status": "error", "message": err.Error()}
					} else {
						resp.Result = map[string]any{"status": "ok"}
					}
				} else {
					resp.Result = map[string]any{"status": "error", "message": "client_id and client_secret required"}
				}
			} else {
				resp.Result = map[string]any{"status": "ok"}
			}

		case "metadata.game.lookup":
			var params lookupParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				resp.Error = &Error{Code: "INVALID_PARAMS", Message: err.Error()}
			} else {
				result, errObj := handleLookup(params)
				if errObj != nil {
					resp.Error = errObj
				} else {
					resp.Result = result
				}
			}

		default:
			resp.Error = &Error{Code: "UNKNOWN_METHOD", Message: "unknown method: " + req.Method}
		}

		out, _ := json.Marshal(resp)
		binary.Write(os.Stdout, binary.BigEndian, uint32(len(out)))
		os.Stdout.Write(out)
	}
}
