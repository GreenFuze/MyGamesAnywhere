package main

import (
	"encoding/base64"
	"encoding/binary"
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

// --------------- Epic constants ---------------

const (
	epicClientID     = "34a02cf8f4414e29b15921876da36f9a"
	epicClientSecret = "daafbccc737745039dffe53d94fc76cf"

	epicOAuthHost    = "account-public-service-prod03.ol.epicgames.com"
	epicCatalogHost  = "catalog-public-service-prod06.ol.epicgames.com"
	epicLibraryHost  = "library-service.live.use1a.on.epicgames.com"

	tokenFile    = "tokens.json"
	localPort    = "9091"
)

// --------------- Saved tokens ---------------

type savedTokens struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	AccountID    string    `json:"account_id"`
	DisplayName  string    `json:"display_name"`
	ExpiresAt    time.Time `json:"expires_at"`
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

// --------------- Epic OAuth ---------------

func basicAuthHeader() string {
	creds := epicClientID + ":" + epicClientSecret
	return "basic " + base64.StdEncoding.EncodeToString([]byte(creds))
}

type epicTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id"`
	DisplayName  string `json:"displayName"`
	ExpiresIn    int    `json:"expires_in"`
	ExpiresAt    string `json:"expires_at"`
	TokenType    string `json:"token_type"`
	ErrorCode    string `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
}

func epicTokenRequest(params url.Values) (*epicTokenResponse, error) {
	tokenURL := fmt.Sprintf("https://%s/account/api/oauth/token", epicOAuthHost)
	client := &http.Client{Timeout: 15 * time.Second}

	req, _ := http.NewRequest("POST", tokenURL, strings.NewReader(params.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", basicAuthHeader())

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result epicTokenResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode token response: %w", err)
	}
	if result.ErrorCode != "" {
		return nil, fmt.Errorf("Epic OAuth error: %s: %s", result.ErrorCode, result.ErrorMessage)
	}
	if result.AccessToken == "" {
		return nil, fmt.Errorf("no access token in Epic response: %s", string(body))
	}
	return &result, nil
}

func exchangeAuthCode(code string) (*epicTokenResponse, error) {
	return epicTokenRequest(url.Values{
		"grant_type": {"authorization_code"},
		"code":       {code},
		"token_type": {"eg1"},
	})
}

func refreshAccessToken(refreshToken string) (*epicTokenResponse, error) {
	return epicTokenRequest(url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"token_type":    {"eg1"},
	})
}

// applyTokenResponse updates the cached tokens. Caller must hold tokenMu.
func applyTokenResponse(tr *epicTokenResponse) {
	tokens.AccessToken = tr.AccessToken
	if tr.RefreshToken != "" {
		tokens.RefreshToken = tr.RefreshToken
	}
	tokens.AccountID = tr.AccountID
	tokens.DisplayName = tr.DisplayName
	if tr.ExpiresAt != "" {
		tokens.ExpiresAt, _ = time.Parse(time.RFC3339, tr.ExpiresAt)
	} else if tr.ExpiresIn > 0 {
		tokens.ExpiresAt = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	}
}

func verifyToken() error {
	verifyURL := fmt.Sprintf("https://%s/account/api/oauth/verify", epicOAuthHost)
	client := &http.Client{Timeout: 10 * time.Second}
	req, _ := http.NewRequest("GET", verifyURL, nil)
	req.Header.Set("Authorization", "bearer "+tokens.AccessToken)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("verify request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("token verify: status %d", resp.StatusCode)
	}
	return nil
}

// --------------- Browser OAuth flow ---------------

func getEpicLoginURL() string {
	redirectURL := fmt.Sprintf("https://www.epicgames.com/id/api/redirect?clientId=%s&responseType=code", epicClientID)
	return "https://www.epicgames.com/id/login?redirectUrl=" + url.QueryEscape(redirectURL)
}

func openBrowser(targetURL string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL)
	case "darwin":
		cmd = exec.Command("open", targetURL)
	default:
		cmd = exec.Command("xdg-open", targetURL)
	}
	cmd.Start()
}

const authPageHTML = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>Epic Games - MyGamesAnywhere Authentication</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; max-width: 600px; margin: 60px auto; padding: 0 20px; background: #121212; color: #e0e0e0; }
  h1 { color: #fff; }
  .step { background: #1e1e1e; border-radius: 8px; padding: 16px 20px; margin: 16px 0; border-left: 4px solid #0078f2; }
  .step-num { font-weight: bold; color: #0078f2; font-size: 1.1em; }
  a { color: #4db8ff; }
  input[type=text] { width: 100%%; padding: 12px; font-size: 16px; border: 1px solid #444; border-radius: 6px; background: #2a2a2a; color: #fff; box-sizing: border-box; }
  button { background: #0078f2; color: #fff; border: none; padding: 12px 24px; font-size: 16px; border-radius: 6px; cursor: pointer; margin-top: 8px; }
  button:hover { background: #0066d6; }
  .success { display: none; background: #1a3a1a; border: 1px solid #4caf50; border-radius: 8px; padding: 20px; text-align: center; }
  .success h2 { color: #4caf50; }
  code { background: #2a2a2a; padding: 2px 6px; border-radius: 3px; font-size: 0.9em; }
</style>
</head>
<body>
<h1>Epic Games Authentication</h1>
<div id="form-section">
  <div class="step">
    <span class="step-num">Step 1:</span> <a href="%s" target="_blank">Click here to sign in to Epic Games</a> (opens in new tab)
  </div>
  <div class="step">
    <span class="step-num">Step 2:</span> After signing in, you'll see a JSON page. Find the <code>"authorizationCode"</code> value and copy it.
  </div>
  <div class="step">
    <span class="step-num">Step 3:</span> Paste the authorization code below and click Submit.
    <br><br>
    <form method="POST" action="/auth/epic/code">
      <input type="text" name="code" placeholder="Paste authorization code here..." autofocus required>
      <br>
      <button type="submit">Submit</button>
    </form>
  </div>
</div>
<div class="success" id="success-section">
  <h2>Authentication Successful!</h2>
  <p>You can close this tab and return to MyGamesAnywhere.</p>
</div>
</body>
</html>`

func runOAuthFlow() (string, error) {
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	listenAddr := "localhost:" + localPort
	loginURL := getEpicLoginURL()

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, authPageHTML, loginURL)
	})

	mux.HandleFunc("/auth/epic/code", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		r.ParseForm()
		code := strings.TrimSpace(r.FormValue("code"))
		if code == "" {
			http.Error(w, "no code provided", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, `<!DOCTYPE html><html><head><style>body{font-family:sans-serif;max-width:600px;margin:60px auto;padding:0 20px;background:#121212;color:#e0e0e0;text-align:center;} h2{color:#4caf50;}</style></head><body><h2>Authentication Successful!</h2><p>You can close this tab and return to MyGamesAnywhere.</p></body></html>`)
		codeCh <- code
	})

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return "", fmt.Errorf("listen on %s: %w", listenAddr, err)
	}
	srv := &http.Server{Handler: mux}
	go srv.Serve(listener)
	defer srv.Shutdown(nil)

	localURL := "http://localhost:" + localPort
	log.Printf("=== EPIC GAMES AUTHENTICATION REQUIRED ===")
	log.Printf("Open this URL in your browser: %s", localURL)
	log.Printf("===========================================")

	openBrowser(localURL)

	select {
	case code := <-codeCh:
		return code, nil
	case err := <-errCh:
		return "", err
	case <-time.After(300 * time.Second):
		return "", fmt.Errorf("OAuth flow timed out (300s)")
	}
}

