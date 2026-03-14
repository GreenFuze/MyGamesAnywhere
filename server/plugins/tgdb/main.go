package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
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

// TheGamesDB API types.

type tgdbResponse struct {
	Code   int    `json:"code"`
	Status string `json:"status"`
	Data   struct {
		Count int        `json:"count"`
		Games []tgdbGame `json:"games"`
	} `json:"data"`
}

type tgdbGame struct {
	ID          int    `json:"id"`
	GameTitle   string `json:"game_title"`
	ReleaseDate string `json:"release_date"`
	Platform    int    `json:"platform"`
	Developers  []int  `json:"developers"`
}

// Config for credentials.

type tgdbConfig struct {
	APIKey string `json:"api_key"`
}

var cfg tgdbConfig

// Our platform → TheGamesDB platform ID.
var platformMap = map[string]int{
	"windows_pc": 1,
	"ms_dos":     1,
	"arcade":     23,
	"gba":        5,
	"ps1":        10,
	"ps2":        11,
	"ps3":        12,
	"psp":        13,
	"xbox_360":   15,
	"scummvm":    1,
}

const (
	tgdbBaseURL = "https://api.thegamesdb.net/v1"
	configFile  = "config.json"
)

// Rate limiter: 2 requests per second (1 per 500ms).
var rateLimiter = time.NewTicker(500 * time.Millisecond)

// --- Title normalization ---

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

// --- Config ---

func loadConfig() (*tgdbConfig, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	var c tgdbConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.APIKey == "" {
		return nil, fmt.Errorf("config.json must contain api_key")
	}
	return &c, nil
}

// --- TheGamesDB API ---

func tgdbSearch(title string, platformID *int) ([]tgdbGame, error) {
	<-rateLimiter.C

	u := fmt.Sprintf("%s/Games/ByGameName?apikey=%s&name=%s",
		tgdbBaseURL, cfg.APIKey, urlEncode(title))
	if platformID != nil {
		u += fmt.Sprintf("&filter[platform]=%d", *platformID)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil, fmt.Errorf("unauthorized (bad api_key?)")
	}
	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("rate limited")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("TGDB API status %d: %s", resp.StatusCode, string(body))
	}

	var tResp tgdbResponse
	if err := json.Unmarshal(body, &tResp); err != nil {
		return nil, fmt.Errorf("decode TGDB response: %w", err)
	}

	return tResp.Data.Games, nil
}

func urlEncode(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if isUnreserved(c) {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func isUnreserved(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~'
}

// --- Init ---

func handleInit() (any, *Error) {
	c, err := loadConfig()
	if err != nil {
		log.Printf("WARNING: %v — TGDB plugin will not be functional", err)
		return map[string]any{"status": "not_configured", "reason": err.Error()}, nil
	}

	cfg = *c

	// Test the key with a simple query.
	_, err = tgdbSearch("test", nil)
	if err != nil {
		return nil, &Error{Code: "AUTH_FAILED", Message: err.Error()}
	}

	log.Println("TGDB plugin initialized and authenticated")
	return map[string]any{"status": "ok"}, nil
}

// --- Scoring ---

func scoreCandidate(normalizedQuery string, queryTokens map[string]bool, g *tgdbGame) float64 {
	if normalizeTitle(g.GameTitle) == normalizedQuery {
		return 1.0
	}
	return jaccardSimilarity(queryTokens, tokenize(g.GameTitle))
}

func platformMatches(gamePlatform int, wanted int) bool {
	return gamePlatform == wanted
}

// --- Lookup ---

const minMatchScore = 0.7
const goodMatchScore = 0.9

func matchGame(q gameQuery) (*lookupResult, error) {
	wantedPlatform, havePlatform := platformMap[q.Platform]
	cleanedTitle := normalizeTitle(q.Title)
	queryTokens := tokenize(q.Title)

	var overallBest *tgdbGame
	overallBestScore := -1.0

	// Pass 1: search with platform filter.
	if havePlatform {
		games, err := tgdbSearch(cleanedTitle, &wantedPlatform)
		if err != nil {
			return nil, err
		}
		for i := range games {
			score := scoreCandidate(cleanedTitle, queryTokens, &games[i])
			if score > overallBestScore {
				overallBestScore = score
				overallBest = &games[i]
			}
		}
		if overallBestScore >= goodMatchScore {
			return buildResult(q, overallBest, overallBestScore)
		}
	}

	// Pass 2: search without platform filter.
	games, err := tgdbSearch(cleanedTitle, nil)
	if err != nil {
		return nil, err
	}
	for i := range games {
		score := scoreCandidate(cleanedTitle, queryTokens, &games[i])
		if havePlatform && platformMatches(games[i].Platform, wantedPlatform) {
			score += 0.01
		}
		if score > overallBestScore {
			overallBestScore = score
			overallBest = &games[i]
		}
	}

	return buildResult(q, overallBest, overallBestScore)
}

func buildResult(q gameQuery, best *tgdbGame, score float64) (*lookupResult, error) {
	if best == nil || score < minMatchScore {
		return nil, nil
	}
	return &lookupResult{
		Index:      q.Index,
		Title:      best.GameTitle,
		ExternalID: fmt.Sprintf("%d", best.ID),
		URL:        fmt.Sprintf("https://thegamesdb.net/game.php?id=%d", best.ID),
	}, nil
}

func handleLookup(params lookupParams) (any, *Error) {
	if cfg.APIKey == "" {
		return map[string]any{"results": []any{}}, nil
	}

	var results []lookupResult
	for _, q := range params.Games {
		r, err := matchGame(q)
		if err != nil {
			log.Printf("TGDB lookup error for %q: %v", q.Title, err)
			continue
		}
		if r != nil {
			results = append(results, *r)
		}
	}

	return map[string]any{"results": results}, nil
}

// --- Main ---

func main() {
	log.SetOutput(os.Stderr)
	log.Println("TGDB metadata plugin started")

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
				"plugin_id":      "metadata-tgdb",
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
					origKey := cfg.APIKey
					cfg.APIKey = key
					_, err := tgdbSearch("test", nil)
					cfg.APIKey = origKey
					if err != nil {
						resp.Result = map[string]any{"status": "error", "message": err.Error()}
					} else {
						resp.Result = map[string]any{"status": "ok"}
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
