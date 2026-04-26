package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// --------------- IPC protocol ---------------

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

// --------------- Config ---------------

type xboxConfig struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	// XBLMarket is sent as x-xbl-market (default US). Optional.
	XBLMarket string `json:"xbl_market,omitempty"`
	// PlayLaunchLocale is the xbox.com path segment for play URLs, e.g. en-US.
	PlayLaunchLocale string `json:"play_launch_locale,omitempty"`
}

const (
	configFile = "config.json"
	tokenFile  = "tokens.json"

	msAuthorizeURL = "https://login.microsoftonline.com/consumers/oauth2/v2.0/authorize"
	msTokenURL     = "https://login.microsoftonline.com/consumers/oauth2/v2.0/token"

	xblAuthURL  = "https://user.auth.xboxlive.com/user/authenticate"
	xstsAuthURL = "https://xsts.auth.xboxlive.com/xsts/authorize"

	titlehubURL = "https://titlehub.xboxlive.com"
)

// Injected at build time via -ldflags "-X main.builtinClientID=... -X main.builtinClientSecret=..."
// These are the MGA app registration credentials, NOT user credentials.
var (
	builtinClientID     string
	builtinClientSecret string
)

var cfg xboxConfig

// --------------- Saved tokens ---------------

type savedTokens struct {
	MSAccessToken  string    `json:"ms_access_token"`
	MSRefreshToken string    `json:"ms_refresh_token"`
	MSExpiresAt    time.Time `json:"ms_expires_at"`
	XSTSToken      string    `json:"xsts_token"`
	UserHash       string    `json:"user_hash"`
	XUID           string    `json:"xuid"`
	XSTSExpiresAt  time.Time `json:"xsts_expires_at"`
}

var tokens savedTokens
var tokenMu sync.Mutex

func loadTokens() error {
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &tokens)
}

func saveTokens() error {
	data, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tokenFile, data, 0600)
}

// --------------- Config loading ---------------

func loadConfig() (*xboxConfig, error) {
	// Start with built-in credentials (injected via ldflags at compile time).
	c := xboxConfig{
		ClientID:     builtinClientID,
		ClientSecret: builtinClientSecret,
	}

	// Overlay with config.json if present (allows local dev override).
	data, err := os.ReadFile(configFile)
	if err == nil {
		if err := json.Unmarshal(data, &c); err != nil {
			return nil, err
		}
	}

	if c.ClientID == "" || c.ClientSecret == "" {
		return nil, fmt.Errorf("no client credentials: set via build-time ldflags or config.json")
	}
	return &c, nil
}

// --------------- PKCE helpers ---------------

func generateCodeVerifier() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func codeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// oauthPending stores PKCE verifiers keyed by OAuth state parameter.
// Protected by tokenMu.
var oauthPending = map[string]string{}

// --------------- Microsoft OAuth ---------------

func buildAuthorizeURL(state string, redirectURI string) string {
	verifier := generateCodeVerifier()

	// Store verifier keyed by state so the callback can retrieve it.
	tokenMu.Lock()
	oauthPending[state] = verifier
	tokenMu.Unlock()

	params := url.Values{
		"client_id":             {cfg.ClientID},
		"response_type":         {"code"},
		"redirect_uri":          {redirectURI},
		"scope":                 {"XboxLive.signin XboxLive.offline_access offline_access"},
		"state":                 {state},
		"response_mode":         {"query"},
		"code_challenge":        {codeChallenge(verifier)},
		"code_challenge_method": {"S256"},
	}
	return msAuthorizeURL + "?" + params.Encode()
}

type msTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func exchangeCodeForToken(code, verifier, redirectURI string) (*msTokenResponse, error) {
	data := url.Values{
		"client_id":     {cfg.ClientID},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
		"scope":         {"XboxLive.signin XboxLive.offline_access offline_access"},
		"code_verifier": {verifier},
	}
	return postMSToken(data)
}

func refreshMSToken(refreshToken string) (*msTokenResponse, error) {
	data := url.Values{
		"client_id":     {cfg.ClientID},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
		"scope":         {"XboxLive.signin XboxLive.offline_access offline_access"},
	}
	return postMSToken(data)
}

