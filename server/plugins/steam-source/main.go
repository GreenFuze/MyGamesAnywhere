package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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

// Config.

type steamConfig struct {
	APIKey   string `json:"api_key"`
	SteamID  string `json:"steam_id"`
	VanityURL string `json:"vanity_url"`
}

var cfg steamConfig

const (
	steamAPIBase = "https://api.steampowered.com"
	storeAPIBase = "https://store.steampowered.com/api"
	configFile   = "config.json"
)

// Rate limiter: 2 requests per second for store API.
var rateLimiter = time.NewTicker(500 * time.Millisecond)

// --- Steam API types ---

type ownedGamesResponse struct {
	Response struct {
		GameCount int         `json:"game_count"`
		Games     []ownedGame `json:"games"`
	} `json:"response"`
}

type ownedGame struct {
	AppID           int    `json:"appid"`
	Name            string `json:"name"`
	PlaytimeForever int    `json:"playtime_forever"`
	ImgIconURL      string `json:"img_icon_url"`
	ImgLogoURL      string `json:"img_logo_url"`
}

type vanityURLResponse struct {
	Response struct {
		SteamID string `json:"steamid"`
		Success int    `json:"success"`
		Message string `json:"message"`
	} `json:"response"`
}

type appDetailWrapper struct {
	Success bool       `json:"success"`
	Data    appDetails `json:"data"`
}

type appDetails struct {
	Type             string          `json:"type"`
	Name             string          `json:"name"`
	AppID            int             `json:"steam_appid"`
	ShortDescription string          `json:"short_description"`
	HeaderImage      string          `json:"header_image"`
	Developers       []string        `json:"developers"`
	Publishers       []string        `json:"publishers"`
	Metacritic       *metacriticInfo `json:"metacritic"`
	Genres           []genreInfo     `json:"genres"`
	Screenshots      []screenshotInfo `json:"screenshots"`
	Movies           []movieInfo     `json:"movies"`
	ReleaseDate      releaseDateInfo `json:"release_date"`
}

type metacriticInfo struct {
	Score int `json:"score"`
}

type genreInfo struct {
	Description string `json:"description"`
}

type screenshotInfo struct {
	PathFull string `json:"path_full"`
}

type movieInfo struct {
	Webm struct {
		Max string `json:"max"`
	} `json:"webm"`
}

type releaseDateInfo struct {
	Date string `json:"date"`
}

// --- Steam achievement API types ---

type playerAchievementsResponse struct {
	PlayerStats struct {
		SteamID  string            `json:"steamID"`
		GameName string            `json:"gameName"`
		Achievements []playerAchievement `json:"achievements"`
	} `json:"playerstats"`
}

type playerAchievement struct {
	APIName    string `json:"apiname"`
	Achieved   int    `json:"achieved"`
	UnlockTime int64  `json:"unlocktime"`
}

type schemaResponse struct {
	Game struct {
		GameName string `json:"gameName"`
		Stats    struct {
			Achievements []schemaAchievement `json:"achievements"`
		} `json:"availableGameStats"`
	} `json:"game"`
}

type schemaAchievement struct {
	Name         string `json:"name"`
	DisplayName  string `json:"displayName"`
	Description  string `json:"description"`
	Icon         string `json:"icon"`
	IconGray     string `json:"icongray"`
}

type globalAchievementResponse struct {
	AchievementPercentages struct {
		Achievements []globalAchievement `json:"achievements"`
	} `json:"achievementpercentages"`
}

type globalAchievement struct {
	Name    string  `json:"name"`
	Percent float64 `json:"percent"`
}

// --- Output types ---

