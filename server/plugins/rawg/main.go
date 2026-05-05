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
	"sort"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/pkg/titlematch"
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
	Index        int    `json:"index"`
	Title        string `json:"title"`
	Platform     string `json:"platform"`
	RootPath     string `json:"root_path"`
	GroupKind    string `json:"group_kind"`
	LookupIntent string `json:"lookup_intent,omitempty"`
}

type mediaItem struct {
	Type     string `json:"type"`
	URL      string `json:"url"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

type lookupResult struct {
	Index       int         `json:"index"`
	Title       string      `json:"title,omitempty"`
	Platform    string      `json:"platform,omitempty"`
	ExternalID  string      `json:"external_id"`
	URL         string      `json:"url,omitempty"`
	Description string      `json:"description,omitempty"`
	ReleaseDate string      `json:"release_date,omitempty"`
	Genres      []string    `json:"genres,omitempty"`
	Developer   string      `json:"developer,omitempty"`
	Publisher   string      `json:"publisher,omitempty"`
	Media       []mediaItem `json:"media,omitempty"`
	Rating      float64     `json:"rating,omitempty"`
	MaxPlayers  int         `json:"max_players,omitempty"`
}

// RAWG API types.

type rawgSearchResponse struct {
	Count   int        `json:"count"`
	Results []rawgGame `json:"results"`
}

type rawgGame struct {
	ID              int              `json:"id"`
	Slug            string           `json:"slug"`
	Name            string           `json:"name"`
	Platforms       []rawgPlatform   `json:"platforms"`
	Released        string           `json:"released"`
	BackgroundImage string           `json:"background_image"`
	Metacritic      int              `json:"metacritic"`
	Genres          []rawgNamed      `json:"genres"`
	Screenshots     []rawgScreenshot `json:"short_screenshots"`
}

type rawgPlatform struct {
	Platform rawgPlatformInfo `json:"platform"`
}

type rawgPlatformInfo struct {
	ID   int    `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

type rawgNamed struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type rawgScreenshot struct {
	Image string `json:"image"`
}

type rawgGameDetail struct {
	Description string      `json:"description_raw"`
	Developers  []rawgNamed `json:"developers"`
	Publishers  []rawgNamed `json:"publishers"`
}

// Config for credentials.

type rawgConfig struct {
	APIKey string `json:"api_key"`
}

var apiKey string

// Our platform → RAWG platform ID.
// NOTE: arcade has no dedicated RAWG platform ID; we skip the platform filter.
var platformMap = map[string]int{
	"windows_pc": 4,
	"ms_dos":     4,
	"gba":        24,
	"n64":        83,
	"ps1":        27,
	"ps2":        15,
	"ps3":        16,
	"psp":        17,
	"xbox_360":   14,
	"scummvm":    4,
}

const (
	rawgBaseURL = "https://api.rawg.io/api"
	configFile  = "config.json"
)

// Rate limiter: 5 requests per second (1 per 200ms).
var rateLimiter = time.NewTicker(200 * time.Millisecond)

// --- Title normalization ---

var (
	trailingParensRE = regexp.MustCompile(`[\s_]*\([^)]*\)\s*$`)
	setupPrefixRE    = regexp.MustCompile(`^setup[_\s]`)
	versionSuffixRE  = regexp.MustCompile(`[\s._]+v?\d+(\.\d+)+([\s._]+[a-z]{2,3})*\s*$`)
	nonAlphaNumRE    = regexp.MustCompile(`[^a-z0-9\s]+`)
	multiSpaceRE     = regexp.MustCompile(`\s{2,}`)
)

func normalizeTitle(s string) string {
	return titlematch.NormalizeLookupTitle(s)
}

func tokenize(s string) map[string]bool {
	return titlematch.TokenizeLookupTitle(s)
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

func scoreCandidate(normalizedQuery string, queryTokens map[string]bool, g *rawgGame) float64 {
	if normalizeTitle(g.Name) == normalizedQuery {
		return 1.0
	}
	return jaccardSimilarity(queryTokens, tokenize(g.Name))
}

// --- Config ---

func loadConfig() (*rawgConfig, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	var cfg rawgConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("config.json must contain api_key")
	}
	return &cfg, nil
}

// --- RAWG API ---

func rawgSearch(title string, platformID int, exact bool) ([]rawgGame, error) {
	<-rateLimiter.C

	params := url.Values{
		"key":            {apiKey},
		"search":         {title},
		"search_precise": {"true"},
		"page_size":      {"5"},
	}
	if platformID > 0 {
		params.Set("platforms", fmt.Sprintf("%d", platformID))
	}
	if exact {
		params.Set("search_exact", "true")
	}

	reqURL := rawgBaseURL + "/games?" + params.Encode()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("RAWG request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("unauthorized (invalid API key?)")
	}
	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("rate limited")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("RAWG API status %d: %s", resp.StatusCode, string(body))
	}

	var result rawgSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode RAWG response: %w", err)
	}
	return result.Results, nil
}