func postMSToken(data url.Values) (*msTokenResponse, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.PostForm(msTokenURL, data)
	if err != nil {
		return nil, fmt.Errorf("MS token request: %w", err)
	}
	defer resp.Body.Close()

	var result msTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode MS token: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("MS OAuth error: %s: %s", result.Error, result.ErrorDesc)
	}
	if result.AccessToken == "" {
		return nil, fmt.Errorf("no access token in MS response")
	}
	return &result, nil
}

// --------------- Xbox Live token exchange ---------------

type xblAuthResponse struct {
	Token         string `json:"Token"`
	DisplayClaims struct {
		Xui []struct {
			Uhs string `json:"uhs"`
			Xid string `json:"xid"`
			Gtg string `json:"gtg"`
		} `json:"xui"`
	} `json:"DisplayClaims"`
	IssueInstant string `json:"IssueInstant"`
	NotAfter     string `json:"NotAfter"`
}

func getXBLToken(msAccessToken string) (*xblAuthResponse, error) {
	// Azure AD apps must prefix with "d="
	rpsTicket := "d=" + msAccessToken

	body := map[string]any{
		"Properties": map[string]any{
			"AuthMethod": "RPS",
			"SiteName":   "user.auth.xboxlive.com",
			"RpsTicket":  rpsTicket,
		},
		"RelyingParty": "http://auth.xboxlive.com",
		"TokenType":    "JWT",
	}
	return postXboxAuth(xblAuthURL, body)
}

func getXSTSToken(xblToken string) (*xblAuthResponse, error) {
	body := map[string]any{
		"Properties": map[string]any{
			"SandboxId":  "RETAIL",
			"UserTokens": []string{xblToken},
		},
		"RelyingParty": "http://xboxlive.com",
		"TokenType":    "JWT",
	}
	return postXboxAuth(xstsAuthURL, body)
}

func postXboxAuth(endpoint string, body any) (*xblAuthResponse, error) {
	payload, _ := json.Marshal(body)
	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("POST", endpoint, strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xbox auth request to %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("xbox auth %s: status %d: %s", endpoint, resp.StatusCode, string(respBody))
	}

	var result xblAuthResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode xbox auth: %w", err)
	}
	return &result, nil
}

// fullXboxAuth does the complete MS token -> XBL token -> XSTS token chain.
func fullXboxAuth(msAccessToken string) error {
	log.Println("exchanging MS token for Xbox Live user token...")
	xblResp, err := getXBLToken(msAccessToken)
	if err != nil {
		return fmt.Errorf("XBL auth: %w", err)
	}
	log.Println("got XBL token, exchanging for XSTS token...")

	xstsResp, err := getXSTSToken(xblResp.Token)
	if err != nil {
		return fmt.Errorf("XSTS auth: %w", err)
	}

	tokenMu.Lock()
	tokens.XSTSToken = xstsResp.Token
	if len(xstsResp.DisplayClaims.Xui) > 0 {
		tokens.UserHash = xstsResp.DisplayClaims.Xui[0].Uhs
		tokens.XUID = xstsResp.DisplayClaims.Xui[0].Xid
	}
	if xstsResp.NotAfter != "" {
		tokens.XSTSExpiresAt, _ = time.Parse(time.RFC3339, xstsResp.NotAfter)
	}
	tokenMu.Unlock()

	log.Printf("XSTS auth complete. XUID=%s, expires=%s", tokens.XUID, tokens.XSTSExpiresAt.Format(time.RFC3339))
	return nil
}

// --------------- OAuth browser flow ---------------

func randomState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// --------------- Ensure authenticated (refresh only) ---------------

