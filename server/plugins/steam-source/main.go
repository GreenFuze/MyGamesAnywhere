package main

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
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
	APIKey  string `json:"api_key"`
	SteamID string `json:"steam_id"` // resolved via Steam OpenID login
	// RefreshToken is the long-lived Steam token obtained by scanning a QR
	// challenge with the Steam mobile app. It is the credential that lets MGA
	// read the Steam Families shared library (ADR-0033): MGA mints short-lived
	// access tokens from it on demand. Absent or rejected tokens degrade to
	// owned-games-only; MGA never handles a Steam password.
	RefreshToken string `json:"refresh_token"`
}

const (
	steamAPIBase   = "https://api.steampowered.com"
	configFile     = "config.json"
	steamOpenIDURL = "https://steamcommunity.com/openid/login"
)

// storeAPIBase is a var (not const) so tests can point appdetails enrichment at
// an httptest server.
var storeAPIBase = "https://store.steampowered.com/api"

var errNoAchievementSchema = errors.New("steam achievement schema is unavailable")
var fetchAchievementSchemaBaseURL = steamAPIBase

// errNoFamilyGroup means the account is not a member of any Steam Family group,
// so there is no shared library to enumerate. This is a normal, non-error state.
var errNoFamilyGroup = errors.New("steam account is not in a family group")

// errAccessTokenRejected means Steam rejected the stored app-approved token.
// Owned games still list; the shared portion degrades and the integration is
// flagged as needing a refreshed token (ADR-0033).
var errAccessTokenRejected = errors.New("steam access token was rejected (expired or invalid)")

// steamFamilyAPIBase is overridable in tests to point IFamilyGroupsService
// calls at an httptest server.
var steamFamilyAPIBase = steamAPIBase
var steamProfileAPIBase = steamAPIBase

type providerIdentity struct {
	Provider    string `json:"provider"`
	Subject     string `json:"subject"`
	DisplayName string `json:"display_name,omitempty"`
	AvatarURL   string `json:"avatar_url,omitempty"`
}

// oauthPending tracks OpenID state tokens for CSRF validation.
var oauthPending = map[string]bool{}

func randomState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

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

type appDetailWrapper struct {
	Success bool       `json:"success"`
	Data    appDetails `json:"data"`
}

type appDetails struct {
	Type             string           `json:"type"`
	Name             string           `json:"name"`
	AppID            int              `json:"steam_appid"`
	ShortDescription string           `json:"short_description"`
	HeaderImage      string           `json:"header_image"`
	Developers       []string         `json:"developers"`
	Publishers       []string         `json:"publishers"`
	Metacritic       *metacriticInfo  `json:"metacritic"`
	Genres           []genreInfo      `json:"genres"`
	Screenshots      []screenshotInfo `json:"screenshots"`
	Movies           []movieInfo      `json:"movies"`
	ReleaseDate      releaseDateInfo  `json:"release_date"`
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
		SteamID      string              `json:"steamID"`
		GameName     string              `json:"gameName"`
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
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	IconGray    string `json:"icongray"`
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
	// Shared marks a Steam Families borrowed title (ADR-0033). SharedOwner is
	// the lending account's SteamID for attribution.
	Shared      bool   `json:"shared,omitempty"`
	SharedOwner string `json:"shared_owner,omitempty"`
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

	return &c, nil
}

func configFromMap(config map[string]any) steamConfig {
	result := steamConfig{}
	if apiKey := configString(config, "api_key"); apiKey != "" {
		result.APIKey = apiKey
	}
	if steamID := configString(config, "steam_id"); steamID != "" {
		result.SteamID = steamID
	}
	if refreshToken := configString(config, "refresh_token"); refreshToken != "" {
		result.RefreshToken = refreshToken
	}
	return result
}

