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
	Index          int      `json:"index"`
	Title          string   `json:"title,omitempty"`
	Platform       string   `json:"platform,omitempty"`
	ExternalID     string   `json:"external_id"`
	URL            string   `json:"url,omitempty"`
	Description    string   `json:"description,omitempty"`
	ReleaseDate    string   `json:"release_date,omitempty"`
	Genres         []string `json:"genres,omitempty"`
	Developer      string   `json:"developer,omitempty"`
	Publisher      string   `json:"publisher,omitempty"`
	CoverURL       string   `json:"cover_url,omitempty"`
	ScreenshotURLs []string `json:"screenshot_urls,omitempty"`
	VideoURLs      []string `json:"video_urls,omitempty"`
	Rating         float64  `json:"rating,omitempty"`
	MaxPlayers     int      `json:"max_players,omitempty"`
}

// Steam API types.

type steamSearchResult struct {
	AppID string `json:"appid"`
	Name  string `json:"name"`
	Icon  string `json:"icon"`
	Logo  string `json:"logo"`
}

type steamAppDetailWrapper struct {
	Success bool            `json:"success"`
	Data    steamAppDetails `json:"data"`
}

type steamAppDetails struct {
	Name             string            `json:"name"`
	ShortDescription string            `json:"short_description"`
	HeaderImage      string            `json:"header_image"`
	Developers       []string          `json:"developers"`
	Publishers       []string          `json:"publishers"`
	Metacritic       *steamMetacritic  `json:"metacritic"`
	Genres           []steamGenre      `json:"genres"`
	Screenshots      []steamScreenshot `json:"screenshots"`
	Movies           []steamMovie      `json:"movies"`
	ReleaseDate      steamReleaseDate  `json:"release_date"`
	Categories       []steamCategory   `json:"categories"`
}

type steamMetacritic struct {
	Score int `json:"score"`
}

type steamGenre struct {
	Description string `json:"description"`
}

type steamScreenshot struct {
	PathFull string `json:"path_full"`
}

type steamMovie struct {
	Webm struct {
		Max string `json:"max"`
	} `json:"webm"`
}

type steamReleaseDate struct {
	Date string `json:"date"`
}

type steamCategory struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
}

// Steam is PC-only; only these platforms make sense.
var supportedPlatforms = map[string]bool{
	"windows_pc": true,
	"ms_dos":     true,
	"scummvm":    true,
}

const searchURL = "https://steamcommunity.com/actions/SearchApps/"

// Rate limiter: 2 requests per second.
var rateLimiter = time.NewTicker(500 * time.Millisecond)

// --- Title normalization (shared logic) ---

var (
	trailingParensRE = regexp.MustCompile(`[\s_]*\([^)]*\)\s*$`)
	setupPrefixRE    = regexp.MustCompile(`^setup[_\s]`)
	versionSuffixRE  = regexp.MustCompile(`[\s._]+v?\d+(\.\d+)+([\s._]+[a-z]{2,3})*\s*$`)
	nonAlphaNumRE    = regexp.MustCompile(`[^a-z0-9\s]+`)
	multiSpaceRE     = regexp.MustCompile(`\s{2,}`)
)

func normalizeTitle(s string) string {
	s = strings.ToLower(s)
	for trailingParensRE.MatchString(s) {
		s = trailingParensRE.ReplaceAllString(s, "")
	}
	s = setupPrefixRE.ReplaceAllString(s, "")
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

// --- Steam API ---

func steamSearch(title string) ([]steamSearchResult, error) {
	<-rateLimiter.C

	reqURL := searchURL + url.PathEscape(title)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("Steam search request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("rate limited")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Steam API status %d: %s", resp.StatusCode, string(body))
	}

	var results []steamSearchResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("decode Steam response: %w", err)
	}
	return results, nil
}

const detailURL = "https://store.steampowered.com/api/appdetails"

func steamAppDetail(appID string) (*steamAppDetails, error) {
	<-rateLimiter.C

	reqURL := fmt.Sprintf("%s?appids=%s&l=english", detailURL, appID)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("Steam detail request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Steam detail status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var wrapper map[string]steamAppDetailWrapper
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("decode Steam detail: %w", err)
	}

	entry, ok := wrapper[appID]
	if !ok || !entry.Success {
		return nil, fmt.Errorf("no data for appid %s", appID)
	}
	return &entry.Data, nil
}

// --- Init ---

func handleInit() (any, *Error) {
	// Smoke-test the search endpoint.
	<-rateLimiter.C
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(searchURL + "test")
	if err != nil {
		return nil, &Error{Code: "API_ERROR", Message: fmt.Sprintf("connectivity check failed: %v", err)}
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, &Error{Code: "API_ERROR", Message: fmt.Sprintf("connectivity check returned status %d", resp.StatusCode)}
	}

	log.Println("Steam plugin initialized")
	return map[string]any{"status": "ok"}, nil
}