func rawgGameDetail_fetch(gameID int) (*rawgGameDetail, error) {
	<-rateLimiter.C

	reqURL := fmt.Sprintf("%s/games/%d?key=%s", rawgBaseURL, gameID, apiKey)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("RAWG detail request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("RAWG detail status %d", resp.StatusCode)
	}

	var detail rawgGameDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, fmt.Errorf("decode RAWG detail: %w", err)
	}
	return &detail, nil
}

func rawgStatusMessage(statusCode int) (string, bool) {
	switch statusCode {
	case http.StatusOK:
		return "", false
	case http.StatusUnauthorized:
		return "invalid API key", true
	case http.StatusTooManyRequests:
		return "RAWG rate limit reached", true
	case http.StatusForbidden:
		return "RAWG rejected the request (forbidden or quota/account restriction)", true
	default:
		return fmt.Sprintf("RAWG API returned status %d", statusCode), true
	}
}

// buildSearchQueries returns an ordered list of (platformID, exact) pairs to try.
type searchPass struct {
	platformID int
	exact      bool
}

func buildSearchQueries(platform string) []searchPass {
	rawgID := platformMap[platform]
	var passes []searchPass

	// Pass 1: search with platform filter (skip if no RAWG platform mapping).
	if rawgID > 0 {
		passes = append(passes, searchPass{platformID: rawgID, exact: false})
	}

	// Pass 2: search without platform filter.
	passes = append(passes, searchPass{platformID: 0, exact: false})

	// Pass 3: search_exact=true (no platform filter).
	passes = append(passes, searchPass{platformID: 0, exact: true})

	return passes
}

// --- Init ---