func configFromDirectParams(params json.RawMessage) (steamConfig, *Error) {
	if len(strings.TrimSpace(string(params))) == 0 {
		return configFromMap(nil), nil
	}
	var config map[string]any
	if err := json.Unmarshal(params, &config); err != nil {
		return steamConfig{}, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	return configFromMap(config), nil
}

// --- Steam API calls ---

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

func fetchSteamIdentity(apiKey, steamID, fallbackName string) providerIdentity {
	identity := providerIdentity{
		Provider:    "steam",
		Subject:     steamID,
		DisplayName: strings.TrimSpace(fallbackName),
	}
	if strings.TrimSpace(apiKey) == "" || strings.TrimSpace(steamID) == "" {
		return identity
	}

	requestURL := fmt.Sprintf(
		"%s/ISteamUser/GetPlayerSummaries/v2/?key=%s&steamids=%s",
		steamProfileAPIBase,
		url.QueryEscape(apiKey),
		url.QueryEscape(steamID),
	)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(requestURL)
	if err != nil {
		log.Printf("steam profile lookup failed: %v", err)
		return identity
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("steam profile lookup returned status %d", resp.StatusCode)
		return identity
	}

	var result struct {
		Response struct {
			Players []struct {
				SteamID     string `json:"steamid"`
				PersonaName string `json:"personaname"`
				AvatarFull  string `json:"avatarfull"`
			} `json:"players"`
		} `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Printf("decode steam profile: %v", err)
		return identity
	}
	if len(result.Response.Players) == 0 || result.Response.Players[0].SteamID != steamID {
		return identity
	}
	player := result.Response.Players[0]
	if name := strings.TrimSpace(player.PersonaName); name != "" {
		identity.DisplayName = name
	}
	identity.AvatarURL = strings.TrimSpace(player.AvatarFull)
	return identity
}

// --- Steam Families (shared library) API ---

type familyGroupResponse struct {
	Response struct {
		FamilyGroupID         string `json:"family_groupid"`
		IsNotMemberOfAnyGroup bool   `json:"is_not_member_of_any_group"`
	} `json:"response"`
}

type sharedLibraryResponse struct {
	Response struct {
		Apps []sharedApp `json:"apps"`
	} `json:"response"`
}

type sharedApp struct {
	AppID         int      `json:"appid"`
	Name          string   `json:"name"`
	OwnerSteamIDs []string `json:"owner_steamids"`
	ExcludeReason int      `json:"exclude_reason"`
}

// steamFamilyClient issues authenticated IFamilyGroupsService requests using a
// player-supplied webapi_token. Absent tokens never reach here.
func steamFamilyGet(path string, query url.Values) (*http.Response, error) {
	requestURL := fmt.Sprintf("%s/%s/?%s", steamFamilyAPIBase, path, query.Encode())
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(requestURL)
	if err != nil {
		return nil, err
	}
	// A short-lived webapi_token that has expired returns 401/403. Surface this
	// as a distinct, recoverable condition so the owned scan is unaffected.
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		resp.Body.Close()
		return nil, errAccessTokenRejected
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp, nil
}

// fetchFamilyGroupID resolves the caller's Steam Family group id. Returns
// errNoFamilyGroup when the account belongs to no group.
func fetchFamilyGroupID(accessToken, steamID string) (string, error) {
	query := url.Values{"access_token": {accessToken}, "steamid": {steamID}}
	resp, err := steamFamilyGet("IFamilyGroupsService/GetFamilyGroupForUser/v1", query)
	if err != nil {
		return "", fmt.Errorf("family group lookup: %w", err)
	}
	defer resp.Body.Close()

	var result familyGroupResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode family group: %w", err)
	}
	if result.Response.IsNotMemberOfAnyGroup || strings.TrimSpace(result.Response.FamilyGroupID) == "" {
		return "", errNoFamilyGroup
	}
	return result.Response.FamilyGroupID, nil
}

// fetchSharedLibraryApps lists apps shared into the family library by other
// members. Apps the caller already owns are excluded via include_own=false;
// each returned app is attributed to an owning SteamID other than the caller.
func fetchSharedLibraryApps(accessToken, familyGroupID, selfSteamID string) ([]sharedApp, error) {
	query := url.Values{
		"access_token":   {accessToken},
		"family_groupid": {familyGroupID},
		"include_own":    {"false"},
		"include_free":   {"false"},
	}
	resp, err := steamFamilyGet("IFamilyGroupsService/GetSharedLibraryApps/v1", query)
	if err != nil {
		return nil, fmt.Errorf("shared library apps: %w", err)
	}
	defer resp.Body.Close()

	var result sharedLibraryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode shared library: %w", err)
	}

	shared := make([]sharedApp, 0, len(result.Response.Apps))
	for _, app := range result.Response.Apps {
		if app.AppID <= 0 || strings.TrimSpace(app.Name) == "" {
			continue
		}
		// Defensive: only treat an app as shared when a lender other than the
		// caller owns it. Guards against include_own being ignored upstream.
		if !isSharedFromOther(app, selfSteamID) {
			continue
		}
		shared = append(shared, app)
	}
	return shared, nil
}

// isSharedFromOther reports whether the app is owned by someone other than the
// caller (i.e. genuinely borrowed).
func isSharedFromOther(app sharedApp, selfSteamID string) bool {
	if len(app.OwnerSteamIDs) == 0 {
		return false
	}
	for _, owner := range app.OwnerSteamIDs {
		if strings.TrimSpace(owner) != "" && owner != selfSteamID {
			return true
		}
	}
	return false
}

// sharedOwnerLabel returns a lender SteamID to attribute a borrowed title to.
func sharedOwnerLabel(app sharedApp, selfSteamID string) string {
	for _, owner := range app.OwnerSteamIDs {
		if strings.TrimSpace(owner) != "" && owner != selfSteamID {
			return owner
		}
	}
	return ""
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
	// Player credentials are supplied only by the active profile integration.
	// Plugin startup must not inspect or validate a process-wide account.
	return map[string]any{"status": "ok", "message": "connection sign-in required"}, nil
}

// --- Games list ---

func handleGamesList(params json.RawMessage) (any, *Error) {
	effectiveCfg, errObj := configFromDirectParams(params)
	if errObj != nil {
		return nil, errObj
	}
	if effectiveCfg.APIKey == "" {
		return nil, &Error{Code: "NOT_CONFIGURED", Message: "steam source requires api_key before it can scan games"}
	}
	if effectiveCfg.SteamID == "" {
		return nil, &Error{Code: "AUTH_REQUIRED", Message: "steam source requires Steam login before it can scan games"}
	}

	owned, err := fetchOwnedGames(effectiveCfg.APIKey, effectiveCfg.SteamID)
	if err != nil {
		return nil, &Error{Code: "API_ERROR", Message: err.Error()}
	}

	log.Printf("fetched %d owned games, enriching with store details...", len(owned))

	games := make([]gameEntry, 0, len(owned))
	ownedAppIDs := make(map[int]bool, len(owned))
	for i, og := range owned {
		ownedAppIDs[og.AppID] = true
		if og.Name == "" {
			continue
		}

		base := gameEntry{
			ExternalID:      fmt.Sprintf("%d", og.AppID),
			Title:           og.Name,
			Platform:        "windows_pc",
			URL:             fmt.Sprintf("https://store.steampowered.com/app/%d", og.AppID),
			PlaytimeMinutes: og.PlaytimeForever,
		}
		if og.ImgIconURL != "" {
			base.Media = append(base.Media, mediaItem{
				Type: "icon",
				URL:  fmt.Sprintf("https://steamcdn-a.akamaihd.net/steamcommunity/public/images/apps/%d/%s.jpg", og.AppID, og.ImgIconURL),
			})
		}

		entry, keep := enrichEntry(base, og.AppID)
		if !keep {
			continue
		}
		games = append(games, entry)
		if (i+1)%25 == 0 {
			log.Printf("  enriched %d/%d games", i+1, len(owned))
		}
	}

	result := map[string]any{}

	// Steam Families shared library (ADR-0033). Only attempted when the player
	// completed app-approved sign-in. Any failure here degrades to owned-games-only and
	// never fails the scan; an expired token is reported so the integration can
	// flag that a refreshed token is needed.
	if effectiveCfg.RefreshToken != "" {
		shared, sharedErr := fetchSharedGames(effectiveCfg, ownedAppIDs)
		if sharedErr != nil {
			if errors.Is(sharedErr, errNoFamilyGroup) {
				log.Printf("no Steam Family group for this account; shared library skipped")
			} else {
				log.Printf("shared library unavailable (owned games unaffected): %v", sharedErr)
				result["shared_library_error"] = sharedErr.Error()
				if errors.Is(sharedErr, errAccessTokenRejected) {
					result["shared_library_token_expired"] = true
				}
			}
		} else {
			log.Printf("adding %d shared (family) games", len(shared))
			games = append(games, shared...)
		}
	}

	log.Printf("returning %d games (owned=%d)", len(games), len(owned))
	result["games"] = games
	return result, nil
}

// fetchSharedGames resolves and enriches the Steam Families shared library for
// the configured account. Apps already owned are skipped.
func fetchSharedGames(cfg steamConfig, ownedAppIDs map[int]bool) ([]gameEntry, error) {
	// Mint a short-lived access token from the stored refresh token. A rejected
	// refresh token surfaces as errAccessTokenRejected so the caller degrades to
	// owned-games-only and flags that the player should sign in again.
	accessToken, err := newSteamAuthClient().AccessTokenFor(cfg.RefreshToken, cfg.SteamID)
	if err != nil {
		return nil, err
	}

	familyGroupID, err := fetchFamilyGroupID(accessToken, cfg.SteamID)
	if err != nil {
		return nil, err
	}
	apps, err := fetchSharedLibraryApps(accessToken, familyGroupID, cfg.SteamID)
	if err != nil {
		return nil, err
	}

	shared := make([]gameEntry, 0, len(apps))
	for _, app := range apps {
		if ownedAppIDs[app.AppID] {
			continue
		}
		base := gameEntry{
			ExternalID:  fmt.Sprintf("%d", app.AppID),
			Title:       app.Name,
			Platform:    "windows_pc",
			URL:         fmt.Sprintf("https://store.steampowered.com/app/%d", app.AppID),
			Shared:      true,
			SharedOwner: sharedOwnerLabel(app, cfg.SteamID),
		}
		entry, keep := enrichEntry(base, app.AppID)
		if !keep {
			continue
		}
		shared = append(shared, entry)
	}
	return shared, nil
}

// enrichEntry augments a base game entry with Steam store detail. It preserves
// the base entry's identity and shared attribution. Returns keep=false when the
// store classifies the app as something other than a game/demo (e.g. DLC, tool).
func enrichEntry(entry gameEntry, appID int) (gameEntry, bool) {
	detail, err := fetchAppDetails(appID)
	if err != nil {
		log.Printf("  %s: detail fetch failed: %v", entry.Title, err)
		return entry, true
	}
	if detail.Type != "game" && detail.Type != "demo" {
		return entry, false
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
	return entry, true
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
		fetchAchievementSchemaBaseURL, apiKey, appID)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("achievement schema request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusBadRequest && strings.TrimSpace(string(body)) == "{}" {
			return nil, errNoAchievementSchema
		}
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

func buildSteamAchievementEntries(
	schemaAchievements []schemaAchievement,
	playerMap map[string]playerAchievement,
	globalRarity map[string]float64,
) ([]achievementEntry, int) {
	achievements := make([]achievementEntry, 0, len(schemaAchievements))
	unlocked := 0
	for _, sa := range schemaAchievements {
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
	return achievements, unlocked
}

func handleAchievementsGet(params json.RawMessage) (any, *Error) {
	var p struct {
		ExternalGameID string         `json:"external_game_id"`
		Config         map[string]any `json:"config"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}
	if p.ExternalGameID == "" {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "external_game_id required"}
	}
	effectiveCfg := configFromMap(p.Config)
	if effectiveCfg.APIKey == "" || effectiveCfg.SteamID == "" {
		return nil, &Error{Code: "NOT_CONFIGURED", Message: "steam source not configured"}
	}

	var appID int
	if _, err := fmt.Sscanf(p.ExternalGameID, "%d", &appID); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "external_game_id must be a numeric Steam app ID"}
	}

	schema, err := fetchAchievementSchema(effectiveCfg.APIKey, appID)
	if err != nil {
		if errors.Is(err, errNoAchievementSchema) {
			log.Printf("steam achievements unavailable for appid %d: no public achievement schema", appID)
			return map[string]any{
				"source":           "steam",
				"external_game_id": p.ExternalGameID,
				"total_count":      0,
				"unlocked_count":   0,
				"achievements":     []any{},
			}, nil
		}
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

	playerResp, err := fetchPlayerAchievements(effectiveCfg.APIKey, effectiveCfg.SteamID, appID)
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

	achievements, unlocked := buildSteamAchievementEntries(schema.Game.Stats.Achievements, playerMap, globalRarity)

	log.Printf("achievements for appid %d: %d/%d unlocked", appID, unlocked, len(achievements))

	return map[string]any{
		"source":           "steam",
		"external_game_id": p.ExternalGameID,
		"total_count":      len(achievements),
		"unlocked_count":   unlocked,
		"achievements":     achievements,
	}, nil
}