// --------------- Ensure authenticated ---------------

func ensureAuthenticated() error {
	tokenMu.Lock()
	defer tokenMu.Unlock()

	if tokens.AccessToken == "" {
		loadTokens()
	}

	// If token is still valid, verify it.
	if tokens.AccessToken != "" && time.Now().Before(tokens.ExpiresAt.Add(-5*time.Minute)) {
		if err := verifyToken(); err == nil {
			log.Printf("using cached Epic token (expires %s)", tokens.ExpiresAt.Format(time.RFC3339))
			return nil
		}
		log.Println("cached token verification failed, will refresh")
	}

	// Try refresh.
	if tokens.RefreshToken != "" {
		log.Println("refreshing Epic access token...")
		tr, err := refreshAccessToken(tokens.RefreshToken)
		if err == nil {
			applyTokenResponse(tr)
			saveTokens()
			log.Printf("refreshed Epic token for %s", tokens.DisplayName)
			return nil
		}
		log.Printf("refresh failed: %v, falling back to browser flow", err)
	}

	// Full browser flow.
	tokenMu.Unlock()
	code, err := runOAuthFlow()
	tokenMu.Lock()
	if err != nil {
		return fmt.Errorf("OAuth flow: %w", err)
	}

	log.Println("exchanging Epic auth code for token...")
	tr, err := exchangeAuthCode(code)
	if err != nil {
		return fmt.Errorf("exchange code: %w", err)
	}
	applyTokenResponse(tr)
	saveTokens()
	log.Printf("authenticated as %s (account %s)", tokens.DisplayName, tokens.AccountID)
	return nil
}

