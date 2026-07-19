package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

func TestFilesystemIncludePathsFromConfigSupportsLegacyRootPath(t *testing.T) {
	includes := filesystemIncludePathsFromConfig(map[string]any{
		"root_path": "Games/Arcade",
	})
	if len(includes) != 1 {
		t.Fatalf("include count = %d, want 1", len(includes))
	}
	if includes[0].Path != "Games/Arcade" {
		t.Fatalf("path = %q, want Games/Arcade", includes[0].Path)
	}
	if !includes[0].Recursive {
		t.Fatal("legacy root path should default to recursive")
	}
}

func TestFilesystemIncludePathsFromConfigReadsNormalizedIncludePaths(t *testing.T) {
	includes := filesystemIncludePathsFromConfig(map[string]any{
		"include_paths": []any{
			map[string]any{"path": `Games\Arcade`, "recursive": false},
		},
	})
	if len(includes) != 1 {
		t.Fatalf("include count = %d, want 1", len(includes))
	}
	if includes[0].Path != "Games/Arcade" {
		t.Fatalf("path = %q, want Games/Arcade", includes[0].Path)
	}
	if includes[0].Recursive {
		t.Fatal("recursive = true, want false")
	}
}

func TestFilesystemIncludePathsFromConfigReadsNestedExcludePaths(t *testing.T) {
	includes := filesystemIncludePathsFromConfig(map[string]any{
		"include_paths": []any{
			map[string]any{
				"path":          "Games/Arcade",
				"recursive":     true,
				"exclude_paths": []any{`Games\Arcade\Bad`, "", "Games/Arcade/Skip"},
			},
		},
	})
	if len(includes) != 1 {
		t.Fatalf("include count = %d, want 1", len(includes))
	}
	excludes := includes[0].ExcludePaths
	if len(excludes) != 2 {
		t.Fatalf("exclude count = %d, want 2", len(excludes))
	}
	if excludes[0] != "Games/Arcade/Bad" || excludes[1] != "Games/Arcade/Skip" {
		t.Fatalf("excludes = %#v", excludes)
	}
}

func TestDrivePathExcludedMatchesDescendantsOnly(t *testing.T) {
	excludes := []string{"Games/Arcade/Skip"}
	if !drivePathExcluded("Games/Arcade/Skip", excludes) {
		t.Fatal("expected exact excluded path to match")
	}
	if !drivePathExcluded("Games/Arcade/Skip/Nested/Game.zip", excludes) {
		t.Fatal("expected descendant path to match")
	}
	if drivePathExcluded("Games/Arcade/SkipButDifferent/Game.zip", excludes) {
		t.Fatal("did not expect sibling prefix to match")
	}
}

func TestDrivePathNotFoundSentinelSurvivesWrapping(t *testing.T) {
	err := fmt.Errorf("resolve save sync path: %w", errDrivePathNotFound)
	if !errors.Is(err, errDrivePathNotFound) {
		t.Fatalf("expected wrapped save-sync lookup error to classify as not found")
	}
}

func TestDriveTokenConfigRoundTrip(t *testing.T) {
	expiry := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	updates := tokenConfigUpdates(&oauth2.Token{
		AccessToken:  "access",
		RefreshToken: "refresh",
		TokenType:    "Bearer",
		Expiry:       expiry,
	})
	tok := tokenFromConfig(updates)
	if tok == nil {
		t.Fatal("tokenFromConfig returned nil")
	}
	if tok.AccessToken != "access" || tok.RefreshToken != "refresh" || tok.TokenType != "Bearer" || !tok.Expiry.Equal(expiry) {
		t.Fatalf("token = %+v, want round-tripped token expiring %s", tok, expiry)
	}
}

