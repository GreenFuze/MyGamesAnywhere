package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
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

// IPC metadata types.

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

type completionTimeResult struct {
	MainStory     float64 `json:"main_story,omitempty"`
	MainExtra     float64 `json:"main_extra,omitempty"`
	Completionist float64 `json:"completionist,omitempty"`
}

type lookupResult struct {
	Index          int                   `json:"index"`
	Title          string                `json:"title,omitempty"`
	Platform       string                `json:"platform,omitempty"`
	ExternalID     string                `json:"external_id"`
	URL            string                `json:"url,omitempty"`
	Description    string                `json:"description,omitempty"`
	ReleaseDate    string                `json:"release_date,omitempty"`
	Genres         []string              `json:"genres,omitempty"`
	Developer      string                `json:"developer,omitempty"`
	Publisher      string                `json:"publisher,omitempty"`
	Media          []mediaItem           `json:"media,omitempty"`
	Rating         float64               `json:"rating,omitempty"`
	MaxPlayers     int                   `json:"max_players,omitempty"`
	CompletionTime *completionTimeResult `json:"completion_time,omitempty"`
}

// HLTB API constants and types.

const (
	hltbBaseURL      = "https://howlongtobeat.com"
	hltbFindInitURL  = hltbBaseURL + "/api/find/init"
	hltbFindURL      = hltbBaseURL + "/api/find"
	hltbLegacySearch = hltbBaseURL + "/api/search"
	hltbImageURL     = hltbBaseURL + "/games/"
)

// 10 req/s rate limiter.
var rateLimiter = time.NewTicker(100 * time.Millisecond)

type hltbSearchPayload struct {
	SearchType    string            `json:"searchType"`
	SearchTerms   []string          `json:"searchTerms"`
	SearchPage    int               `json:"searchPage"`
	Size          int               `json:"size"`
	SearchOptions hltbSearchOptions `json:"searchOptions"`
	UseCache      bool              `json:"useCache"`
}

type hltbSearchOptions struct {
	Games      hltbGameOptions `json:"games"`
	Users      hltbUserOptions `json:"users"`
	Lists      hltbListOptions `json:"lists"`
	Filter     string          `json:"filter"`
	Sort       int             `json:"sort"`
	Randomizer int             `json:"randomizer"`
}

type hltbGameOptions struct {
	UserID        int           `json:"userId"`
	Platform      string        `json:"platform"`
	SortCategory  string        `json:"sortCategory"`
	RangeCategory string        `json:"rangeCategory"`
	RangeTime     hltbRange     `json:"rangeTime"`
	Gameplay      hltbGameplay  `json:"gameplay"`
	RangeYear     hltbRangeYear `json:"rangeYear"`
	Modifier      string        `json:"modifier"`
}

type hltbRange struct {
	Min *int `json:"min"`
	Max *int `json:"max"`
}

type hltbRangeYear struct {
	Min string `json:"min"`
	Max string `json:"max"`
}

type hltbGameplay struct {
	Perspective string `json:"perspective"`
	Flow        string `json:"flow"`
	Genre       string `json:"genre"`
	Difficulty  string `json:"difficulty"`
}

type hltbUserOptions struct {
	SortCategory string `json:"sortCategory"`
}

type hltbListOptions struct {
	SortCategory string `json:"sortCategory"`
}

type hltbSearchResponse struct {
	Color       string     `json:"color"`
	Title       string     `json:"title"`
	Category    string     `json:"category"`
	Count       int        `json:"count"`
	PageCurrent int        `json:"pageCurrent"`
	PageTotal   int        `json:"pageTotal"`
	PageSize    int        `json:"pageSize"`
	Data        []hltbGame `json:"data"`
}

type hltbGame struct {
	GameID          int    `json:"game_id"`
	GameName        string `json:"game_name"`
	GameNameDate    int    `json:"game_name_date"`
	GameAlias       string `json:"game_alias"`
	GameType        string `json:"game_type"`
	GameImage       string `json:"game_image"`
	CompMain        int    `json:"comp_main"`   // seconds
	CompPlus        int    `json:"comp_plus"`   // seconds
	CompComplete    int    `json:"comp_100"`    // seconds
	CompAll         int    `json:"comp_all"`    // seconds
	InvestedCo      int    `json:"invested_co"` // seconds
	InvestedMp      int    `json:"invested_mp"` // seconds
	CountComp       int    `json:"count_comp"`
	CountPlaying    int    `json:"count_playing"`
	CountBacklog    int    `json:"count_backlog"`
	CountReview     int    `json:"count_review"`
	ReviewScore     int    `json:"review_score"`
	CountRetired    int    `json:"count_retired"`
	ProfileDev      string `json:"profile_dev"`
	ProfilePopular  int    `json:"profile_popular"`
	ProfileSteam    int    `json:"profile_steam"`
	ProfilePlatform string `json:"profile_platform"`
	ReleaseWorld    int    `json:"release_world"`
}