// ensureAuthenticated checks cached tokens and tries a silent refresh.
// If refresh fails, returns an error — the frontend must trigger re-auth via check_config.
func ensureAuthenticated(ctx context.Context) error {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	// Try loading saved tokens.
	if tokens.XSTSToken == "" {
		loadTokens()
	}

	// If XSTS token is still valid, we're good.
	if tokens.XSTSToken != "" && time.Now().Before(tokens.XSTSExpiresAt.Add(-5*time.Minute)) {
		log.Printf("using cached XSTS token (expires %s)", tokens.XSTSExpiresAt.Format(time.RFC3339))
		return nil
	}

	// Try refreshing MS token if we have a refresh token.
	if tokens.MSRefreshToken != "" {
		log.Println("XSTS token expired, refreshing MS token...")
		msResp, err := refreshMSToken(tokens.MSRefreshToken)
		if err == nil {
			tokens.MSAccessToken = msResp.AccessToken
			if msResp.RefreshToken != "" {
				tokens.MSRefreshToken = msResp.RefreshToken
			}
			tokens.MSExpiresAt = time.Now().Add(time.Duration(msResp.ExpiresIn) * time.Second)

			tokenMu.Unlock()
			err = fullXboxAuth(msResp.AccessToken)
			tokenMu.Lock()
			if err == nil {
				saveTokens()
				return nil
			}
			log.Printf("Xbox auth after refresh failed: %v", err)
		} else {
			log.Printf("MS token refresh failed: %v", err)
		}
	}

	return fmt.Errorf("not authenticated — re-auth required via OAuth flow")
}

// --------------- Xbox Live API calls ---------------

func xboxAPIGet(ctx context.Context, apiURL string) ([]byte, error) {
	tokenMu.Lock()
	auth := fmt.Sprintf("XBL3.0 x=%s;%s", tokens.UserHash, tokens.XSTSToken)
	tokenMu.Unlock()

	req, _ := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	req.Header.Set("Authorization", auth)
	req.Header.Set("x-xbl-contract-version", "2")
	market := strings.TrimSpace(cfg.XBLMarket)
	if market == "" {
		market = "US"
	}
	req.Header.Set("x-xbl-market", market)
	req.Header.Set("x-xbl-client-name", "XboxApp")
	req.Header.Set("x-xbl-client-type", "UWA")
	req.Header.Set("x-xbl-client-version", "39.39.22001.0")
	req.Header.Set("Accept-Language", "en-US")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("xbox API GET %s: %w", apiURL, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("xbox API %s: status %d: %s", apiURL, resp.StatusCode, string(body))
	}
	return body, nil
}

// --------------- Titlehub types ---------------

// titleIDString unmarshals Title Hub titleId whether the API sends a JSON number or string.
type titleIDString string

func (t *titleIDString) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		*t = ""
		return nil
	}
	if s[0] == '"' {
		var unq string
		if err := json.Unmarshal(b, &unq); err != nil {
			return err
		}
		*t = titleIDString(strings.TrimSpace(unq))
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err == nil {
		*t = titleIDString(strings.TrimSpace(string(n)))
		return nil
	}
	var f float64
	if err := json.Unmarshal(b, &f); err == nil {
		*t = titleIDString(strconv.FormatInt(int64(f), 10))
		return nil
	}
	return fmt.Errorf("titleId: unexpected JSON %s", s)
}

type titleHubResponse struct {
	XUID   string  `json:"xuid"`
	Titles []title `json:"titles"`
}

type title struct {
	TitleID       titleIDString `json:"titleId"`
	Name          string        `json:"name"`
	Type          string        `json:"type"`
	Devices       []string      `json:"devices"`
	DisplayImage  string        `json:"displayImage"`
	MediaItemType string        `json:"mediaItemType"`
	ModernTitleID string        `json:"modernTitleId"`
	IsBundle      bool          `json:"isBundle"`
	IsStreamable  bool          `json:"isStreamable"`
	ProductID     string        `json:"productId"`
	Achievement   *achievement  `json:"achievement"`
	GamePass      *gamePass     `json:"gamePass"`
	Images        []titleImage  `json:"images"`
	TitleHistory  *titleHistory `json:"titleHistory"`
	Detail        *titleDetail  `json:"detail"`
}

type achievement struct {
	CurrentAchievements int     `json:"currentAchievements"`
	TotalAchievements   int     `json:"totalAchievements"`
	CurrentGamerscore   int     `json:"currentGamerscore"`
	TotalGamerscore     int     `json:"totalGamerscore"`
	ProgressPercentage  float64 `json:"progressPercentage"`
}

type gamePass struct {
	IsGamePass bool `json:"isGamePass"`
}