// --------------- Epic API calls ---------------

var rateLimiter = time.NewTicker(300 * time.Millisecond)

func epicAPIGet(apiURL string) ([]byte, error) {
	<-rateLimiter.C

	tokenMu.Lock()
	accessToken := tokens.AccessToken
	tokenMu.Unlock()

	client := &http.Client{Timeout: 15 * time.Second}
	req, _ := http.NewRequest("GET", apiURL, nil)
	req.Header.Set("Authorization", "bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Epic API GET %s: %w", apiURL, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Epic API %s: status %d: %s", apiURL, resp.StatusCode, string(body))
	}
	return body, nil
}

// --------------- Library types ---------------

type libraryResponse struct {
	Records          []libraryRecord  `json:"records"`
	ResponseMetadata responseMetadata `json:"responseMetadata"`
}

type libraryRecord struct {
	Namespace     string `json:"namespace"`
	CatalogItemID string `json:"catalogItemId"`
	AppName       string `json:"appName"`
}

type responseMetadata struct {
	NextCursor string `json:"nextCursor"`
}

// --------------- Catalog types ---------------

type catalogItem struct {
	ID               string           `json:"id"`
	Title            string           `json:"title"`
	Description      string           `json:"description"`
	LongDescription  string           `json:"longDescription"`
	Namespace        string           `json:"namespace"`
	Developer        string           `json:"developer"`
	Categories       []categoryInfo   `json:"categories"`
	KeyImages        []keyImageInfo   `json:"keyImages"`
	ReleaseInfo      []releaseInfo    `json:"releaseInfo"`
	DlcItemList      []any            `json:"dlcItemList"`
	MainGameItem     *mainGameRef     `json:"mainGameItem"`
}

type categoryInfo struct {
	Path string `json:"path"`
}

type keyImageInfo struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type releaseInfo struct {
	AppID    string   `json:"appId"`
	Platform []string `json:"platform"`
}

type mainGameRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// --------------- Output types ---------------

type gameEntry struct {
	ExternalID     string   `json:"external_id"`
	Title          string   `json:"title"`
	Platform       string   `json:"platform,omitempty"`
	URL            string   `json:"url,omitempty"`
	Description    string   `json:"description,omitempty"`
	ReleaseDate    string   `json:"release_date,omitempty"`
	Genres         []string `json:"genres,omitempty"`
	Developer      string   `json:"developer,omitempty"`
	Publisher      string   `json:"publisher,omitempty"`
	CoverURL       string   `json:"cover_url,omitempty"`
	ScreenshotURLs []string `json:"screenshot_urls,omitempty"`
	VideoURLs      []string `json:"video_urls,omitempty"`
}

// --------------- Fetch library ---------------

func fetchLibrary() ([]libraryRecord, error) {
	var allRecords []libraryRecord
	cursor := ""

	for {
		apiURL := fmt.Sprintf("https://%s/library/api/public/items?includeMetadata=true", epicLibraryHost)
		if cursor != "" {
			apiURL += "&cursor=" + url.QueryEscape(cursor)
		}

		body, err := epicAPIGet(apiURL)
		if err != nil {
			return nil, err
		}

		var resp libraryResponse
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decode library response: %w", err)
		}

		allRecords = append(allRecords, resp.Records...)
		if resp.ResponseMetadata.NextCursor == "" {
			break
		}
		cursor = resp.ResponseMetadata.NextCursor
	}

	return allRecords, nil
}

