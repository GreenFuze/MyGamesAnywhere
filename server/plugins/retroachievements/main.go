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
	"strconv"
	"strings"
	"time"
	"unicode"
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

// Plugin config.

type raConfig struct {
	APIKey   string `json:"api_key"`
	Username string `json:"username"`
}

var cfg raConfig

const (
	raAPIBase  = "https://retroachievements.org/API"
	configFile = "config.json"
)

// Rate limiter: ~4 req/s to be polite to RA servers.
var rateLimiter = time.NewTicker(250 * time.Millisecond)

// Platform -> RA console ID mapping.
// RA uses numeric console IDs; a single platform may map to multiple.
var platformToConsoleIDs = map[string][]int{
	"gba":      {5},
	"gbc":      {6},
	"gb":       {4},
	"nes":      {7},
	"snes":     {3},
	"n64":      {2},
	"nds":      {18},
	"ps1":      {12},
	"ps2":      {21},
	"psp":      {41},
	"arcade":   {27},
	"genesis":  {1},
	"megadrive": {1},
	"sms":      {11},
	"ms_dos":   {},
	"xbox_360": {},
	"scummvm":  {},
	"ps3":      {},
}

// IPC metadata request/response types (matching the server's contract).

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

// RA API response types.

type raGameListEntry struct {
	Title       string `json:"Title"`
	ID          int    `json:"ID"`
	ConsoleID   int    `json:"ConsoleID"`
	ConsoleName string `json:"ConsoleName"`
	ImageIcon   string `json:"ImageIcon"`
	NumAchievements int `json:"NumAchievements"`
}

type raGameExtended struct {
	ID              int                    `json:"ID"`
	Title           string                 `json:"Title"`
	ConsoleID       int                    `json:"ConsoleID"`
	ConsoleName     string                 `json:"ConsoleName"`
	Genre           string                 `json:"Genre"`
	Developer       string                 `json:"Developer"`
	Publisher       string                 `json:"Publisher"`
	Released        string                 `json:"Released"`
	ImageIcon       string                 `json:"ImageIcon"`
	ImageTitle      string                 `json:"ImageTitle"`
	ImageIngame     string                 `json:"ImageIngame"`
	ImageBoxArt     string                 `json:"ImageBoxArt"`
	RichPresencePatch string               `json:"RichPresencePatch"`
	NumAchievements int                    `json:"NumAchievements"`
	NumDistinctPlayersCasual int           `json:"NumDistinctPlayersCasual"`
	Achievements    map[string]raAchievement `json:"Achievements"`
}

type raAchievement struct {
	ID             int    `json:"ID"`
	Title          string `json:"Title"`
	Description    string `json:"Description"`
	Points         int    `json:"Points"`
	TrueRatio      int    `json:"TrueRatio"`
	BadgeName      string `json:"BadgeName"`
	NumAwarded     int    `json:"NumAwarded"`
	NumAwardedHardcore int `json:"NumAwardedHardcore"`
	DisplayOrder   int    `json:"DisplayOrder"`
	DateEarned     string `json:"DateEarned,omitempty"`
	DateEarnedHardcore string `json:"DateEarnedHardcore,omitempty"`
}

// RA API calls.

func raGet(endpoint string, params url.Values) ([]byte, error) {
	<-rateLimiter.C

	params.Set("z", cfg.Username)
	params.Set("y", cfg.APIKey)

	apiURL := fmt.Sprintf("%s/%s?%s", raAPIBase, endpoint, params.Encode())

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("RA API %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("RA API %s read body: %w", endpoint, err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("RA API %s: status %d: %s", endpoint, resp.StatusCode, string(body))
	}
	return body, nil
}

func fetchGameList(consoleID int) ([]raGameListEntry, error) {
	params := url.Values{
		"i": {strconv.Itoa(consoleID)},
		"h": {"1"}, // include hashes
	}
	data, err := raGet("API_GetGameList.php", params)
	if err != nil {
		return nil, err
	}
	var games []raGameListEntry
	if err := json.Unmarshal(data, &games); err != nil {
		return nil, fmt.Errorf("decode game list: %w", err)
	}
	return games, nil
}

func fetchGameExtended(gameID int) (*raGameExtended, error) {
	params := url.Values{
		"i": {strconv.Itoa(gameID)},
	}
	data, err := raGet("API_GetGameExtended.php", params)
	if err != nil {
		return nil, err
	}
	var game raGameExtended
	if err := json.Unmarshal(data, &game); err != nil {
		return nil, fmt.Errorf("decode game extended: %w", err)
	}
	return &game, nil
}

func fetchUserGameProgress(gameID int) (*raGameExtended, error) {
	params := url.Values{
		"g": {strconv.Itoa(gameID)},
		"u": {cfg.Username},
	}
	data, err := raGet("API_GetGameInfoAndUserProgress.php", params)
	if err != nil {
		return nil, err
	}
	var game raGameExtended
	if err := json.Unmarshal(data, &game); err != nil {
		return nil, fmt.Errorf("decode user progress: %w", err)
	}
	return &game, nil
}

// Title normalization for matching.