type titleImage struct {
	URL  string `json:"url"`
	Type string `json:"type"`
}

type titleHistory struct {
	LastTimePlayed string `json:"lastTimePlayed"`
	Visible        bool   `json:"visible"`
}

type titleDetail struct {
	Description      string   `json:"description"`
	DeveloperName    string   `json:"developerName"`
	PublisherName    string   `json:"publisherName"`
	Genres           []string `json:"genres"`
	ReleaseDate      string   `json:"releaseDate"`
	ShortDescription string   `json:"shortDescription"`
}

// --------------- Xbox Achievements API types ---------------

type xboxAchievementsResponse struct {
	Achievements []xboxAchievement `json:"achievements"`
	PagingInfo   struct {
		ContinuationToken *string `json:"continuationToken"`
		TotalRecords      int     `json:"totalRecords"`
	} `json:"pagingInfo"`
}

type xboxAchievement struct {
	ID                string                 `json:"id"`
	Name              string                 `json:"name"`
	Description       string                 `json:"description"`
	LockedDescription string                 `json:"lockedDescription"`
	IsSecret          bool                   `json:"isSecret"`
	ProgressState     string                 `json:"progressState"`
	Rarity            *xboxAchievementRarity `json:"rarity"`
	Rewards           []xboxReward           `json:"rewards"`
	Progression       *xboxProgression       `json:"progression"`
	MediaAssets       []xboxMediaAsset       `json:"mediaAssets"`
}

type xboxAchievementRarity struct {
	CurrentCategory   string  `json:"currentCategory"`
	CurrentPercentage float64 `json:"currentPercentage"`
}

type xboxReward struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Value       string `json:"value"`
	ValueType   string `json:"valueType"`
}

type xboxProgression struct {
	Requirements []xboxRequirement `json:"requirements"`
	TimeUnlocked string            `json:"timeUnlocked"`
}

type xboxRequirement struct {
	ID      string `json:"id"`
	Current string `json:"current"`
	Target  string `json:"target"`
}

type xboxMediaAsset struct {
	Name string `json:"name"`
	Type string `json:"type"`
	URL  string `json:"url"`
}

// --------------- Output types ---------------

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
	IsGamePass      bool        `json:"is_game_pass,omitempty"`
	XcloudAvailable bool        `json:"xcloud_available,omitempty"`
	StoreProductID  string      `json:"store_product_id,omitempty"`
	XcloudURL       string      `json:"xcloud_url,omitempty"`
}

// --------------- Fetch title history ---------------

func fetchTitleHistory(ctx context.Context) (*titleHubResponse, error) {
	tokenMu.Lock()
	xuid := tokens.XUID
	tokenMu.Unlock()

	if xuid == "" {
		return nil, fmt.Errorf("no XUID available")
	}

	// Decorations: legacy enrichment + ProductId/TitleHistory for store id, play history, and isStreamable
	// (xCloud availability). filterTo=IsStreamable,IsGame matches the xbox.com play client contract.
	// We omit supportedPlatform=StreamableOnly so the library is not limited to streamable-only titles.
	fields := "achievement,image,gamepass,detail,ProductId,TitleHistory"
	apiURL := fmt.Sprintf("%s/users/xuid(%s)/titles/titlehistory/decoration/%s?filterTo=IsStreamable,IsGame&maxItems=1000",
		titlehubURL, xuid, fields)

	body, err := xboxAPIGet(ctx, apiURL)
	if err != nil {
		return nil, err
	}

	var result titleHubResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode title history: %w", err)
	}
	return &result, nil
}

func xboxPlaySlug(name string) string {
	replacer := strings.NewReplacer("™", "", "®", "", "©", "", ":", "", "'", "", "’", "", "–", "-", "—", "-")
	s := strings.ToLower(strings.TrimSpace(replacer.Replace(name)))
	var b strings.Builder
	lastHyphen := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		case r == ' ', r == '-', r == '_', r == '.':
			if b.Len() > 0 && !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		default:
			// drop other punctuation
		}
	}
	out := strings.Trim(b.String(), "-")
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	return out
}

