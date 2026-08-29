package contentdelivery

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

const (
	ManifestSchemaVersion = 1
	MaxManifestFiles      = 100_000
)

var (
	ErrNotFound                = errors.New("content copy or file not found")
	ErrInvalidContent          = errors.New("content copy contains invalid file evidence")
	ErrUnavailable             = errors.New("content delivery is unavailable")
	ErrMaterializationRequired = errors.New("content materialization is required")
	ErrSourceChanged           = errors.New("source content changed after the last scan")
	ErrJobNotCancellable       = errors.New("materialization job is not cancellable")
)

type Copy struct {
	CanonicalGameID string
	SourceGame      *core.SourceGame
}

type Delivery struct {
	Mode                    core.SourceDeliveryMode `json:"mode"`
	Ready                   bool                    `json:"ready"`
	MaterializationRequired bool                    `json:"materialization_required,omitempty"`
}

type Checksum struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

type ManifestFile struct {
	ID           string            `json:"id"`
	RelativePath string            `json:"relative_path"`
	Name         string            `json:"name"`
	Role         core.GameFileRole `json:"role"`
	Kind         string            `json:"kind,omitempty"`
	Length       int64             `json:"length"`
	Revision     string            `json:"revision"`
	ETag         string            `json:"etag"`
	Checksum     *Checksum         `json:"checksum,omitempty"`
	ModifiedAt   *time.Time        `json:"modified_at,omitempty"`
}

type Manifest struct {
	SchemaVersion   int            `json:"schema_version"`
	CopyID          string         `json:"copy_id"`
	CanonicalGameID string         `json:"canonical_game_id"`
	Title           string         `json:"title"`
	Platform        core.Platform  `json:"platform"`
	Revision        string         `json:"revision"`
	ETag            string         `json:"etag"`
	Delivery        Delivery       `json:"delivery"`
	Files           []ManifestFile `json:"files"`
}

func BuildManifest(copy *Copy, delivery Delivery) (*Manifest, error) {
	if copy == nil || copy.SourceGame == nil || strings.TrimSpace(copy.SourceGame.ID) == "" {
		return nil, fmt.Errorf("%w: copy is required", ErrInvalidContent)
	}

	files := make([]ManifestFile, 0, len(copy.SourceGame.Files))
	seenPaths := make(map[string]struct{}, len(copy.SourceGame.Files))
	seenIDs := make(map[string]struct{}, len(copy.SourceGame.Files))
	for _, sourceFile := range copy.SourceGame.Files {
		if sourceFile.IsDir {
			continue
		}
		if sourceFile.Size < 0 {
			return nil, fmt.Errorf("%w: file size cannot be negative", ErrInvalidContent)
		}
		if len(files) >= MaxManifestFiles {
			return nil, fmt.Errorf("%w: manifest exceeds %d files", ErrInvalidContent, MaxManifestFiles)
		}
		relativePath, err := NormalizeRelativePath(sourceFile.Path)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidContent, err)
		}
		if _, exists := seenPaths[relativePath]; exists {
			return nil, fmt.Errorf("%w: normalized path collision", ErrInvalidContent)
		}
		seenPaths[relativePath] = struct{}{}

		fileID := FileID(copy.SourceGame.ID, relativePath)
		if _, exists := seenIDs[fileID]; exists {
			return nil, fmt.Errorf("%w: opaque file id collision", ErrInvalidContent)
		}
		seenIDs[fileID] = struct{}{}

		revision := FileRevision(relativePath, sourceFile)
		files = append(files, ManifestFile{
			ID:           fileID,
			RelativePath: relativePath,
			Name:         path.Base(relativePath),
			Role:         sourceFile.Role,
			Kind:         strings.TrimSpace(sourceFile.FileKind),
			Length:       sourceFile.Size,
			Revision:     revision,
			ETag:         QuoteETag(revision),
			Checksum:     ExplicitSHA256(sourceFile),
			ModifiedAt:   normalizedTime(sourceFile.ModifiedAt),
		})
	}

	sort.Slice(files, func(i, j int) bool { return files[i].RelativePath < files[j].RelativePath })
	manifestRevision := ManifestRevision(copy.SourceGame.ID, files)
	return &Manifest{
		SchemaVersion:   ManifestSchemaVersion,
		CopyID:          copy.SourceGame.ID,
		CanonicalGameID: copy.CanonicalGameID,
		Title:           copy.SourceGame.RawTitle,
		Platform:        copy.SourceGame.Platform,
		Revision:        manifestRevision,
		ETag:            QuoteETag(manifestRevision),
		Delivery:        delivery,
		Files:           files,
	}, nil
}

func NormalizeRelativePath(value string) (string, error) {
	if value == "" || strings.ContainsRune(value, '\x00') {
		return "", errors.New("file path is empty or contains NUL")
	}
	normalized := strings.ReplaceAll(value, "\\", "/")
	if strings.HasPrefix(normalized, "/") || (len(normalized) >= 2 && normalized[1] == ':') {
		return "", errors.New("absolute file path is not allowed")
	}
	for _, segment := range strings.Split(normalized, "/") {
		if segment == ".." {
			return "", errors.New("file path traversal is not allowed")
		}
	}
	normalized = path.Clean(normalized)
	if normalized == "." || normalized == "" || normalized == ".." || strings.HasPrefix(normalized, "../") {
		return "", errors.New("file path is not a safe relative path")
	}
	return normalized, nil
}

func FileID(copyID, relativePath string) string {
	return digest("mga-content-file-v1", copyID, relativePath)
}

func ValidFileID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func FileRevision(relativePath string, file core.GameFile) string {
	modified := ""
	if file.ModifiedAt != nil {
		modified = file.ModifiedAt.UTC().Format(time.RFC3339Nano)
	}
	return digest(
		"mga-content-revision-v1",
		relativePath,
		string(file.Role),
		strings.TrimSpace(file.FileKind),
		fmt.Sprintf("%d", file.Size),
		strings.TrimSpace(file.ObjectID),
		strings.TrimSpace(file.Revision),
		modified,
	)
}

func ManifestRevision(copyID string, files []ManifestFile) string {
	parts := make([]string, 0, len(files)+2)
	parts = append(parts, "mga-content-manifest-v1", copyID)
	for _, file := range files {
		parts = append(parts, file.ID+":"+file.Revision)
	}
	return digest(parts...)
}

func QuoteETag(revision string) string {
	return `"` + strings.Trim(strings.TrimSpace(revision), `"`) + `"`
}

func ExplicitSHA256(file core.GameFile) *Checksum {
	for _, evidence := range []string{file.Revision, file.ObjectID} {
		value := strings.TrimSpace(evidence)
		if len(value) != len("sha256:")+64 || !strings.EqualFold(value[:len("sha256:")], "sha256:") {
			continue
		}
		digestValue := strings.ToLower(value[len("sha256:"):])
		decoded, err := hex.DecodeString(digestValue)
		if err == nil && len(decoded) == sha256.Size {
			return &Checksum{Algorithm: "sha256", Value: digestValue}
		}
	}
	return nil
}

func digest(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func normalizedTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	normalized := value.UTC()
	return &normalized
}