var multiSpace = regexp.MustCompile(`\s+`)

func normalizeTitle(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			return unicode.ToLower(r)
		}
		return ' '
	}, s)
	s = multiSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

func tokenize(s string) map[string]bool {
	tokens := make(map[string]bool)
	for _, t := range strings.Fields(normalizeTitle(s)) {
		tokens[t] = true
	}
	return tokens
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

// Game list cache: console ID -> game list, to avoid re-fetching per query.
var gameListCache = make(map[int][]raGameListEntry)

func getGameList(consoleID int) ([]raGameListEntry, error) {
	if cached, ok := gameListCache[consoleID]; ok {
		return cached, nil
	}
	games, err := fetchGameList(consoleID)
	if err != nil {
		return nil, err
	}
	gameListCache[consoleID] = games
	log.Printf("cached %d games for console %d", len(games), consoleID)
	return games, nil
}

const minMatchScore = 0.7

func raImageURL(path string) string {
	if path == "" {
		return ""
	}
	return "https://retroachievements.org" + path
}

// metadata.game.lookup handler.

func matchGame(q gameQuery) (*lookupResult, error) {
	consoleIDs, ok := platformToConsoleIDs[q.Platform]
	if !ok || len(consoleIDs) == 0 {
		return nil, nil
	}

	queryNorm := normalizeTitle(q.Title)
	queryTokens := tokenize(q.Title)
	if len(queryTokens) == 0 {
		return nil, nil
	}

	var bestGame *raGameListEntry
	var bestScore float64

	for _, cid := range consoleIDs {
		games, err := getGameList(cid)
		if err != nil {
			log.Printf("RA game list for console %d failed: %v", cid, err)
			continue
		}

		for i := range games {
			g := &games[i]
			candidateNorm := normalizeTitle(g.Title)

			var score float64
			if candidateNorm == queryNorm {
				score = 1.0
			} else {
				score = jaccardSimilarity(queryTokens, tokenize(g.Title))
			}

			if score > bestScore {
				bestScore = score
				bestGame = g
			}
		}
	}

	if bestGame == nil || bestScore < minMatchScore {
		return nil, nil
	}

	ext, err := fetchGameExtended(bestGame.ID)
	if err != nil {
		log.Printf("RA extended info for %d failed: %v", bestGame.ID, err)
		r := &lookupResult{
			Index:      q.Index,
			Title:      bestGame.Title,
			ExternalID: strconv.Itoa(bestGame.ID),
			URL:        fmt.Sprintf("https://retroachievements.org/game/%d", bestGame.ID),
		}
		if bestGame.ImageIcon != "" {
			r.Media = append(r.Media, mediaItem{Type: "icon", URL: raImageURL(bestGame.ImageIcon)})
		}
		return r, nil
	}

	r := &lookupResult{
		Index:      q.Index,
		Title:      ext.Title,
		Platform:   q.Platform,
		ExternalID: strconv.Itoa(ext.ID),
		URL:        fmt.Sprintf("https://retroachievements.org/game/%d", ext.ID),
		Developer:  ext.Developer,
		Publisher:  ext.Publisher,
	}

	if ext.Released != "" {
		r.ReleaseDate = ext.Released
	}
	if ext.Genre != "" {
		for _, g := range strings.Split(ext.Genre, ", ") {
			g = strings.TrimSpace(g)
			if g != "" {
				r.Genres = append(r.Genres, g)
			}
		}
	}

	if ext.ImageBoxArt != "" {
		r.Media = append(r.Media, mediaItem{Type: "cover", URL: raImageURL(ext.ImageBoxArt)})
	}
	if ext.ImageIcon != "" {
		r.Media = append(r.Media, mediaItem{Type: "icon", URL: raImageURL(ext.ImageIcon)})
	}
	if ext.ImageTitle != "" {
		r.Media = append(r.Media, mediaItem{Type: "logo", URL: raImageURL(ext.ImageTitle)})
	}
	if ext.ImageIngame != "" {
		r.Media = append(r.Media, mediaItem{Type: "screenshot", URL: raImageURL(ext.ImageIngame)})
	}

	return r, nil
}

func handleLookup(params lookupParams) (any, *Error) {
	if cfg.APIKey == "" || cfg.Username == "" {
		return map[string]any{"results": []any{}}, nil
	}

	var results []lookupResult
	for _, q := range params.Games {
		r, err := matchGame(q)
		if err != nil {
			log.Printf("RA lookup error for %q: %v", q.Title, err)
			continue
		}
		if r != nil {
			results = append(results, *r)
		}
	}

	return map[string]any{"results": results}, nil
}

// achievements.game.get handler.

type achievementEntry struct {
	ExternalID   string  `json:"external_id"`
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	LockedIcon   string  `json:"locked_icon,omitempty"`
	UnlockedIcon string  `json:"unlocked_icon,omitempty"`
	Points       int     `json:"points,omitempty"`
	Rarity       float64 `json:"rarity,omitempty"`
	Unlocked     bool    `json:"unlocked"`
	UnlockedAt   string  `json:"unlocked_at,omitempty"`
}

func handleAchievementsGet(params json.RawMessage) (any, *Error) {
	var p struct {
		ExternalGameID string `json:"external_game_id"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	if p.ExternalGameID == "" {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "external_game_id required"}
	}
	if cfg.APIKey == "" || cfg.Username == "" {
		return nil, &Error{Code: "NOT_CONFIGURED", Message: "retroachievements plugin not configured"}
	}

	gameID, err := strconv.Atoi(p.ExternalGameID)
	if err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "external_game_id must be a numeric RA game ID"}
	}

	game, err := fetchUserGameProgress(gameID)
	if err != nil {
		return nil, &Error{Code: "API_ERROR", Message: err.Error()}
	}

	totalPlayers := game.NumDistinctPlayersCasual
	achievements := make([]achievementEntry, 0, len(game.Achievements))
	unlocked := 0
	totalPoints := 0
	earnedPoints := 0

	for _, a := range game.Achievements {
		badgeURL := ""
		badgeLockedURL := ""
		if a.BadgeName != "" {
			badgeURL = fmt.Sprintf("https://media.retroachievements.org/Badge/%s.png", a.BadgeName)
			badgeLockedURL = fmt.Sprintf("https://media.retroachievements.org/Badge/%s_lock.png", a.BadgeName)
		}

		entry := achievementEntry{
			ExternalID:   strconv.Itoa(a.ID),
			Title:        a.Title,
			Description:  a.Description,
			LockedIcon:   badgeLockedURL,
			UnlockedIcon: badgeURL,
			Points:       a.Points,
		}

		if totalPlayers > 0 && a.NumAwarded > 0 {
			entry.Rarity = float64(a.NumAwarded) / float64(totalPlayers) * 100.0
		}

		totalPoints += a.Points

		earned := a.DateEarned != "" || a.DateEarnedHardcore != ""
		entry.Unlocked = earned
		if a.DateEarnedHardcore != "" {
			entry.UnlockedAt = a.DateEarnedHardcore
		} else if a.DateEarned != "" {
			entry.UnlockedAt = a.DateEarned
		}
		if earned {
			unlocked++
			earnedPoints += a.Points
		}

		achievements = append(achievements, entry)
	}

	log.Printf("RA achievements for game %d: %d/%d unlocked, %d/%d points",
		gameID, unlocked, len(achievements), earnedPoints, totalPoints)

	return map[string]any{
		"source":           "retroachievements",
		"external_game_id": p.ExternalGameID,
		"total_count":      len(achievements),
		"unlocked_count":   unlocked,
		"total_points":     totalPoints,
		"earned_points":    earnedPoints,
		"achievements":     achievements,
	}, nil
}

// Config loading.

func loadConfig() (*raConfig, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	var c raConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.APIKey == "" {
		return nil, fmt.Errorf("config.json must contain api_key")
	}
	if c.Username == "" {
		return nil, fmt.Errorf("config.json must contain username")
	}
	return &c, nil
}

// Init handler.

func handleInit() (any, *Error) {
	c, err := loadConfig()
	if err != nil {
		log.Printf("WARNING: %v — RetroAchievements plugin will not be functional", err)
		return map[string]any{"status": "not_configured", "reason": err.Error()}, nil
	}
	cfg = *c

	// Validate by fetching console list (lightweight call).
	params := url.Values{}
	_, err = raGet("API_GetConsoleIDs.php", params)
	if err != nil {
		return nil, &Error{Code: "AUTH_FAILED", Message: fmt.Sprintf("RA API validation failed: %v", err)}
	}

	log.Printf("RetroAchievements plugin initialized for user %q", cfg.Username)
	return map[string]any{
		"status":   "ok",
		"username": cfg.Username,
	}, nil
}

// Main IPC loop.

func main() {
	log.SetOutput(os.Stderr)
	log.Println("RetroAchievements plugin started")

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
				"plugin_id":      "retroachievements",
				"plugin_version": "1.0.0",
				"capabilities":   []string{"metadata", "achievements"},
			}

		case "plugin.check_config":
			var p struct {
				Config map[string]any `json:"config"`
			}
			if err := json.Unmarshal(req.Params, &p); err == nil {
				key, _ := p.Config["api_key"].(string)
				user, _ := p.Config["username"].(string)
				if key == "" || user == "" {
					resp.Result = map[string]any{"status": "error", "message": "api_key and username required"}
				} else {
					origCfg := cfg
					cfg = raConfig{APIKey: key, Username: user}
					params := url.Values{}
					_, err := raGet("API_GetConsoleIDs.php", params)
					cfg = origCfg
					if err != nil {
						resp.Result = map[string]any{"status": "error", "message": err.Error()}
					} else {
						resp.Result = map[string]any{"status": "ok"}
					}
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

		case "achievements.game.get":
			result, errObj := handleAchievementsGet(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}

		default:
			resp.Error = &Error{Code: "UNKNOWN_METHOD", Message: "unknown method: " + req.Method}
		}

		out, _ := json.Marshal(resp)
		binary.Write(os.Stdout, binary.BigEndian, uint32(len(out)))
		os.Stdout.Write(out)
	}
}