// HLTB API calls.

var httpClient = &http.Client{Timeout: 15 * time.Second}

type hltbFindInitResponse struct {
	Token string `json:"token"`
	HPKey string `json:"hpKey"`
	HPVal string `json:"hpVal"`
}

func applyHLTBHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Origin", hltbBaseURL)
	req.Header.Set("Referer", hltbBaseURL+"/")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
}

func initHLTBSearchAuth() (*hltbFindInitResponse, error) {
	req, err := http.NewRequest("GET", hltbFindInitURL+fmt.Sprintf("?t=%d", time.Now().UnixMilli()), nil)
	if err != nil {
		return nil, fmt.Errorf("new init request: %w", err)
	}
	applyHLTBHeaders(req)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HLTB auth init request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read HLTB auth init response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HLTB auth init: status %d: %s", resp.StatusCode, string(respBody[:min(len(respBody), 200)]))
	}

	var result hltbFindInitResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode HLTB auth init response: %w", err)
	}
	if result.Token == "" || result.HPKey == "" || result.HPVal == "" {
		return nil, fmt.Errorf("HLTB auth init returned incomplete token payload")
	}
	return &result, nil
}

func searchHLTBViaFind(query string) (*hltbSearchResponse, error) {
	auth, err := initHLTBSearchAuth()
	if err != nil {
		return nil, err
	}

	terms := strings.Fields(query)
	if len(terms) == 0 {
		return nil, fmt.Errorf("empty search query")
	}

	payload := hltbSearchPayload{
		SearchType:  "games",
		SearchTerms: terms,
		SearchPage:  1,
		Size:        20,
		UseCache:    true,
		SearchOptions: hltbSearchOptions{
			Games: hltbGameOptions{
				UserID:        0,
				Platform:      "",
				SortCategory:  "popular",
				RangeCategory: "main",
				RangeTime:     hltbRange{},
				Gameplay:      hltbGameplay{},
				RangeYear:     hltbRangeYear{},
				Modifier:      "",
			},
			Users:      hltbUserOptions{SortCategory: "postcount"},
			Lists:      hltbListOptions{SortCategory: "follows"},
			Filter:     "",
			Sort:       0,
			Randomizer: 0,
		},
	}

	bodyMap := map[string]any{
		"searchType":    payload.SearchType,
		"searchTerms":   payload.SearchTerms,
		"searchPage":    payload.SearchPage,
		"size":          payload.Size,
		"searchOptions": payload.SearchOptions,
		"useCache":      payload.UseCache,
		auth.HPKey:      auth.HPVal,
	}

	body, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("marshal search payload: %w", err)
	}

	req, err := http.NewRequest("POST", hltbFindURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	applyHLTBHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-auth-token", auth.Token)
	req.Header.Set("x-hp-key", auth.HPKey)
	req.Header.Set("x-hp-val", auth.HPVal)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HLTB search request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read HLTB response: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HLTB search: status %d: %s", resp.StatusCode, string(respBody[:min(len(respBody), 200)]))
	}

	var result hltbSearchResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode HLTB response: %w", err)
	}
	return &result, nil
}

func searchHLTBLegacy(query string) (*hltbSearchResponse, error) {
	terms := strings.Fields(query)
	if len(terms) == 0 {
		return nil, fmt.Errorf("empty search query")
	}

	payload := hltbSearchPayload{
		SearchType:  "games",
		SearchTerms: terms,
		SearchPage:  1,
		Size:        20,
		SearchOptions: hltbSearchOptions{
			Games: hltbGameOptions{
				UserID:        0,
				Platform:      "",
				SortCategory:  "popular",
				RangeCategory: "main",
				RangeTime:     hltbRange{},
				Gameplay:      hltbGameplay{},
				RangeYear:     hltbRangeYear{},
				Modifier:      "",
			},
			Users:      hltbUserOptions{SortCategory: "postcount"},
			Lists:      hltbListOptions{SortCategory: "follows"},
			Filter:     "",
			Sort:       0,
			Randomizer: 0,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal legacy search payload: %w", err)
	}

	req, err := http.NewRequest("POST", hltbLegacySearch, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("new legacy request: %w", err)
	}
	applyHLTBHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HLTB legacy search request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read HLTB legacy response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HLTB legacy search: status %d: %s", resp.StatusCode, string(respBody[:min(len(respBody), 200)]))
	}

	var result hltbSearchResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode HLTB legacy response: %w", err)
	}
	return &result, nil
}