func fetchCatalogItem(namespace, catalogItemID string) (*catalogItem, error) {
	apiURL := fmt.Sprintf("https://%s/catalog/api/shared/namespace/%s/bulk/items?id=%s&includeDLCDetails=true&includeMainGameDetails=true&country=US&locale=en",
		epicCatalogHost, namespace, catalogItemID)

	body, err := epicAPIGet(apiURL)
	if err != nil {
		return nil, err
	}

	var items map[string]catalogItem
	if err := json.Unmarshal(body, &items); err != nil {
		return nil, fmt.Errorf("decode catalog response: %w", err)
	}

	item, ok := items[catalogItemID]
	if !ok {
		return nil, fmt.Errorf("catalog item %s not found in response", catalogItemID)
	}
	return &item, nil
}

func catalogToGameEntry(rec libraryRecord, item *catalogItem) *gameEntry {
	entry := &gameEntry{
		ExternalID: rec.AppName,
		Title:      item.Title,
		Platform:   "windows_pc",
		Developer:  item.Developer,
	}

	if item.Description != "" {
		entry.Description = item.Description
	} else if item.LongDescription != "" {
		entry.Description = item.LongDescription
	}

	for _, cat := range item.Categories {
		parts := strings.Split(cat.Path, "/")
		if len(parts) > 0 {
			genre := parts[len(parts)-1]
			genre = strings.ReplaceAll(genre, "-", " ")
			if genre != "" && genre != "games" && genre != "addons" && genre != "applications" {
				entry.Genres = append(entry.Genres, genre)
			}
		}
	}

	for _, img := range item.KeyImages {
		switch img.Type {
		case "DieselStoreFrontWide", "OfferImageWide", "DieselGameBoxTall", "Thumbnail":
			if entry.CoverURL == "" {
				entry.CoverURL = img.URL
			}
		case "DieselStoreFrontTall", "OfferImageTall", "CodeRedemption_340x440":
			if entry.CoverURL == "" {
				entry.CoverURL = img.URL
			}
		case "Screenshot":
			entry.ScreenshotURLs = append(entry.ScreenshotURLs, img.URL)
		}
	}

	if entry.CoverURL == "" {
		for _, img := range item.KeyImages {
			if img.URL != "" {
				entry.CoverURL = img.URL
				break
			}
		}
	}

	if rec.AppName != "" {
		entry.URL = fmt.Sprintf("https://store.epicgames.com/en-US/p/%s", rec.AppName)
	}

	return entry
}

// --------------- IPC handlers ---------------

func handleInit() (any, *Error) {
	if err := ensureAuthenticated(); err != nil {
		return nil, &Error{Code: "AUTH_FAILED", Message: err.Error()}
	}

	return map[string]any{
		"status":       "ok",
		"account_id":   tokens.AccountID,
		"display_name": tokens.DisplayName,
	}, nil
}

func handleGamesList(params json.RawMessage) (any, *Error) {
	tokenMu.Lock()
	hasToken := tokens.AccessToken != ""
	tokenMu.Unlock()

	if !hasToken {
		return map[string]any{"games": []any{}}, nil
	}

	if err := ensureAuthenticated(); err != nil {
		return nil, &Error{Code: "AUTH_FAILED", Message: err.Error()}
	}

	records, err := fetchLibrary()
	if err != nil {
		return nil, &Error{Code: "API_ERROR", Message: err.Error()}
	}
	log.Printf("fetched %d library records, enriching from catalog...", len(records))

	var games []gameEntry
	for i, rec := range records {
		if rec.CatalogItemID == "" || rec.Namespace == "" {
			continue
		}

		item, err := fetchCatalogItem(rec.Namespace, rec.CatalogItemID)
		if err != nil {
			log.Printf("  [%d/%d] catalog fetch failed for %s: %v", i+1, len(records), rec.AppName, err)
			continue
		}

		// Skip DLC (items that reference a main game).
		if item.MainGameItem != nil {
			continue
		}

		entry := catalogToGameEntry(rec, item)
		if entry.Title == "" {
			continue
		}

		games = append(games, *entry)

		if (i+1)%25 == 0 {
			log.Printf("  enriched %d/%d records", i+1, len(records))
		}
	}

	log.Printf("returning %d games (filtered from %d records)", len(games), len(records))
	return map[string]any{"games": games}, nil
}

// --------------- Main ---------------

func main() {
	log.SetOutput(os.Stderr)
	log.Println("Epic game source plugin started")

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
				"plugin_id":      "game-source-epic",
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
