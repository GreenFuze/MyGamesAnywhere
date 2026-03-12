package scan

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

// FileKind is the coarse type of a file determined by extension.
type FileKind string

const (
	FileKindExecutable    FileKind = "executable"
	FileKindDOSExecutable FileKind = "dos_executable"
	FileKindScript        FileKind = "script"
	FileKindArchive       FileKind = "archive"
	FileKindDiscImage     FileKind = "disc_image"
	FileKindDiscMeta      FileKind = "disc_meta"
	FileKindAudio         FileKind = "audio"
	FileKindImage         FileKind = "image"
	FileKindDocument      FileKind = "document"
	FileKindUnknown       FileKind = "unknown"
)

// AnnotatedFile is a FileEntry enriched with classification metadata.
type AnnotatedFile struct {
	core.FileEntry
	Kind      FileKind
	Extension string
	Dir       string // parent directory (relative)
	Depth     int    // nesting depth (0 = root-level entry)
}

// GameScanner classifies games from a filesystem-style game source.
type GameScanner struct {
	logger           core.Logger
	grouper          *FileGrouper
	platformDetector *PlatformDetector
	classifier       *GroupClassifier
}

func NewGameScanner(logger core.Logger) *GameScanner {
	return &GameScanner{
		logger:           logger,
		grouper:          NewFileGrouper(),
		platformDetector: NewPlatformDetector(),
		classifier:       NewGroupClassifier(),
	}
}

// ScanFiles takes a flat file listing from a filesystem source plugin
// and returns classified games with their files.
// Step 1: annotate each entry with kind, extension, depth.
// Step 2: group files into game candidates.
// Step 3: detect platform for each group.
// Step 4: classify each group (self_contained, packed, extras, unknown).
// Step 5 (game file mapping) is TODO.
func (s *GameScanner) ScanFiles(ctx context.Context, files []core.FileEntry) ([]*core.Game, []*core.GameFile, error) {
	annotated := annotateFiles(files)
	groups := s.grouper.Group(annotated)
	s.platformDetector.DetectAll(groups)
	s.classifier.ClassifyAll(groups)
	s.logger.Info("ScanFiles pipeline complete", "files", len(files), "groups", len(groups))
	_ = groups // next steps will consume groups
	return nil, nil, nil
}

// annotateFiles enriches raw file entries with kind, extension, directory, and depth.
func annotateFiles(files []core.FileEntry) []AnnotatedFile {
	out := make([]AnnotatedFile, 0, len(files))
	for _, f := range files {
		ext := ""
		if !f.IsDir {
			ext = strings.ToLower(filepath.Ext(f.Name))
		}
		dir := filepath.Dir(f.Path)
		if dir == "." {
			dir = ""
		}
		depth := 0
		if f.Path != "" {
			depth = strings.Count(filepath.ToSlash(f.Path), "/")
		}

		out = append(out, AnnotatedFile{
			FileEntry: f,
			Kind:      detectFileKind(ext, f.IsDir),
			Extension: ext,
			Dir:       dir,
			Depth:     depth,
		})
	}
	return out
}

func detectFileKind(ext string, isDir bool) FileKind {
	if isDir {
		return FileKindUnknown
	}
	if kind, ok := extToKind[ext]; ok {
		return kind
	}
	return FileKindUnknown
}

var extToKind = map[string]FileKind{
	// Executables
	".exe": FileKindExecutable,
	".msi": FileKindExecutable,
	".com": FileKindDOSExecutable,
	".bat": FileKindScript,
	".cmd": FileKindScript,
	".sh":  FileKindScript,

	// Archives
	".zip": FileKindArchive,
	".7z":  FileKindArchive,
	".rar": FileKindArchive,
	".tar": FileKindArchive,
	".gz":  FileKindArchive,

	// Disc images
	".iso": FileKindDiscImage,
	".img": FileKindDiscImage,
	".mdf": FileKindDiscImage,
	".chd": FileKindDiscImage,
	".cue": FileKindDiscMeta,
	".ccd": FileKindDiscMeta,
	".mds": FileKindDiscMeta,
	// .bin intentionally omitted: too ambiguous (disc track, GOG data, PS3 EBOOT)
	".sub": FileKindDiscMeta,

	// Audio
	".mp3":  FileKindAudio,
	".ogg":  FileKindAudio,
	".flac": FileKindAudio,
	".wav":  FileKindAudio,
	".voc":  FileKindAudio,
	".cmf":  FileKindAudio,
	".mid":  FileKindAudio,
	".xm":   FileKindAudio,
	".mod":  FileKindAudio,
	".s3m":  FileKindAudio,

	// Images
	".png": FileKindImage,
	".jpg": FileKindImage,
	".bmp": FileKindImage,
	".gif": FileKindImage,
	".ico": FileKindImage,

	// Documents
	".pdf": FileKindDocument,
	".txt": FileKindDocument,
	".doc": FileKindDocument,
	".rtf": FileKindDocument,
	".nfo": FileKindDocument,
	".md":  FileKindDocument,
}