// --- Config check (Add Integration wizard) ---

func handleCheckConfig(params json.RawMessage) (any, *Error) {
	var p struct {
		Config      map[string]any `json:"config"`
		RedirectURI string         `json:"redirect_uri"`
		ForceOAuth  bool           `json:"force_oauth"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}

	// Validate API key is present.
	apiKey, _ := p.Config["api_key"].(string)
	if apiKey == "" {
		return map[string]any{"status": "error", "message": "api_key required"}, nil
	}

	steamID := configString(p.Config, "steam_id")
	if steamID != "" && !p.ForceOAuth {
		// Verify the key works with this steam ID.
		_, err := fetchOwnedGames(apiKey, steamID)
		if err != nil {
			return map[string]any{"status": "error", "message": err.Error()}, nil
		}
		return steamAuthOKResponse(steamID), nil
	}

	// No Steam identity yet — redirect user to Steam OpenID login.
	state := randomState()
	oauthPending[state] = true

	// Append state to redirect URI so OAuthController can route it back to us.
	returnTo := p.RedirectURI
	if strings.Contains(returnTo, "?") {
		returnTo += "&state=" + state
	} else {
		returnTo += "?state=" + state
	}

	// Extract realm (scheme + host) from redirect URI.
	parsed, err := url.Parse(p.RedirectURI)
	if err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: "invalid redirect_uri"}
	}
	realm := parsed.Scheme + "://" + parsed.Host

	// Build Steam OpenID URL.
	openIDParams := url.Values{
		"openid.ns":         {"http://specs.openid.net/auth/2.0"},
		"openid.mode":       {"checkid_setup"},
		"openid.return_to":  {returnTo},
		"openid.realm":      {realm},
		"openid.identity":   {"http://specs.openid.net/auth/2.0/identifier_select"},
		"openid.claimed_id": {"http://specs.openid.net/auth/2.0/identifier_select"},
	}
	authorizeURL := steamOpenIDURL + "?" + openIDParams.Encode()

	return map[string]any{
		"status":        "oauth_required",
		"authorize_url": authorizeURL,
		"state":         state,
	}, nil
}

// --- QR sign-in (auth.qr.begin / auth.qr.poll) ---

// handleQRBegin starts a Steam QR sign-in and returns the challenge to show.
func handleQRBegin(json.RawMessage) (any, *Error) {
	session, err := newSteamAuthClient().BeginQRSession()
	if err != nil {
		return nil, &Error{Code: "AUTH_ERROR", Message: err.Error()}
	}
	return map[string]any{
		"status":           "pending",
		"client_id":        session.ClientID,
		"request_id":       session.RequestID,
		"challenge_url":    session.ChallengeURL,
		"interval_seconds": session.IntervalSecs,
	}, nil
}

// handleQRPoll reports whether the player approved the QR challenge. On success
// it returns config_updates so the server persists the refresh token and the
// signed-in Steam identity.
func handleQRPoll(params json.RawMessage) (any, *Error) {
	var p struct {
		ClientID  string `json:"client_id"`
		RequestID string `json:"request_id"`
		APIKey    string `json:"api_key"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}

	client := newSteamAuthClient()
	refreshToken, accountName, steamID, err := client.PollQRSession(p.ClientID, p.RequestID)
	switch {
	case errors.Is(err, errAuthPending):
		return map[string]any{"status": "pending"}, nil
	case errors.Is(err, errAuthSessionExpired):
		return nil, &Error{Code: "AUTH_EXPIRED", Message: err.Error()}
	case err != nil:
		return nil, &Error{Code: "AUTH_ERROR", Message: err.Error()}
	}

	// Bind the connection to the account proven by the approved token. Never
	// reuse a previously typed SteamID: the player may deliberately approve a
	// different account while correcting a connection.
	identity := fetchSteamIdentity(strings.TrimSpace(p.APIKey), steamID, accountName)
	updates := map[string]any{
		"refresh_token":     refreshToken,
		"steam_id":          steamID,
		"provider_identity": identity,
	}

	log.Printf("steam QR sign-in completed for SteamID %s", steamID)
	return map[string]any{
		"status":            "ok",
		"account_name":      identity.DisplayName,
		"provider_identity": identity,
		"config_updates":    updates,
	}, nil
}

