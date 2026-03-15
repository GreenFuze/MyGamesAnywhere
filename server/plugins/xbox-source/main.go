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
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
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
	RedirectURI  string `json:"redirect_uri"`
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
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	var c xboxConfig
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.ClientID == "" || c.ClientSecret == "" {
		return nil, fmt.Errorf("config.json must contain client_id and client_secret")
	}
	if c.RedirectURI == "" {
		c.RedirectURI = "http://localhost:9090/auth/xbox/callback"
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

// Held during an active OAuth flow so the token exchange can use it.
var pkceVerifier string

// --------------- Microsoft OAuth ---------------

func buildAuthorizeURL(state string) string {
	pkceVerifier = generateCodeVerifier()
	params := url.Values{
		"client_id":             {cfg.ClientID},
		"response_type":        {"code"},
		"redirect_uri":         {cfg.RedirectURI},
		"scope":                {"XboxLive.signin XboxLive.offline_access offline_access"},
		"state":                {state},
		"response_mode":        {"query"},
		"code_challenge":       {codeChallenge(pkceVerifier)},
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

func exchangeCodeForToken(code string) (*msTokenResponse, error) {
	data := url.Values{
		"client_id":     {cfg.ClientID},
		"code":          {code},
		"redirect_uri":  {cfg.RedirectURI},
		"grant_type":    {"authorization_code"},
		"scope":         {"XboxLive.signin XboxLive.offline_access offline_access"},
		"code_verifier": {pkceVerifier},
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

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}

// runOAuthFlow starts a local HTTP server, opens the browser, and waits for the callback.
// Returns the authorization code.
func runOAuthFlow(ctx context.Context) (string, error) {
	u, err := url.Parse(cfg.RedirectURI)
	if err != nil {
		return "", fmt.Errorf("parse redirect URI: %w", err)
	}
	listenAddr := u.Host

	state := randomState()
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	mux.HandleFunc(u.Path, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			errCh <- fmt.Errorf("OAuth state mismatch")
			return
		}
		if errMsg := r.URL.Query().Get("error"); errMsg != "" {
			desc := r.URL.Query().Get("error_description")
			http.Error(w, "Auth error: "+errMsg, http.StatusBadRequest)
			errCh <- fmt.Errorf("OAuth error: %s: %s", errMsg, desc)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			http.Error(w, "no code in callback", http.StatusBadRequest)
			errCh <- fmt.Errorf("no code in OAuth callback")
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<html><body><h2>Xbox authentication successful!</h2><p>You can close this tab and return to MyGamesAnywhere.</p></body></html>`)
		codeCh <- code
	})

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return "", fmt.Errorf("listen on %s: %w", listenAddr, err)
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)
	defer srv.Shutdown(context.Background())

	authorizeURL := buildAuthorizeURL(state)
	log.Printf("=== XBOX AUTHENTICATION REQUIRED ===")
	log.Printf("Open this URL in your browser:")
	log.Printf("%s", authorizeURL)
	log.Printf("====================================")

	openBrowser(authorizeURL)

	select {
	case code := <-codeCh:
		return code, nil
	case err := <-errCh:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(300 * time.Second):
		return "", fmt.Errorf("OAuth flow timed out (300s)")
	}
}

// --------------- Ensure authenticated ---------------

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
			log.Printf("Xbox auth after refresh failed: %v, falling back to browser flow", err)
		} else {
			log.Printf("MS token refresh failed: %v, falling back to browser flow", err)
		}
	}

	// Full browser OAuth flow.
	tokenMu.Unlock()
	code, err := runOAuthFlow(ctx)
	tokenMu.Lock()
	if err != nil {
		return fmt.Errorf("OAuth flow: %w", err)
	}

	log.Println("exchanging auth code for MS token...")
	msResp, err := exchangeCodeForToken(code)
	if err != nil {
		return fmt.Errorf("exchange code: %w", err)
	}
	tokens.MSAccessToken = msResp.AccessToken
	tokens.MSRefreshToken = msResp.RefreshToken
	tokens.MSExpiresAt = time.Now().Add(time.Duration(msResp.ExpiresIn) * time.Second)

	tokenMu.Unlock()
	err = fullXboxAuth(msResp.AccessToken)
	tokenMu.Lock()
	if err != nil {
		return err
	}

	saveTokens()
	return nil
}

// --------------- Xbox Live API calls ---------------

func xboxAPIGet(ctx context.Context, apiURL string) ([]byte, error) {
	tokenMu.Lock()
	auth := fmt.Sprintf("XBL3.0 x=%s;%s", tokens.UserHash, tokens.XSTSToken)
	tokenMu.Unlock()

	req, _ := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	req.Header.Set("Authorization", auth)
	req.Header.Set("x-xbl-contract-version", "2")
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

type titleHubResponse struct {
	XUID   string  `json:"xuid"`
	Titles []title `json:"titles"`
}

type title struct {
	TitleID       string         `json:"titleId"`
	Name          string         `json:"name"`
	Type          string         `json:"type"`
	Devices       []string       `json:"devices"`
	DisplayImage  string         `json:"displayImage"`
	MediaItemType string         `json:"mediaItemType"`
	ModernTitleID string         `json:"modernTitleId"`
	IsBundle      bool           `json:"isBundle"`
	Achievement   *achievement   `json:"achievement"`
	GamePass      *gamePass      `json:"gamePass"`
	Images        []titleImage   `json:"images"`
	TitleHistory  *titleHistory  `json:"titleHistory"`
	Detail        *titleDetail   `json:"detail"`
}

type achievement struct {
	CurrentAchievements  int     `json:"currentAchievements"`
	TotalAchievements    int     `json:"totalAchievements"`
	CurrentGamerscore    int     `json:"currentGamerscore"`
	TotalGamerscore      int     `json:"totalGamerscore"`
	ProgressPercentage   float64 `json:"progressPercentage"`
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

// --------------- Output types ---------------

type mediaItem struct {
	Type     string `json:"type"`
	URL      string `json:"url"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

type gameEntry struct {
	ExternalID  string      `json:"external_id"`
	Title       string      `json:"title"`
	Platform    string      `json:"platform,omitempty"`
	URL         string      `json:"url,omitempty"`
	Description string      `json:"description,omitempty"`
	ReleaseDate string      `json:"release_date,omitempty"`
	Genres      []string    `json:"genres,omitempty"`
	Developer   string      `json:"developer,omitempty"`
	Publisher   string      `json:"publisher,omitempty"`
	Media       []mediaItem `json:"media,omitempty"`
	IsGamePass  bool        `json:"is_game_pass,omitempty"`
}

// --------------- Fetch title history ---------------

func fetchTitleHistory(ctx context.Context) (*titleHubResponse, error) {
	tokenMu.Lock()
	xuid := tokens.XUID
	tokenMu.Unlock()

	if xuid == "" {
		return nil, fmt.Errorf("no XUID available")
	}

	fields := "achievement,image,gamepass,detail"
	apiURL := fmt.Sprintf("%s/users/xuid(%s)/titles/titlehistory/decoration/%s?maxItems=1000",
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

func titleToGameEntry(t title) *gameEntry {
	if t.Name == "" || t.TitleID == "" {
		return nil
	}

	entry := &gameEntry{
		ExternalID: t.TitleID,
		Title:      t.Name,
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

// --------------- IPC handlers ---------------

func handleInit() (any, *Error) {
	c, err := loadConfig()
	if err != nil {
		log.Printf("WARNING: %v", err)
		return map[string]any{"status": "not_configured", "reason": err.Error()}, nil
	}
	cfg = *c

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	if err := ensureAuthenticated(ctx); err != nil {
		return nil, &Error{Code: "AUTH_FAILED", Message: err.Error()}
	}

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
			resp.Result = map[string]any{"status": "ok"}

		case "source.games.list":
			result, errObj := handleGamesList(req.Params)
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