func searchHLTB(query string) (*hltbSearchResponse, error) {
	<-rateLimiter.C

	resp, err := searchHLTBViaFind(query)
	if err == nil {
		return resp, nil
	}

	log.Printf("HLTB live search fallback for %q after api/find failed: %v", query, err)

	legacyResp, legacyErr := searchHLTBLegacy(query)
	if legacyErr == nil {
		return legacyResp, nil
	}
	return nil, fmt.Errorf("api/find failed: %v; legacy fallback failed: %v", err, legacyErr)
}

// Title normalization and matching.

var multiSpace = regexp.MustCompile(`\s+`)

func normalizeTitle(s string) string {
	return titlematch.NormalizeLookupTitle(s)
}

func tokenize(s string) map[string]bool {
	return titlematch.TokenizeLookupTitle(s)
}

func jaccardSimilarity(a, b map[string]bool) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	intersect := 0
	for k := range a {
		if b[k] {
			intersect++
		}
	}
	union := len(a) + len(b) - intersect
	if union == 0 {
		return 0
	}
	return float64(intersect) / float64(union)
}

const minMatchScore = 0.7

func secondsToHours(s int) float64 {
	if s <= 0 {
		return 0
	}
	return math.Round(float64(s)/3600*10) / 10
}

func matchGame(q gameQuery) (*lookupResult, error) {
	resp, err := searchHLTB(q.Title)
	if err != nil {
		return nil, err
	}

	if len(resp.Data) == 0 {
		return nil, nil
	}

	queryNorm := normalizeTitle(q.Title)
	queryTokens := tokenize(q.Title)
	if len(queryTokens) == 0 {
		return nil, nil
	}

	var bestGame *hltbGame
	var bestScore float64

	for i := range resp.Data {
		g := &resp.Data[i]
		if g.GameType != "" && g.GameType != "game" {
			continue
		}
		candidateNorm := normalizeTitle(g.GameName)

		var score float64
		if candidateNorm == queryNorm {
			score = 1.0
		} else {
			score = jaccardSimilarity(queryTokens, tokenize(g.GameName))
		}

		// Also check aliases (pipe-separated).
		if g.GameAlias != "" {
			for _, alias := range strings.Split(g.GameAlias, ",") {
				alias = strings.TrimSpace(alias)
				if alias == "" {
					continue
				}
				aliasNorm := normalizeTitle(alias)
				var aliasScore float64
				if aliasNorm == queryNorm {
					aliasScore = 1.0
				} else {
					aliasScore = jaccardSimilarity(queryTokens, tokenize(alias))
				}
				if aliasScore > score {
					score = aliasScore
				}
			}
		}

		if score > bestScore {
			bestScore = score
			bestGame = g
		}
	}

	if bestGame == nil || bestScore < minMatchScore {
		return nil, nil
	}

	r := &lookupResult{
		Index:      q.Index,
		Title:      bestGame.GameName,
		ExternalID: strconv.Itoa(bestGame.GameID),
		URL:        fmt.Sprintf("%s/game/%d", hltbBaseURL, bestGame.GameID),
	}

	if bestGame.GameImage != "" {
		r.Media = append(r.Media, mediaItem{
			Type: "cover",
			URL:  hltbImageURL + bestGame.GameImage,
		})
	}

	if bestGame.ProfileDev != "" {
		r.Developer = bestGame.ProfileDev
	}

	ct := &completionTimeResult{
		MainStory:     secondsToHours(bestGame.CompMain),
		MainExtra:     secondsToHours(bestGame.CompPlus),
		Completionist: secondsToHours(bestGame.CompComplete),
	}
	if ct.MainStory > 0 || ct.MainExtra > 0 || ct.Completionist > 0 {
		r.CompletionTime = ct
	}

	return r, nil
}

func handleLookup(params lookupParams) (any, *Error) {
	var results []lookupResult
	for _, q := range params.Games {
		r, err := matchGame(q)
		if err != nil {
			log.Printf("HLTB lookup error for title=%q platform=%q root_path=%q: %v", q.Title, q.Platform, q.RootPath, err)
			continue
		}
		if r != nil {
			results = append(results, *r)
		}
	}

	return map[string]any{"results": results}, nil
}

// Init handler.

func handleInit() (any, *Error) {
	resp, err := searchHLTB("The Legend of Zelda")
	if err != nil {
		log.Printf("WARNING: HLTB connectivity check failed: %v", err)
		return map[string]any{"status": "degraded", "reason": err.Error()}, nil
	}
	log.Printf("HLTB plugin initialized (connectivity OK, test returned %d results)", len(resp.Data))
	return map[string]any{"status": "ok"}, nil
}

// Main IPC loop.

func main() {
	log.SetOutput(os.Stderr)
	log.Println("HLTB metadata plugin started")

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
				"plugin_id":      "metadata-hltb",
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
