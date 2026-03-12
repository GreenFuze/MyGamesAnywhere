package scanner

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
	Dir       string            // parent directory (relative)
	Depth     int               // nesting depth (0 = root-level entry)
	Role      core.GameFileRole // assigned during role assignment step
}

// Scanner classifies games from a filesystem-style game source.
// It is stateless and pure: given a flat file listing, it returns
// classified game groups. It knows nothing about the database,
// metadata plugins, or previous scans.
type Scanner struct {
	logger           core.Logger
	grouper          *FileGrouper
	platformDetector *PlatformDetector
	classifier       *GroupClassifier
	roleAssigner     *RoleAssigner
}

func New(logger core.Logger) *Scanner {
	return &Scanner{
		logger:           logger,
		grouper:          NewFileGrouper(),
		platformDetector: NewPlatformDetector(),
		classifier:       NewGroupClassifier(),
		roleAssigner:     NewRoleAssigner(),
	}
}

// ScanFiles takes a flat file listing from a filesystem source plugin
// and returns classified game groups.
//
// Pipeline:
//  1. Annotate each entry with kind, extension, depth.
//  2. Group files into game candidates.
//  3. Detect platform for each group.
//  4. Classify each group (self_contained, packed, extras, unknown).
//  5. Assign file roles (root, required, optional) within each group.
func (s *Scanner) ScanFiles(ctx context.Context, files []core.FileEntry) ([]GameGroup, error) {
	annotated := annotateFiles(files)
	groups := s.grouper.Group(annotated)
	s.platformDetector.DetectAll(groups)
	s.classifier.ClassifyAll(groups)
	s.roleAssigner.AssignAll(groups)
	s.logger.Info("ScanFiles pipeline complete", "files", len(files), "groups", len(groups))
	return groups, nil
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