func launchableStoreProductID(id string) bool {
	id = strings.TrimSpace(strings.ToUpper(id))
	if len(id) < 2 {
		return false
	}
	p := id[:2]
	return p == "9P" || p == "9N" || p == "9M" || p == "9W"
}

func buildXcloudPlayURL(locale, slug, productID string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		locale = "en-US"
	}
	slug = strings.Trim(strings.TrimSpace(slug), "-")
	productID = strings.TrimSpace(productID)
	if slug == "" || productID == "" {
		return ""
	}
	return fmt.Sprintf("https://www.xbox.com/%s/play/launch/%s/%s", locale, slug, productID)
}

func titleToGameEntry(t title) *gameEntry {
	tid := strings.TrimSpace(string(t.TitleID))
	if t.Name == "" || tid == "" {
		return nil
	}

	entry := &gameEntry{
		ExternalID:      tid,
		Title:           t.Name,
		XcloudAvailable: t.IsStreamable,
	}
	if pid := strings.TrimSpace(t.ProductID); pid != "" {
		entry.StoreProductID = pid
	}
	if t.IsStreamable && launchableStoreProductID(entry.StoreProductID) {
		if slug := xboxPlaySlug(t.Name); slug != "" {
			entry.XcloudURL = buildXcloudPlayURL(cfg.PlayLaunchLocale, slug, entry.StoreProductID)
		}
	}

	if t.DisplayImage != "" {
		entry.Media = append(entry.Media, mediaItem{Type: "cover", URL: t.DisplayImage})
	}

	platform := detectPlatform(t.Devices)
	entry.Platform = platform

	if t.GamePass != nil && t.GamePass.IsGamePass {
		entry.IsGamePass = true
	}

	if t.Detail != nil {
		if t.Detail.Description != "" {
			entry.Description = t.Detail.Description
		} else if t.Detail.ShortDescription != "" {
			entry.Description = t.Detail.ShortDescription
		}
		entry.Developer = t.Detail.DeveloperName
		entry.Publisher = t.Detail.PublisherName
		entry.Genres = t.Detail.Genres
		if t.Detail.ReleaseDate != "" {
			entry.ReleaseDate = t.Detail.ReleaseDate
		}
	}

	for _, img := range t.Images {
		switch img.Type {
		case "BoxArt":
			entry.Media = append(entry.Media, mediaItem{Type: "cover", URL: img.URL})
		case "Poster", "BrandedKeyArt", "SuperHeroArt":
			entry.Media = append(entry.Media, mediaItem{Type: "artwork", URL: img.URL})
		case "Screenshot":
			entry.Media = append(entry.Media, mediaItem{Type: "screenshot", URL: img.URL})
		case "Logo":
			entry.Media = append(entry.Media, mediaItem{Type: "logo", URL: img.URL})
		}
	}

	return entry
}

func detectPlatform(devices []string) string {
	for _, d := range devices {
		switch strings.ToLower(d) {
		case "win32", "pc":
			return "windows_pc"
		case "xboxone":
			return "xbox_one"
		case "xboxseries", "scarlett":
			return "xbox_series"
		case "xbox360":
			return "xbox_360"
		}
	}
	if len(devices) > 0 {
		return "xbox_" + strings.ToLower(devices[0])
	}
	return "xbox"
}

// --------------- Xbox Achievements fetch ---------------

func fetchXboxAchievements(ctx context.Context, titleID string) ([]xboxAchievement, error) {
	tokenMu.Lock()
	xuid := tokens.XUID
	tokenMu.Unlock()

	if xuid == "" {
		return nil, fmt.Errorf("no XUID available")
	}

	apiURL := fmt.Sprintf(
		"https://achievements.xboxlive.com/users/xuid(%s)/achievements?titleId=%s&maxItems=1000",
		xuid, titleID,
	)

	body, err := xboxAPIGet(ctx, apiURL)
	if err != nil {
		return nil, err
	}

	var result xboxAchievementsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode xbox achievements: %w", err)
	}
	return result.Achievements, nil
}

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

