package openapi

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	internalHttp "github.com/GreenFuze/MyGamesAnywhere/server/internal/http"
	"github.com/go-chi/chi/v5"
)

// RouteEntry is a method + path pair discovered from the router.
type RouteEntry struct {
	Method string
	Path   string
}

// CollectRoutes builds the router with nil handlers and collects all public routes (method + path).
func CollectRoutes() ([]RouteEntry, error) {
	r := internalHttp.BuildRouter(nil, 0)
	var entries []RouteEntry
	err := chi.Walk(r, func(method, path string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		if method != "" && path != "" {
			entries = append(entries, RouteEntry{Method: method, Path: path})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// BuildOpenAPI returns the OpenAPI 3.0 YAML document as bytes.
func BuildOpenAPI(routes []RouteEntry, docs []OperationDoc) ([]byte, error) {
	docByKey := make(map[string]OperationDoc)
	for _, d := range docs {
		docByKey[d.Method+" "+d.Path] = d
	}

	var b bytes.Buffer
	b.WriteString("openapi: 3.0.3\n")
	b.WriteString("info:\n  title: MyGamesAnywhere Server API\n  description: Public HTTP API for the MyGamesAnywhere desktop app server.\n  version: 1.0.0\n")
	b.WriteString("paths:\n")

	// Sort routes for stable output
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Path != routes[j].Path {
			return routes[i].Path < routes[j].Path
		}
		return routes[i].Method < routes[j].Method
	})

	pathSeen := make(map[string]bool)
	for _, e := range routes {
		if pathSeen[e.Path] {
			continue
		}
		pathSeen[e.Path] = true
		pathRoutes := make([]RouteEntry, 0)
		for _, r := range routes {
			if r.Path == e.Path {
				pathRoutes = append(pathRoutes, r)
			}
		}

		b.WriteString("\n  ")
		b.WriteString(escapeYAMLString(e.Path))
		b.WriteString(":\n")
		for _, re := range pathRoutes {
			key := re.Method + " " + re.Path
			op := docByKey[key]
			b.WriteString("    ")
			b.WriteString(strings.ToLower(re.Method))
			b.WriteString(":\n")
			if op.Summary != "" {
				b.WriteString("      summary: ")
				b.WriteString(escapeYAMLString(op.Summary))
				b.WriteString("\n")
			}
			if op.Description != "" {
				b.WriteString("      description: ")
				b.WriteString(escapeYAMLString(op.Description))
				b.WriteString("\n")
			}
			// Path parameters (before requestBody per OpenAPI order)
			if strings.Contains(re.Path, "{") {
				b.WriteString("      parameters:\n")
				for _, seg := range strings.Split(re.Path, "/") {
					if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
						name := strings.TrimSuffix(strings.TrimPrefix(seg, "{"), "}")
						b.WriteString("        - name: ")
						b.WriteString(name)
						b.WriteString("\n          in: path\n          required: true\n          schema:\n            type: string\n")
					}
				}
			}
			if op.RequestBodyDoc != "" {
				b.WriteString("      requestBody:\n        description: ")
				b.WriteString(escapeYAMLString(op.RequestBodyDoc))
				b.WriteString("\n        content:\n          application/json:\n            schema:\n              type: object\n")
			}
			b.WriteString("      responses:\n")
			codes := []string{"200", "201", "202", "400", "404", "500", "501"}
			for _, code := range codes {
				desc := op.ResponseDocs[code]
				if desc == "" && (code == "500" || code == "400") {
					desc = "Error response"
				}
				if desc != "" {
					b.WriteString("        ")
					b.WriteString(code)
					b.WriteString(":\n          description: ")
					b.WriteString(escapeYAMLString(desc))
					b.WriteString("\n")
				}
			}
		}
	}
	return b.Bytes(), nil
}

func escapeYAMLString(s string) string {
	if s == "" {
		return "''"
	}
	if strings.Contains(s, "\n") || strings.Contains(s, ":") || strings.Contains(s, "#") {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	return s
}

// Generate writes the OpenAPI YAML file to outPath (e.g. server/openapi.yaml).
func Generate(outPath string) error {
	routes, err := CollectRoutes()
	if err != nil {
		return fmt.Errorf("collect routes: %w", err)
	}
	docs := Operations()
	out, err := BuildOpenAPI(routes, docs)
	if err != nil {
		return fmt.Errorf("build openapi: %w", err)
	}
	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return os.WriteFile(outPath, out, 0644)
}