// --- Lookup ---

func handleLookup(params lookupParams) (any, *Error) {
	var results []lookupResult
	for _, q := range params.Games {
		if !supportedPlatforms[q.Platform] {
			continue
		}
		r, err := matchGame(q)
		if err != nil {
			log.Printf("Steam lookup error for %q: %v", q.Title, err)
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
	cleanedTitle := normalizeTitle(q.Title)
	queryTokens := tokenize(q.Title)

	// Pass 1: search with the cleaned title.
	results, err := steamSearch(cleanedTitle)
	if err != nil {
		return nil, err
	}

	var best *steamSearchResult
	bestScore := -1.0

	for i := range results {
		score := scoreCandidate(cleanedTitle, queryTokens, results[i].Name)
		if score > bestScore {
			bestScore = score
			best = &results[i]
		}
	}

	if best == nil || bestScore < minMatchScore {
		return nil, nil
	}

	r := &lookupResult{
		Index:      q.Index,
		Title:      best.Name,
		ExternalID: best.AppID,
		URL:        fmt.Sprintf("https://store.steampowered.com/app/%s", best.AppID),
	}

	detail, err := steamAppDetail(best.AppID)
	if err != nil {
		log.Printf("Steam detail fetch for %s: %v (continuing with search data)", best.AppID, err)
		return r, nil
	}

	r.Description = detail.ShortDescription
	r.CoverURL = detail.HeaderImage
	r.ReleaseDate = detail.ReleaseDate.Date
	if len(detail.Developers) > 0 {
		r.Developer = detail.Developers[0]
	}
	if len(detail.Publishers) > 0 {
		r.Publisher = detail.Publishers[0]
	}
	if detail.Metacritic != nil && detail.Metacritic.Score > 0 {
		r.Rating = float64(detail.Metacritic.Score)
	}
	for _, g := range detail.Genres {
		r.Genres = append(r.Genres, g.Description)
	}
	for _, ss := range detail.Screenshots {
		if ss.PathFull != "" {
			r.ScreenshotURLs = append(r.ScreenshotURLs, ss.PathFull)
		}
	}
	for _, mv := range detail.Movies {
		if mv.Webm.Max != "" {
			r.VideoURLs = append(r.VideoURLs, mv.Webm.Max)
		}
	}

	for _, cat := range detail.Categories {
		if cat.ID == 1 {
			r.MaxPlayers = 2
			break
		}
	}

	return r, nil
}

func scoreCandidate(normalizedQuery string, queryTokens map[string]bool, candidateName string) float64 {
	normalizedCandidate := normalizeTitle(candidateName)
	if normalizedCandidate == normalizedQuery {
		return 1.0
	}

	candidateTokens := tokenize(candidateName)
	jaccard := jaccardSimilarity(queryTokens, candidateTokens)

	// Store titles often have subtitles (": 20th Anniversary World Tour", etc.).
	// If the candidate starts with the full query, query has 3+ tokens, and the
	// suffix doesn't look like a sequel number, apply a prefix bonus.
	if len(queryTokens) >= 3 && strings.HasPrefix(normalizedCandidate, normalizedQuery+" ") {
		suffix := normalizedCandidate[len(normalizedQuery)+1:]
		if !isSequelSuffix(suffix) {
			ratio := float64(len(normalizedQuery)) / float64(len(normalizedCandidate))
			prefixScore := 0.75 + 0.2*ratio
			if prefixScore > jaccard {
				return prefixScore
			}
		}
	}

	return jaccard
}

func isSequelSuffix(suffix string) bool {
	words := strings.Fields(suffix)
	if len(words) == 0 {
		return false
	}
	first := words[0]
	// Pure cardinal number (2, 3, 64) indicates a sequel.
	// Ordinals like "20th", "25th" are anniversaries, not sequels.
	if len(first) > 0 && first[0] >= '0' && first[0] <= '9' {
		for _, c := range first {
			if c < '0' || c > '9' {
				return false
			}
		}
		return true
	}
	romans := map[string]bool{
		"ii": true, "iii": true, "iv": true,
		"vi": true, "vii": true, "viii": true, "ix": true,
	}
	return romans[first]
}

// --- Main ---

func main() {
	log.SetOutput(os.Stderr)
	log.Println("Steam metadata plugin started")

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
				"plugin_id":      "metadata-steam",
				"plugin_version": "1.0.0",
				"capabilities":   []string{"metadata"},
			}

		case "plugin.check_config":
			resp.Result = map[string]any{"status": "ok"}

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