func TestDriveFolderBrowserListsMyDriveAndPaginatedSharedFolders(t *testing.T) {
	var sharedPageTokens []string
	srv := newFakeDriveService(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		switch {
		case strings.Contains(query, "sharedWithMe"):
			pageToken := r.URL.Query().Get("pageToken")
			sharedPageTokens = append(sharedPageTokens, pageToken)
			if pageToken == "" {
				writeDriveJSON(t, w, map[string]any{
					"nextPageToken": "page-2",
					"files":         []map[string]any{{"id": "shared-z", "name": "Zelda"}},
				})
				return
			}
			writeDriveJSON(t, w, map[string]any{
				"files": []map[string]any{{"id": "shared-a", "name": "Arcade"}},
			})
		case strings.Contains(query, "'root' in parents"):
			writeDriveJSON(t, w, map[string]any{
				"files": []map[string]any{{"id": "my-games", "name": "Games"}},
			})
		case strings.Contains(query, "'shared-a' in parents"):
			writeDriveJSON(t, w, map[string]any{
				"files": []map[string]any{{"id": "shared-child", "name": "SNES"}},
			})
		default:
			http.Error(w, "unexpected query: "+query, http.StatusBadRequest)
		}
	})

	browser := newDriveFolderBrowser(srv)
	root, err := browser.browse("")
	if err != nil {
		t.Fatalf("browse My Drive root: %v", err)
	}
	if len(root) != 2 || root[0].LocationKind != "shared_with_me" || root[0].Selectable || root[1].Path != "Games" {
		t.Fatalf("root folders = %#v", root)
	}

	shared, err := browser.browse(driveSharedBrowseToken)
	if err != nil {
		t.Fatalf("browse Shared with me: %v", err)
	}
	if len(shared) != 2 || shared[0].Name != "Arcade" || shared[1].Name != "Zelda" {
		t.Fatalf("shared folders = %#v, want sorted results from every page", shared)
	}
	if len(sharedPageTokens) != 2 || sharedPageTokens[0] != "" || sharedPageTokens[1] != "page-2" {
		t.Fatalf("shared page tokens = %#v", sharedPageTokens)
	}
	if shared[0].ObjectID != "shared-a" || shared[0].DisplayPath != "Shared with me/Arcade" || !shared[0].Selectable {
		t.Fatalf("shared selection = %#v", shared[0])
	}

	children, err := browser.browse(shared[0].Path)
	if err != nil {
		t.Fatalf("browse shared child: %v", err)
	}
	if len(children) != 1 || children[0].ObjectID != "shared-child" || children[0].DisplayPath != "Shared with me/Arcade/SNES" {
		t.Fatalf("shared children = %#v", children)
	}
}

func TestDriveBrowseTokenRejectsUntrustedObjectID(t *testing.T) {
	token := driveFolderTokenPrefix + "../outside?path=Shared%20with%20me%2FGames"
	if _, err := driveBrowseLocationFromToken(token); err == nil {
		t.Fatal("malformed object ID must fail fast")
	}
}

func TestResolveIncludeFolderIDUsesStableProviderObject(t *testing.T) {
	requestedPath := ""
	srv := newFakeDriveService(t, func(w http.ResponseWriter, r *http.Request) {
		requestedPath = r.URL.Path
		writeDriveJSON(t, w, map[string]any{
			"id":       "shared-folder",
			"name":     "Renamed by owner",
			"mimeType": driveFolderMimeType,
			"trashed":  false,
		})
	})

	folderID, err := resolveIncludeFolderID(srv, filesystemIncludePathsFromConfig(map[string]any{
		"include_paths": []any{map[string]any{
			"path":      "Shared with me/Old name",
			"recursive": true,
			"object_id": "shared-folder",
		}},
	})[0])
	if err != nil {
		t.Fatalf("resolve stable folder ID: %v", err)
	}
	if folderID != "shared-folder" || !strings.HasSuffix(requestedPath, "/files/shared-folder") {
		t.Fatalf("folderID=%q requestedPath=%q, want direct stable-object lookup", folderID, requestedPath)
	}
}

func newFakeDriveService(t *testing.T, handler http.HandlerFunc) *drive.Service {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	srv, err := drive.NewService(context.Background(), option.WithEndpoint(server.URL+"/"), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("create fake Drive service: %v", err)
	}
	return srv
}

func writeDriveJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Errorf("encode fake Drive response: %v", err)
	}
}