func buildXboxAchievementEntries(xboxAchs []xboxAchievement) ([]achievementEntry, int, int, int) {
	achievements := make([]achievementEntry, 0, len(xboxAchs))
	unlocked := 0
	totalPoints := 0
	earnedPoints := 0

	for _, xa := range xboxAchs {
		entry := achievementEntry{
			ExternalID:  xa.ID,
			Title:       xa.Name,
			Description: xa.Description,
		}

		for _, ma := range xa.MediaAssets {
			if ma.Type == "Icon" && ma.URL != "" {
				entry.UnlockedIcon = ma.URL
				break
			}
		}

		if xa.Rarity != nil {
			entry.Rarity = xa.Rarity.CurrentPercentage
		}

		points := 0
		for _, rw := range xa.Rewards {
			if rw.ValueType == "Int" && rw.Type == "Gamerscore" {
				fmt.Sscanf(rw.Value, "%d", &points)
			}
		}
		entry.Points = points
		totalPoints += points

		isUnlocked, unlockedAt := xboxAchievementUnlocked(xa)
		if unlockedAt != "" {
			entry.UnlockedAt = unlockedAt
		}
		entry.Unlocked = isUnlocked
		if isUnlocked {
			unlocked++
			earnedPoints += points
		}

		achievements = append(achievements, entry)
	}

	return achievements, unlocked, totalPoints, earnedPoints
}

func xboxAchievementUnlocked(xa xboxAchievement) (bool, string) {
	if strings.EqualFold(strings.TrimSpace(xa.ProgressState), "Achieved") {
		return true, normalizedXboxUnlockedAt(xa.Progression)
	}
	if xa.Progression == nil {
		return false, ""
	}
	unlockedAt := normalizedXboxUnlockedAt(xa.Progression)
	return unlockedAt != "", unlockedAt
}

func normalizedXboxUnlockedAt(progression *xboxProgression) string {
	if progression == nil {
		return ""
	}
	trimmed := strings.TrimSpace(progression.TimeUnlocked)
	if trimmed == "" {
		return ""
	}
	parsed, err := time.Parse(time.RFC3339Nano, trimmed)
	if err != nil || parsed.IsZero() {
		return ""
	}
	return parsed.UTC().Format(time.RFC3339Nano)
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := ensureAuthenticated(ctx); err != nil {
		return nil, &Error{Code: "AUTH_FAILED", Message: err.Error()}
	}

	xboxAchs, err := fetchXboxAchievements(ctx, p.ExternalGameID)
	if err != nil {
		return nil, &Error{Code: "API_ERROR", Message: err.Error()}
	}

	achievements, unlocked, totalPoints, earnedPoints := buildXboxAchievementEntries(xboxAchs)

	log.Printf("achievements for Xbox title %s: %d/%d unlocked, %d/%d gamerscore",
		p.ExternalGameID, unlocked, len(achievements), earnedPoints, totalPoints)

	return map[string]any{
		"source":           "xbox",
		"external_game_id": p.ExternalGameID,
		"total_count":      len(achievements),
		"unlocked_count":   unlocked,
		"total_points":     totalPoints,
		"earned_points":    earnedPoints,
		"achievements":     achievements,
	}, nil
}

// --------------- IPC handlers ---------------

func handleInit() (any, *Error) {
	c, err := loadConfig()
	if err != nil {
		log.Printf("WARNING: %v", err)
		return map[string]any{"status": "not_configured", "reason": err.Error()}, nil
	}
	cfg = *c

	// Try loading cached tokens (non-blocking). Auth happens via check_config + OAuth callback.
	loadTokens()

	return map[string]any{
		"status": "ok",
		"xuid":   tokens.XUID,
	}, nil
}

