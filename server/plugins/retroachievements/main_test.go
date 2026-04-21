package main

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestRAGetUsesBrowserLikeHeaders(t *testing.T) {
	origBase := raAPIBase
	origClient := raHTTPClient
	origTicker := rateLimiter
	defer func() {
		raAPIBase = origBase
		raHTTPClient = origClient
		rateLimiter = origTicker
	}()

	cfg = raConfig{Username: "retro-user", APIKey: "retro-key"}
	rateLimiter = time.NewTicker(time.Microsecond)
	t.Cleanup(rateLimiter.Stop)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != raUserAgent {
			t.Fatalf("user-agent = %q, want %q", got, raUserAgent)
		}
		if got := r.Header.Get("Accept"); !strings.Contains(got, "application/json") {
			t.Fatalf("accept = %q, want JSON-capable header", got)
		}
		if got := r.Header.Get("Accept-Language"); got == "" {
			t.Fatal("accept-language should be set")
		}
		values := r.URL.Query()
		if values.Get("z") != cfg.Username || values.Get("y") != cfg.APIKey {
			t.Fatalf("query = %v, want credentials included", values)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `[{"ID":1,"Name":"NES"}]`)
	}))
	defer server.Close()

	raAPIBase = server.URL
	raHTTPClient = server.Client()

	body, err := raGet("API_GetConsoleIDs.php", url.Values{"i": {"1"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != `[{"ID":1,"Name":"NES"}]` {
		t.Fatalf("body = %q", body)
	}
}

func TestRAGetIncludesResponseBodyOnFailure(t *testing.T) {
	origBase := raAPIBase
	origClient := raHTTPClient
	origTicker := rateLimiter
	defer func() {
		raAPIBase = origBase
		raHTTPClient = origClient
		rateLimiter = origTicker
	}()

	cfg = raConfig{Username: "retro-user", APIKey: "retro-key"}
	rateLimiter = time.NewTicker(time.Microsecond)
	t.Cleanup(rateLimiter.Stop)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "cloudflare block", http.StatusForbidden)
	}))
	defer server.Close()

	raAPIBase = server.URL
	raHTTPClient = server.Client()

	_, err := raGet("API_GetConsoleIDs.php", url.Values{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "status 403") {
		t.Fatalf("error = %v, want status code", err)
	}
	if !strings.Contains(err.Error(), "cloudflare block") {
		t.Fatalf("error = %v, want response body", err)
	}
}

func TestClassifyCheckConfigErrorCloudflareBlockIsUnavailable(t *testing.T) {
	status, message := classifyCheckConfigError(
		errors.New("RA API API_GetConsoleIDs.php: status 403: <title>Attention Required! | Cloudflare</title>"),
	)
	if status != "unavailable" {
		t.Fatalf("status = %q, want %q", status, "unavailable")
	}
	if !strings.Contains(strings.ToLower(message), "blocked or unavailable") {
		t.Fatalf("message = %q, want upstream unavailable wording", message)
	}
}

func TestValidateCheckConfigMissingCredentialsIsError(t *testing.T) {
	result := validateCheckConfig(map[string]any{})
	if got, _ := result["status"].(string); got != "error" {
		t.Fatalf("status = %q, want %q", got, "error")
	}
}