func configString(config map[string]any, key string) string {
	value, _ := config[key].(string)
	return strings.TrimSpace(value)
}

func steamAuthOKResponse(steamID string) map[string]any {
	return map[string]any{
		"status":          "ok",
		"steam_id":        steamID,
		"source_identity": steamID,
		"config_updates": map[string]any{
			"steam_id": steamID,
		},
	}
}

// --- OAuth callback (Steam OpenID return) ---

func handleOAuthCallback(params json.RawMessage) (any, *Error) {
	var p struct {
		State  string            `json:"state"`
		Params map[string]string `json:"params"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}

	// Verify state for CSRF protection.
	if !oauthPending[p.State] {
		return nil, &Error{Code: "INVALID_STATE", Message: "unknown or expired state token"}
	}
	delete(oauthPending, p.State)

	// Check for user cancellation.
	if p.Params["openid.mode"] == "cancel" {
		return nil, &Error{Code: "AUTH_CANCELLED", Message: "user cancelled Steam login"}
	}

	// Extract Steam64 ID from openid.claimed_id.
	// Format: https://steamcommunity.com/openid/id/76561198012345678
	claimedID := p.Params["openid.claimed_id"]
	if claimedID == "" {
		return nil, &Error{Code: "AUTH_FAILED", Message: "missing openid.claimed_id"}
	}

	const prefix = "https://steamcommunity.com/openid/id/"
	if !strings.HasPrefix(claimedID, prefix) {
		return nil, &Error{Code: "AUTH_FAILED", Message: "unexpected claimed_id format"}
	}

	steamID := strings.TrimPrefix(claimedID, prefix)
	if steamID == "" {
		return nil, &Error{Code: "AUTH_FAILED", Message: "empty steam ID in claimed_id"}
	}

	log.Printf("Steam OpenID login successful: Steam ID %s", steamID)
	return steamAuthOKResponse(steamID), nil
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
				"plugin_version": "1.3.1",
				"capabilities":   []string{"source", "achievements"},
			}

		case "plugin.check_config":
			result, errObj := handleCheckConfig(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}

		case "auth.oauth.callback":
			result, errObj := handleOAuthCallback(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}

		case "auth.qr.begin":
			result, errObj := handleQRBegin(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
			}

		case "auth.qr.poll":
			result, errObj := handleQRPoll(req.Params)
			if errObj != nil {
				resp.Error = errObj
			} else {
				resp.Result = result
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
