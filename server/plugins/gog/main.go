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

// GOG catalog API types.

type gogCatalogResponse struct {
	ProductCount int          `json:"productCount"`
	Products     []gogProduct `json:"products"`
}

type gogProduct struct {
	ID               string   `json:"id"`
	Slug             string   `json:"slug"`
	Title            string   `json:"title"`
	ProductType      string   `json:"productType"`
	Developers       []string `json:"developers"`
	Publishers       []string `json:"publishers"`
	OperatingSystems []string `json:"operatingSystems"`
	CoverVertical    string   `json:"coverVertical"`
	ReleaseDate      string   `json:"releaseDate"`
	Genres           []gogTag `json:"genres"`
}

type gogTag struct {
	Name string `json:"name"`
}

// GOG product detail API types.

type gogProductDetail struct {
	Description *gogDescription   `json:"description"`
	Screenshots []gogScreenshot   `json:"screenshots"`
	Videos      []gogVideo        `json:"videos"`
}

type gogDescription struct {
	Full string `json:"full"`
	Lead string `json:"lead"`
}

type gogScreenshot struct {
	URL string `json:"url"`
}

type gogVideo struct {
	VideoURL string `json:"video_url"`
}

// GOG is PC-only; only these platforms make sense.
var supportedPlatforms = map[string]bool{
	"windows_pc": true,
	"ms_dos":     true,
	"scummvm":    true,
}

const catalogURL = "https://catalog.gog.com/v1/catalog"

// Rate limiter: 3 requests per second.
var rateLimiter = time.NewTicker(333 * time.Millisecond)

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

// --- GOG API ---

func gogSearch(title string) ([]gogProduct, error) {
	<-rateLimiter.C

	params := url.Values{
		"countryCode":  {"US"},
		"currencyCode": {"USD"},
		"locale":       {"en-US"},
		"limit":        {"20"},
		"order":        {"desc:score"},
		"productType":  {"in:game,pack"},
		"query":        {"like:" + title},
	}

	reqURL := catalogURL + "?" + params.Encode()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("GOG catalog request: %w", err)
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
		return nil, fmt.Errorf("GOG API status %d: %s", resp.StatusCode, string(body))
	}

	var result gogCatalogResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode GOG response: %w", err)
	}
	return result.Products, nil
}

const gogDetailURL = "https://api.gog.com/products"

func gogFetchDetail(productID string) (*gogProductDetail, error) {
	<-rateLimiter.C

	reqURL := fmt.Sprintf("%s/%s?expand=description,screenshots,videos&locale=en_US", gogDetailURL, productID)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("GOG detail request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GOG detail status %d", resp.StatusCode)
	}

	var detail gogProductDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, fmt.Errorf("decode GOG detail: %w", err)
	}
	return &detail, nil
}

// --- Init ---

func handleInit() (any, *Error) {
	// Smoke-test the catalog endpoint.
	<-rateLimiter.C
	params := url.Values{
		"countryCode":  {"US"},
		"currencyCode": {"USD"},
		"locale":       {"en-US"},
		"limit":        {"1"},
		"productType":  {"in:game"},
		"query":        {"like:test"},
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(catalogURL + "?" + params.Encode())
	if err != nil {
		return nil, &Error{Code: "API_ERROR", Message: fmt.Sprintf("connectivity check failed: %v", err)}
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, &Error{Code: "API_ERROR", Message: fmt.Sprintf("connectivity check returned status %d", resp.StatusCode)}
	}

	log.Println("GOG plugin initialized")
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
			log.Printf("GOG lookup error for %q: %v", q.Title, err)
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
	products, err := gogSearch(cleanedTitle)
	if err != nil {
		return nil, err
	}

	var best *gogProduct
	bestScore := -1.0

	for i := range products {
		if products[i].ProductType != "game" && products[i].ProductType != "pack" {
			continue
		}
		score := scoreCandidate(cleanedTitle, queryTokens, products[i].Title)
		if score > bestScore {
			bestScore = score
			best = &products[i]
		}
	}

	if bestScore >= goodMatchScore {
		return buildResult(q.Index, best), nil
	}

	// Pass 2: try with the raw title (before normalization) if different.
	rawLower := strings.ToLower(strings.TrimSpace(q.Title))
	if rawLower != cleanedTitle {
		products2, err := gogSearch(rawLower)
		if err != nil {
			return nil, err
		}
		for i := range products2 {
			if products2[i].ProductType != "game" && products2[i].ProductType != "pack" {
				continue
			}
			score := scoreCandidate(cleanedTitle, queryTokens, products2[i].Title)
			if score > bestScore {
				bestScore = score
				best = &products2[i]
			}
		}
	}

	if best == nil || bestScore < minMatchScore {
		return nil, nil
	}

	return buildResult(q.Index, best), nil
}

func buildResult(index int, p *gogProduct) *lookupResult {
	r := &lookupResult{
		Index:      index,
		Title:      p.Title,
		ExternalID: p.ID,
		URL:        fmt.Sprintf("https://www.gog.com/en/game/%s", p.Slug),
	}

	if len(p.Developers) > 0 {
		r.Developer = p.Developers[0]
	}
	if len(p.Publishers) > 0 {
		r.Publisher = p.Publishers[0]
	}
	if p.CoverVertical != "" {
		r.Media = append(r.Media, mediaItem{Type: "cover", URL: p.CoverVertical})
	}
	if p.ReleaseDate != "" {
		r.ReleaseDate = p.ReleaseDate
	}
	for _, g := range p.Genres {
		r.Genres = append(r.Genres, g.Name)
	}

	detail, err := gogFetchDetail(p.ID)
	if err != nil {
		log.Printf("GOG detail fetch for %s: %v (continuing with catalog data)", p.ID, err)
		return r
	}
	if detail.Description != nil {
		if detail.Description.Lead != "" {
			r.Description = detail.Description.Lead
		} else if detail.Description.Full != "" {
			r.Description = detail.Description.Full
		}
	}
	for _, ss := range detail.Screenshots {
		if ss.URL != "" {
			u := ss.URL
			if strings.HasPrefix(u, "//") {
				u = "https:" + u
			}
			r.Media = append(r.Media, mediaItem{Type: "screenshot", URL: u})
		}
	}
	for _, v := range detail.Videos {
		if v.VideoURL != "" {
			r.Media = append(r.Media, mediaItem{Type: "video", URL: v.VideoURL})
		}
	}

	return r
}

func scoreCandidate(normalizedQuery string, queryTokens map[string]bool, candidateName string) float64 {
	normalizedCandidate := normalizeTitle(candidateName)
	if normalizedCandidate == normalizedQuery {
		return 1.0
	}

	candidateTokens := tokenize(candidateName)
	jaccard := jaccardSimilarity(queryTokens, candidateTokens)

	// GOG titles often have subtitles (": Enhanced Edition", ": Complete", etc.).
	// If all query tokens appear in the candidate AND the query has 3+ tokens,
	// apply a containment-aware bonus — but reject if the suffix after the query
	// starts with a digit or roman numeral (likely a sequel, not a subtitle).
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
	log.Println("GOG metadata plugin started")

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
				"plugin_id":      "metadata-gog",
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
