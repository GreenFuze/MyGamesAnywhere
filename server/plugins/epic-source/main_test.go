package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestEpicProfileIsolationIgnoresLegacyTokenFile(t *testing.T) {
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDir) })
	if err := os.WriteFile(tokenFile, []byte(`{"access_token":"legacy-global","refresh_token":"legacy-refresh"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	tokenMu.Lock()
	tokens = savedTokens{}
	tokenMu.Unlock()

	if _, errObj := handleInit(); errObj != nil {
		t.Fatalf("handleInit() error = %+v", errObj)
	}
	if _, errObj := handleGamesList(json.RawMessage(`{}`)); errObj == nil || errObj.Code != "AUTH_REQUIRED" {
		t.Fatalf("handleGamesList() error = %+v, want AUTH_REQUIRED without connection tokens", errObj)
	}
	tokenMu.Lock()
	accessToken := tokens.AccessToken
	tokenMu.Unlock()
	if accessToken != "" {
		t.Fatalf("legacy process token was loaded: %q", accessToken)
	}
}

func TestEpicTokenConfigRoundTrip(t *testing.T) {
	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	config := map[string]any{"tokens": savedTokens{
		AccessToken: "profile-access", RefreshToken: "profile-refresh", AccountID: "orr", ExpiresAt: expires,
	}}
	configured := tokensFromConfig(config)
	if configured == nil || configured.AccessToken != "profile-access" || configured.AccountID != "orr" || !configured.ExpiresAt.Equal(expires) {
		t.Fatalf("tokensFromConfig() = %+v", configured)
	}
}

func TestEpicCheckConfigRequiresExplicitConnectionSignIn(t *testing.T) {
	result, errObj := handleCheckConfig(json.RawMessage(`{"config":{}}`))
	if errObj != nil {
		t.Fatalf("handleCheckConfig() error = %+v", errObj)
	}
	payload, ok := result.(map[string]any)
	if !ok || payload["status"] != "error" {
		t.Fatalf("handleCheckConfig() = %#v, want actionable sign-in error", result)
	}
}

func ipcCall(stdin io.Writer, stdout io.Reader, method string, params any) (*Response, error) {
	req := Request{
		ID:     fmt.Sprintf("test-%d", time.Now().UnixNano()),
		Method: method,
	}
	if params != nil {
		b, _ := json.Marshal(params)
		req.Params = b
	}
	payload, _ := json.Marshal(req)

	if err := binary.Write(stdin, binary.BigEndian, uint32(len(payload))); err != nil {
		return nil, fmt.Errorf("write length: %w", err)
	}
	if _, err := stdin.Write(payload); err != nil {
		return nil, fmt.Errorf("write payload: %w", err)
	}

	var respLen uint32
	if err := binary.Read(stdout, binary.BigEndian, &respLen); err != nil {
		return nil, fmt.Errorf("read resp length: %w", err)
	}
	respData := make([]byte, respLen)
	if _, err := io.ReadFull(stdout, respData); err != nil {
		return nil, fmt.Errorf("read resp payload: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(respData, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &resp, nil
}

func TestEpicSourcePlugin(t *testing.T) {
	if os.Getenv("EPIC_SOURCE_INTEGRATION") != "1" {
		t.Skip("set EPIC_SOURCE_INTEGRATION=1 to run")
	}

	exePath := "./epic-source.exe"
	if _, err := os.Stat(exePath); err != nil {
		t.Fatalf("build the plugin first: go build -o epic-source.exe .")
	}

	cmd := exec.Command(exePath)
	cmd.Dir, _ = os.Getwd()
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		stdin.Close()
		cmd.Wait()
	}()

	t.Log("calling plugin.init (may open browser for Epic auth)...")
	resp, err := ipcCall(stdin, stdout, "plugin.init", nil)
	if err != nil {
		t.Fatalf("plugin.init: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("plugin.init error: %s: %s", resp.Error.Code, resp.Error.Message)
	}
	t.Logf("init result: %v", resp.Result)

	t.Log("calling source.games.list...")
	resp, err = ipcCall(stdin, stdout, "source.games.list", nil)
	if err != nil {
		t.Fatalf("source.games.list: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("source.games.list error: %s: %s", resp.Error.Code, resp.Error.Message)
	}

	resultBytes, _ := json.Marshal(resp.Result)
	var result struct {
		Games []gameEntry `json:"games"`
	}
	json.Unmarshal(resultBytes, &result)

	t.Logf("=== Epic Game Source Results ===")
	t.Logf("Total games returned: %d", len(result.Games))

	withDesc := 0
	withGenres := 0
	withDev := 0
	withMedia := 0

	for _, g := range result.Games {
		if g.Description != "" {
			withDesc++
		}
		if len(g.Genres) > 0 {
			withGenres++
		}
		if g.Developer != "" {
			withDev++
		}
		if len(g.Media) > 0 {
			withMedia++
		}
	}

	t.Logf("\nEnrichment coverage:")
	t.Logf("  Description:  %d/%d (%.0f%%)", withDesc, len(result.Games), pct(withDesc, len(result.Games)))
	t.Logf("  Genres:       %d/%d (%.0f%%)", withGenres, len(result.Games), pct(withGenres, len(result.Games)))
	t.Logf("  Developer:    %d/%d (%.0f%%)", withDev, len(result.Games), pct(withDev, len(result.Games)))
	t.Logf("  Media:        %d/%d (%.0f%%)", withMedia, len(result.Games), pct(withMedia, len(result.Games)))

	t.Logf("\nSample games (first 10):")
	count := 10
	if len(result.Games) < count {
		count = len(result.Games)
	}
	for i := 0; i < count; i++ {
		g := result.Games[i]
		t.Logf("  [%d] %s (id=%s)", i+1, g.Title, g.ExternalID)
		t.Logf("      Developer: %s", g.Developer)
		t.Logf("      Genres: %v", g.Genres)
		t.Logf("      Media items: %d", len(g.Media))
	}

	if len(result.Games) == 0 {
		t.Error("expected at least some games from Epic library")
	}
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(n) / float64(total) * 100
}