func handleGamesList(params json.RawMessage) (any, *Error) {
	tokenMu.Lock()
	hasToken := tokens.XSTSToken != ""
	tokenMu.Unlock()

	if !hasToken {
		return map[string]any{"games": []any{}}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Re-auth if XSTS is expired.
	if err := ensureAuthenticated(ctx); err != nil {
		return nil, &Error{Code: "AUTH_FAILED", Message: err.Error()}
	}

	history, err := fetchTitleHistory(ctx)
	if err != nil {
		return nil, &Error{Code: "API_ERROR", Message: err.Error()}
	}

	log.Printf("fetched %d titles from Xbox title history", len(history.Titles))

	var games []gameEntry
	for _, t := range history.Titles {
		entry := titleToGameEntry(t)
		if entry == nil {
			continue
		}
		games = append(games, *entry)
	}

	log.Printf("returning %d games (filtered from %d titles)", len(games), len(history.Titles))
	return map[string]any{"games": games}, nil
}

// --------------- check_config handler ---------------

func handleCheckConfig(params json.RawMessage) (any, *Error) {
	var p struct {
		Config      xboxConfig `json:"config"`
		RedirectURI string     `json:"redirect_uri"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}

	// Check for valid cached XSTS tokens.
	tokenMu.Lock()
	if tokens.XSTSToken == "" {
		loadTokens()
	}
	hasValid := tokens.XSTSToken != "" && time.Now().Before(tokens.XSTSExpiresAt.Add(-5*time.Minute))
	tokenMu.Unlock()

	if hasValid {
		return map[string]any{"status": "ok"}, nil
	}

	// Try silent refresh if we have a refresh token.
	tokenMu.Lock()
	hasRefresh := tokens.MSRefreshToken != ""
	refreshTok := tokens.MSRefreshToken
	tokenMu.Unlock()

	if hasRefresh {
		msResp, err := refreshMSToken(refreshTok)
		if err == nil {
			tokenMu.Lock()
			tokens.MSAccessToken = msResp.AccessToken
			if msResp.RefreshToken != "" {
				tokens.MSRefreshToken = msResp.RefreshToken
			}
			tokens.MSExpiresAt = time.Now().Add(time.Duration(msResp.ExpiresIn) * time.Second)
			tokenMu.Unlock()

			if err := fullXboxAuth(msResp.AccessToken); err == nil {
				tokenMu.Lock()
				saveTokens()
				tokenMu.Unlock()
				return map[string]any{"status": "ok"}, nil
			}
		}
	}

	// OAuth consent required. Build authorize URL with PKCE.
	state := randomState()
	authorizeURL := buildAuthorizeURL(state, p.RedirectURI)

	return map[string]any{
		"status":        "oauth_required",
		"authorize_url": authorizeURL,
		"state":         state,
	}, nil
}

// --------------- auth.oauth.callback handler ---------------

func handleOAuthCallback(params json.RawMessage) (any, *Error) {
	var p struct {
		Code        string `json:"code"`
		State       string `json:"state"`
		RedirectURI string `json:"redirect_uri"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &Error{Code: "INVALID_PARAMS", Message: err.Error()}
	}

	// Look up the PKCE verifier for this state.
	tokenMu.Lock()
	verifier, ok := oauthPending[p.State]
	if ok {
		delete(oauthPending, p.State)
	}
	tokenMu.Unlock()

	if !ok {
		return nil, &Error{Code: "INVALID_STATE", Message: "no pending OAuth flow for state: " + p.State}
	}

	// Exchange authorization code for MS tokens.
	msResp, err := exchangeCodeForToken(p.Code, verifier, p.RedirectURI)
	if err != nil {
		return nil, &Error{Code: "TOKEN_EXCHANGE_FAILED", Message: err.Error()}
	}

	tokenMu.Lock()
	tokens.MSAccessToken = msResp.AccessToken
	tokens.MSRefreshToken = msResp.RefreshToken
	tokens.MSExpiresAt = time.Now().Add(time.Duration(msResp.ExpiresIn) * time.Second)
	tokenMu.Unlock()

	// Complete the Xbox Live auth chain: MS token -> XBL -> XSTS.
	if err := fullXboxAuth(msResp.AccessToken); err != nil {
		return nil, &Error{Code: "XBOX_AUTH_FAILED", Message: err.Error()}
	}

	tokenMu.Lock()
	saveTokens()
	xuid := tokens.XUID
	tokenMu.Unlock()

	log.Printf("OAuth callback complete, XUID=%s", xuid)
	return map[string]any{"status": "ok", "xuid": xuid}, nil
}

// --------------- Main ---------------

func main() {
	log.SetOutput(os.Stderr)
	log.Println("Xbox game source plugin started")

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
				"plugin_id":      "game-source-xbox",
				"plugin_version": "1.0.0",
				"capabilities":   []string{"source"},
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