type mediaItem struct {
	Type     string `json:"type"`
	URL      string `json:"url"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

type gameEntry struct {
	ExternalID      string      `json:"external_id"`
	Title           string      `json:"title"`
	Platform        string      `json:"platform,omitempty"`
	URL             string      `json:"url,omitempty"`
	Description     string      `json:"description,omitempty"`
	ReleaseDate     string      `json:"release_date,omitempty"`
	Genres          []string    `json:"genres,omitempty"`
	Developer       string      `json:"developer,omitempty"`
	Publisher       string      `json:"publisher,omitempty"`
	Media           []mediaItem `json:"media,omitempty"`
	PlaytimeMinutes int         `json:"playtime_minutes,omitempty"`
}

// --- Config loading ---

func loadConfig() (*steamConfig, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	var c steamConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.APIKey == "" {
		return nil, fmt.Errorf("config.json must contain api_key")
	}
	if c.SteamID == "" && c.VanityURL == "" {
		return nil, fmt.Errorf("config.json must contain steam_id or vanity_url")
	}
	return &c, nil
}

// --- Steam API calls ---

func resolveVanityURL(apiKey, vanityURL string) (string, error) {
	url := fmt.Sprintf("%s/ISteamUser/ResolveVanityURL/v1/?key=%s&vanityurl=%s",
		steamAPIBase, apiKey, vanityURL)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("vanity URL request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("vanity URL: status %d", resp.StatusCode)
	}

	var result vanityURLResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode vanity response: %w", err)
	}
	if result.Response.Success != 1 {
		return "", fmt.Errorf("vanity URL not found: %s", result.Response.Message)
	}
	return result.Response.SteamID, nil
}

func fetchOwnedGames(apiKey, steamID string) ([]ownedGame, error) {
	url := fmt.Sprintf(
		"%s/IPlayerService/GetOwnedGames/v1/?key=%s&steamid=%s&include_appinfo=true&include_played_free_games=true&format=json",
		steamAPIBase, apiKey, steamID,
	)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("owned games request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("unauthorized (invalid API key?)")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("owned games: status %d: %s", resp.StatusCode, string(body))
	}

	var result ownedGamesResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode owned games: %w", err)
	}
	return result.Response.Games, nil
}

func fetchAppDetails(appID int) (*appDetails, error) {
	<-rateLimiter.C

	url := fmt.Sprintf("%s/appdetails?appids=%d&l=english", storeAPIBase, appID)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("app details request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("app details: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var wrapper map[string]appDetailWrapper
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, fmt.Errorf("decode app details: %w", err)
	}

	key := fmt.Sprintf("%d", appID)
	entry, ok := wrapper[key]
	if !ok || !entry.Success {
		return nil, fmt.Errorf("no data for appid %d", appID)
	}
	return &entry.Data, nil
}

// --- Init ---

func handleInit() (any, *Error) {
	c, err := loadConfig()
	if err != nil {
		log.Printf("WARNING: %v — Steam source plugin will not be functional", err)
		return map[string]any{"status": "not_configured", "reason": err.Error()}, nil
	}

	cfg = *c

	if cfg.SteamID == "" && cfg.VanityURL != "" {
		log.Printf("resolving vanity URL %q...", cfg.VanityURL)
		steamID, err := resolveVanityURL(cfg.APIKey, cfg.VanityURL)
		if err != nil {
			return nil, &Error{Code: "AUTH_FAILED", Message: fmt.Sprintf("resolve vanity URL: %v", err)}
		}
		cfg.SteamID = steamID
		log.Printf("resolved Steam ID: %s", cfg.SteamID)
	}

	// Validate by fetching owned games count.
	games, err := fetchOwnedGames(cfg.APIKey, cfg.SteamID)
	if err != nil {
		return nil, &Error{Code: "AUTH_FAILED", Message: err.Error()}
	}

	log.Printf("Steam source plugin initialized: %d owned games for Steam ID %s", len(games), cfg.SteamID)
	return map[string]any{
		"status":    "ok",
		"steam_id":  cfg.SteamID,
		"game_count": len(games),
	}, nil
}

// --- Games list ---

func handleGamesList(params json.RawMessage) (any, *Error) {
	if cfg.APIKey == "" || cfg.SteamID == "" {
		return map[string]any{"games": []any{}}, nil
	}

	owned, err := fetchOwnedGames(cfg.APIKey, cfg.SteamID)
	if err != nil {
		return nil, &Error{Code: "API_ERROR", Message: err.Error()}
	}

	log.Printf("fetched %d owned games, enriching with store details...", len(owned))

	games := make([]gameEntry, 0, len(owned))
	for i, og := range owned {
		if og.Name == "" {
			continue
		}

		appIDStr := fmt.Sprintf("%d", og.AppID)
		entry := gameEntry{
			ExternalID:      appIDStr,
			Title:           og.Name,
			Platform:        "windows_pc",
			URL:             fmt.Sprintf("https://store.steampowered.com/app/%d", og.AppID),
			PlaytimeMinutes: og.PlaytimeForever,
		}

		if og.ImgIconURL != "" {
			entry.Media = append(entry.Media, mediaItem{
				Type: "icon",
				URL:  fmt.Sprintf("https://steamcdn-a.akamaihd.net/steamcommunity/public/images/apps/%d/%s.jpg", og.AppID, og.ImgIconURL),
			})
		}

		detail, err := fetchAppDetails(og.AppID)
		if err != nil {
			log.Printf("  [%d/%d] %s: detail fetch failed: %v", i+1, len(owned), og.Name, err)
		} else {
			if detail.Type != "game" && detail.Type != "demo" {
				continue
			}
			entry.Description = detail.ShortDescription
			entry.ReleaseDate = detail.ReleaseDate.Date
			if detail.HeaderImage != "" {
				entry.Media = append(entry.Media, mediaItem{Type: "cover", URL: detail.HeaderImage})
			}
			if len(detail.Developers) > 0 {
				entry.Developer = detail.Developers[0]
			}
			if len(detail.Publishers) > 0 {
				entry.Publisher = detail.Publishers[0]
			}
			for _, g := range detail.Genres {
				entry.Genres = append(entry.Genres, g.Description)
			}
			for _, ss := range detail.Screenshots {
				if ss.PathFull != "" {
					entry.Media = append(entry.Media, mediaItem{Type: "screenshot", URL: ss.PathFull})
				}
			}
			for _, mv := range detail.Movies {
				if mv.Webm.Max != "" {
					entry.Media = append(entry.Media, mediaItem{Type: "video", URL: mv.Webm.Max})
				}
			}
		}

		games = append(games, entry)
		if (i+1)%25 == 0 {
			log.Printf("  enriched %d/%d games", i+1, len(owned))
		}
	}

	log.Printf("returning %d games (filtered from %d owned)", len(games), len(owned))
	return map[string]any{"games": games}, nil
}

// --- Achievement fetching ---

func fetchPlayerAchievements(apiKey, steamID string, appID int) (*playerAchievementsResponse, error) {
	url := fmt.Sprintf("%s/ISteamUserStats/GetPlayerAchievements/v1/?key=%s&steamid=%s&appid=%d&l=english",
		steamAPIBase, apiKey, steamID, appID)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("player achievements request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("player achievements: status %d: %s", resp.StatusCode, string(body))
	}

	var result playerAchievementsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode player achievements: %w", err)
	}
	return &result, nil
}

func fetchAchievementSchema(apiKey string, appID int) (*schemaResponse, error) {
	url := fmt.Sprintf("%s/ISteamUserStats/GetSchemaForGame/v2/?key=%s&appid=%d&l=english",
		steamAPIBase, apiKey, appID)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("achievement schema request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("achievement schema: status %d: %s", resp.StatusCode, string(body))
	}

	var result schemaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode achievement schema: %w", err)
	}
	return &result, nil
}

func fetchGlobalAchievements(appID int) (map[string]float64, error) {
	url := fmt.Sprintf("%s/ISteamUserStats/GetGlobalAchievementPercentagesForApp/v2/?gameid=%d",
		steamAPIBase, appID)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("global achievements request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, nil // not all games support global stats
	}

	var result globalAchievementResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, nil
	}

	out := make(map[string]float64, len(result.AchievementPercentages.Achievements))
	for _, a := range result.AchievementPercentages.Achievements {
		out[a.Name] = a.Percent
	}
	return out, nil
}

type achievementEntry struct {
	ExternalID   string  `json:"external_id"`
	Title        string  `json:"title"`
	Description  string  `json:"description"`
	LockedIcon   string  `json:"locked_icon,omitempty"`
	UnlockedIcon string  `json:"unlocked_icon,omitempty"`
	Rarity       float64 `json:"rarity,omitempty"`
	Unlocked     bool    `json:"unlocked"`
	UnlockedAt   int64   `json:"unlocked_at,omitempty"`
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
	if cfg.APIKey == "" || cfg.SteamID == "" {
		return nil, &Error{Code: "NOT_CONFIGURED", Message: "steam source not configured"}
	}

	var appID int
	if _, err := fmt.Sscanf(p.ExternalGameID, "%d", &appID); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "external_game_id must be a numeric Steam app ID"}
	}

	schema, err := fetchAchievementSchema(cfg.APIKey, appID)
	if err != nil {
		return nil, &Error{Code: "API_ERROR", Message: fmt.Sprintf("schema: %v", err)}
	}
	if len(schema.Game.Stats.Achievements) == 0 {
		return map[string]any{
			"source":           "steam",
			"external_game_id": p.ExternalGameID,
			"total_count":      0,
			"unlocked_count":   0,
			"achievements":     []any{},
		}, nil
	}

	schemaMap := make(map[string]schemaAchievement, len(schema.Game.Stats.Achievements))
	for _, sa := range schema.Game.Stats.Achievements {
		schemaMap[sa.Name] = sa
	}

	playerResp, err := fetchPlayerAchievements(cfg.APIKey, cfg.SteamID, appID)
	if err != nil {
		log.Printf("player achievements unavailable for %d: %v", appID, err)
	}

	playerMap := make(map[string]playerAchievement)
	if playerResp != nil {
		for _, pa := range playerResp.PlayerStats.Achievements {
			playerMap[pa.APIName] = pa
		}
	}

	globalRarity, _ := fetchGlobalAchievements(appID)

	achievements := make([]achievementEntry, 0, len(schema.Game.Stats.Achievements))
	unlocked := 0
	for _, sa := range schema.Game.Stats.Achievements {
		entry := achievementEntry{
			ExternalID:   sa.Name,
			Title:        sa.DisplayName,
			Description:  sa.Description,
			LockedIcon:   sa.IconGray,
			UnlockedIcon: sa.Icon,
		}
		if r, ok := globalRarity[sa.Name]; ok {
			entry.Rarity = r
		}
		if pa, ok := playerMap[sa.Name]; ok && pa.Achieved == 1 {
			entry.Unlocked = true
			entry.UnlockedAt = pa.UnlockTime
			unlocked++
		}
		achievements = append(achievements, entry)
	}

	log.Printf("achievements for appid %d: %d/%d unlocked", appID, unlocked, len(achievements))

	return map[string]any{
		"source":           "steam",
		"external_game_id": p.ExternalGameID,
		"total_count":      len(achievements),
		"unlocked_count":   unlocked,
		"achievements":     achievements,
	}, nil
}

// --- Main ---

func main() {
	log.SetOutput(os.Stderr)
	log.Println("Steam game source plugin started")

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
				"plugin_id":      "game-source-steam",
				"plugin_version": "1.0.0",
				"capabilities":   []string{"source", "achievements"},
			}

		case "plugin.check_config":
			var params struct {
				Config map[string]any `json:"config"`
			}
			if err := json.Unmarshal(req.Params, &params); err == nil {
				key, _ := params.Config["api_key"].(string)
				sid, _ := params.Config["steam_id"].(string)
				vanity, _ := params.Config["vanity_url"].(string)
				if key == "" {
					resp.Result = map[string]any{"status": "error", "message": "api_key required"}
				} else if sid == "" && vanity == "" {
					resp.Result = map[string]any{"status": "error", "message": "steam_id or vanity_url required"}
				} else {
					testID := sid
					if testID == "" {
						resolved, err := resolveVanityURL(key, vanity)
						if err != nil {
							resp.Result = map[string]any{"status": "error", "message": err.Error()}
						} else {
							testID = resolved
						}
					}
					if testID != "" {
						_, err := fetchOwnedGames(key, testID)
						if err != nil {
							resp.Result = map[string]any{"status": "error", "message": err.Error()}
						} else {
							resp.Result = map[string]any{"status": "ok", "steam_id": testID}
						}
					}
				}
			} else {
				resp.Result = map[string]any{"status": "ok"}
			}

		case "source.games.list":
			result, errObj := handleGamesList(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
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

// filterType returns true if the app type represents a game.
func filterType(t string) bool {
	t = strings.ToLower(t)
	return t == "game" || t == "demo"
}
