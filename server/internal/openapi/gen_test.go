package openapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateOpenAPI runs the generator and writes server/openapi.yaml.
// Each run produces the latest API surface from the router (reflection) and operation docs.
func TestGenerateOpenAPI(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "openapi.yaml")
	if err := Generate(outPath); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	content := string(body)
	if !strings.Contains(content, "openapi: 3.0.3") {
		t.Error("expected openapi 3.0.3 header")
	}
	if !strings.Contains(content, "/api/games") {
		t.Error("expected /api/games path")
	}
	if !strings.Contains(content, "post:") {
		t.Error("expected at least one post method")
	}
}

// TestCollectRoutes discovers routes from the router (nil builder = no-op handlers).
func TestCollectRoutes(t *testing.T) {
	routes, err := CollectRoutes()
	if err != nil {
		t.Fatalf("CollectRoutes: %v", err)
	}
	if len(routes) == 0 {
		t.Fatal("expected at least one route")
	}
	hasHealth := false
	hasPostScan := false
	hasGetScan := false
	for _, r := range routes {
		if r.Method == "GET" && r.Path == "/health" {
			hasHealth = true
		}
		if r.Method == "POST" && r.Path == "/api/scan" {
			hasPostScan = true
		}
		if r.Method == "GET" && r.Path == "/api/scan" {
			hasGetScan = true
		}
	}
	if !hasHealth {
		t.Error("expected GET /health route")
	}
	if !hasPostScan {
		t.Error("expected POST /api/scan route")
	}
	if !hasGetScan {
		t.Error("expected GET /api/scan route")
	}
}