func handleInit() (any, *Error) {
	cfg, err := loadConfig()
	if err != nil {
		log.Printf("WARNING: %v — RAWG plugin will not be functional", err)
		return map[string]any{"status": "not_configured", "reason": err.Error()}, nil
	}

	apiKey = cfg.APIKey

	// Test the API key with a simple request.
	testParams := url.Values{
		"key":       {apiKey},
		"search":    {"test"},
		"page_size": {"1"},
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(rawgBaseURL + "/games?" + testParams.Encode())
	if err != nil {
		return nil, &Error{Code: "API_ERROR", Message: fmt.Sprintf("test request failed: %v", err)}
	}
	resp.Body.Close()
	if msg, failed := rawgStatusMessage(resp.StatusCode); failed {
		code := "API_ERROR"
		if resp.StatusCode == http.StatusUnauthorized {
			code = "AUTH_FAILED"
		} else if resp.StatusCode == http.StatusTooManyRequests {
			code = "RATE_LIMITED"
		}
		return nil, &Error{Code: code, Message: msg}
	}

	log.Println("RAWG plugin initialized and authenticated")
	return map[string]any{"status": "ok"}, nil
}

// --- Lookup ---

func handleLookup(params lookupParams) (any, *Error) {
	if apiKey == "" {
		return map[string]any{"results": []any{}}, nil
	}

	var results []lookupResult
	for _, q := range params.Games {
		if q.LookupIntent == "manual_search" {
			matches, err := matchGamesForManualSearch(q)
			if err != nil {
				log.Printf("RAWG manual lookup error for %q: %v", q.Title, err)
				continue
			}
			results = append(results, matches...)
			continue
		}
		r, err := matchGame(q)
		if err != nil {
			log.Printf("RAWG lookup error for %q: %v", q.Title, err)
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
const manualMinMatchScore = 0.45
const maxManualResults = 10

func matchGame(q gameQuery) (*lookupResult, error) {
	cleanedTitle := normalizeTitle(q.Title)
	passes := buildSearchQueries(q.Platform)
	queryTokens := tokenize(q.Title)

	var overallBest *rawgGame
	overallBestScore := -1.0

	for _, pass := range passes {
		games, err := rawgSearch(cleanedTitle, pass.platformID, pass.exact)
		if err != nil {
			return nil, err
		}
		if len(games) == 0 {
			continue
		}

		for i := range games {
			score := scoreCandidate(cleanedTitle, queryTokens, &games[i])
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

	return buildResult(q, overallBest), nil
}

func matchGamesForManualSearch(q gameQuery) ([]lookupResult, error) {
	queryTokens := tokenize(q.Title)
	type candidate struct {
		game  rawgGame
		score float64
	}
	candidates := map[int]candidate{}
	for _, variant := range titlematch.LookupTitleVariants(q.Title) {
		cleanedTitle := normalizeTitle(variant)
		if cleanedTitle == "" {
			continue
		}
		for _, pass := range buildSearchQueries(q.Platform) {
			games, err := rawgSearch(cleanedTitle, pass.platformID, pass.exact)
			if err != nil {
				return nil, err
			}
			for i := range games {
				score := scoreCandidate(cleanedTitle, queryTokens, &games[i])
				if score < manualMinMatchScore {
					continue
				}
				current, ok := candidates[games[i].ID]
				if !ok || score > current.score {
					candidates[games[i].ID] = candidate{game: games[i], score: score}
				}
			}
		}
	}

	rawgID := platformMap[q.Platform]
	ranked := make([]candidate, 0, len(candidates))
	for _, candidate := range candidates {
		ranked = append(ranked, candidate)
	}
	sort.Slice(ranked, func(i, j int) bool {
		leftPlatform := rawgGamePlatformMatches(ranked[i].game, rawgID)
		rightPlatform := rawgGamePlatformMatches(ranked[j].game, rawgID)
		if leftPlatform != rightPlatform {
			return leftPlatform
		}
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].game.Name < ranked[j].game.Name
	})
	if len(ranked) > maxManualResults {
		ranked = ranked[:maxManualResults]
	}
	results := make([]lookupResult, 0, len(ranked))
	for _, item := range ranked {
		results = append(results, *buildResult(q, &item.game))
	}
	return results, nil
}

func buildResult(q gameQuery, overallBest *rawgGame) *lookupResult {
	rawgID := platformMap[q.Platform]
	r := &lookupResult{
		Index:      q.Index,
		Title:      overallBest.Name,
		ExternalID: fmt.Sprintf("%d", overallBest.ID),
		URL:        fmt.Sprintf("https://rawg.io/games/%s", overallBest.Slug),
	}
	if rawgID > 0 && rawgGamePlatformMatches(*overallBest, rawgID) {
		r.Platform = q.Platform
	}

	r.ReleaseDate = overallBest.Released
	if overallBest.BackgroundImage != "" {
		r.Media = append(r.Media, mediaItem{Type: "background", URL: overallBest.BackgroundImage})
	}
	if overallBest.Metacritic > 0 {
		r.Rating = float64(overallBest.Metacritic)
	}
	for _, g := range overallBest.Genres {
		r.Genres = append(r.Genres, g.Name)
	}
	for _, ss := range overallBest.Screenshots {
		if ss.Image != "" {
			r.Media = append(r.Media, mediaItem{Type: "screenshot", URL: ss.Image})
		}
	}

	detail, err := rawgGameDetail_fetch(overallBest.ID)
	if err != nil {
		log.Printf("RAWG detail fetch for %d: %v (continuing with search data)", overallBest.ID, err)
	} else {
		r.Description = detail.Description
		if len(detail.Developers) > 0 {
			r.Developer = detail.Developers[0].Name
		}
		if len(detail.Publishers) > 0 {
			r.Publisher = detail.Publishers[0].Name
		}
	}

	return r
}

func rawgGamePlatformMatches(game rawgGame, wanted int) bool {
	if wanted <= 0 {
		return false
	}
	for _, platform := range game.Platforms {
		if platform.Platform.ID == wanted {
			return true
		}
	}
	return false
}

// --- Main ---

func main() {
	log.SetOutput(os.Stderr)
	log.Println("RAWG metadata plugin started")

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
				"plugin_id":      "metadata-rawg",
				"plugin_version": "1.0.0",
				"capabilities":   []string{"metadata"},
			}

		case "plugin.check_config":
			var params struct {
				Config map[string]any `json:"config"`
			}
			if err := json.Unmarshal(req.Params, &params); err == nil {
				key, _ := params.Config["api_key"].(string)
				if key != "" {
					testParams := url.Values{
						"key":       {key},
						"search":    {"test"},
						"page_size": {"1"},
					}
					client := &http.Client{Timeout: 10 * time.Second}
					testResp, err := client.Get(rawgBaseURL + "/games?" + testParams.Encode())
					if err != nil {
						resp.Result = map[string]any{"status": "error", "message": err.Error()}
					} else {
						testResp.Body.Close()
						if testResp.StatusCode == 200 {
							resp.Result = map[string]any{"status": "ok"}
						} else if msg, failed := rawgStatusMessage(testResp.StatusCode); failed {
							resp.Result = map[string]any{"status": "error", "message": msg}
						} else {
							resp.Result = map[string]any{"status": "error", "message": "RAWG API returned an unexpected response"}
						}
					}
				} else {
					resp.Result = map[string]any{"status": "error", "message": "api_key required"}
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
