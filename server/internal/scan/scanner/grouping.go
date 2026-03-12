package scanner

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

// GameGroup is a set of files that belong to a single game candidate.
type GameGroup struct {
	Name      string          // derived from directory name or anchor file stem
	RootDir   string          // directory that owns this group (slash-separated, relative)
	Files     []AnnotatedFile // files in this group (directories excluded)
	Platform  core.Platform   // detected target platform (PlatformUnknown when uncertain)
	GroupKind core.GroupKind  // self_contained, packed, extras, or unknown
}

// FileGrouper groups annotated files into game candidates using directory
// structure, name-based heuristics, and multi-file clustering.
type FileGrouper struct {
	containerKeywords      map[string]bool
	platformKeywords       []string
	anchorContainerThresh  int
	anchorRatioThresh      float64
}

func NewFileGrouper() *FileGrouper {
	return &FileGrouper{
		containerKeywords: map[string]bool{
			"installers": true,
			"installer":  true,
			"mame":       true,
			"roms":       true,
			"rom":        true,
			"games":      true,
			"manuals":    true,
			"extras":     true,
			"dlc":        true,
			"patches":    true,
			"mods":       true,
			"updates":    true,
		},
		platformKeywords: []string{
			"playstation",
			"xbox",
			"nintendo",
			"game boy",
			"ms dos",
			"scummvm",
			"sega",
			"dreamcast",
			"saturn",
			"genesis",
			"mega drive",
			"gamecube",
			"wii",
			"super nintendo",
			"snes",
			"atari",
			"amiga",
			"commodore",
			"arcade",
			"neo geo",
			"neogeo",
		},
		anchorContainerThresh: 3,
		anchorRatioThresh:     0.5,
	}
}

// Group takes annotated files and returns game groups.
func (g *FileGrouper) Group(files []AnnotatedFile) []GameGroup {
	tree := buildTree(files)
	var groups []GameGroup
	g.collectGroups(tree, &groups)
	return groups
}

// ── directory tree ──────────────────────────────────────────────────

type dirNode struct {
	name     string
	path     string // slash-separated relative path
	files    []AnnotatedFile
	children map[string]*dirNode
	order    []string // insertion order of children
}

func newDirNode(name, path string) *dirNode {
	return &dirNode{name: name, path: path, children: make(map[string]*dirNode)}
}

func buildTree(files []AnnotatedFile) *dirNode {
	root := newDirNode("", "")
	for _, f := range files {
		if f.IsDir {
			continue
		}
		dir := filepath.ToSlash(f.Dir)
		node := root
		if dir != "" {
			parts := strings.Split(dir, "/")
			pathSoFar := ""
			for _, p := range parts {
				if pathSoFar != "" {
					pathSoFar += "/"
				}
				pathSoFar += p
				child, ok := node.children[p]
				if !ok {
					child = newDirNode(p, pathSoFar)
					node.children[p] = child
					node.order = append(node.order, p)
				}
				node = child
			}
		}
		node.files = append(node.files, f)
	}
	return root
}

// ── tree walk ───────────────────────────────────────────────────────

func (g *FileGrouper) collectGroups(node *dirNode, groups *[]GameGroup) {
	if len(node.files) > 0 {
		*groups = append(*groups, clusterFiles(node.files, node.path)...)
	}
	for _, name := range node.order {
		child := node.children[name]
		if g.isContainer(child) {
			g.collectGroups(child, groups)
		} else {
			allFiles := collectAllFiles(child)
			if len(allFiles) > 0 {
				*groups = append(*groups, GameGroup{
					Name:    child.name,
					RootDir: child.path,
					Files:   allFiles,
				})
			}
		}
	}
}

// isContainer decides whether a directory holds multiple games (container)
// or is itself one game (game directory).
func (g *FileGrouper) isContainer(node *dirNode) bool {
	lower := strings.ToLower(node.name)

	if g.containerKeywords[lower] {
		return true
	}
	for _, kw := range g.platformKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}

	// If child directories themselves contain anchors, this node is
	// organisational (each child is a separate game or sub-container).
	if len(node.children) > 0 {
		for _, child := range node.children {
			if subtreeHasAnchors(child) {
				return true
			}
		}
	}

	// Leaf directory with 3+ distinct anchor cluster-keys AND a high anchor
	// ratio is a flat collection. The ratio guard prevents directories like
	// DOS games (2-5 .exe + many support files) from being split up.
	if len(node.children) == 0 && len(node.files) > 0 {
		keys := distinctAnchorKeys(node.files)
		if len(keys) < g.anchorContainerThresh {
			return false
		}
		anchors := countAnchors(node.files)
		return float64(anchors)/float64(len(node.files)) > g.anchorRatioThresh
	}
	return false
}

// ── anchor helpers ──────────────────────────────────────────────────

func isAnchor(kind FileKind) bool {
	switch kind {
	case FileKindExecutable, FileKindDOSExecutable,
		FileKindArchive, FileKindDiscImage, FileKindDiscMeta:
		return true
	}
	return false
}

func subtreeHasAnchors(node *dirNode) bool {
	for _, f := range node.files {
		if isAnchor(f.Kind) {
			return true
		}
	}
	for _, child := range node.children {
		if subtreeHasAnchors(child) {
			return true
		}
	}
	return false
}

func countAnchors(files []AnnotatedFile) int {
	n := 0
	for _, f := range files {
		if isAnchor(f.Kind) {
			n++
		}
	}
	return n
}

func distinctAnchorKeys(files []AnnotatedFile) map[string]bool {
	keys := map[string]bool{}
	for _, f := range files {
		if isAnchor(f.Kind) {
			keys[clusterKey(f.Name)] = true
		}
	}
	return keys
}

// ── file collection ─────────────────────────────────────────────────

func collectAllFiles(node *dirNode) []AnnotatedFile {
	var all []AnnotatedFile
	all = append(all, node.files...)
	for _, name := range node.order {
		all = append(all, collectAllFiles(node.children[name])...)
	}
	return all
}

// ── multi-file clustering ───────────────────────────────────────────

func clusterFiles(files []AnnotatedFile, parentDir string) []GameGroup {
	clusters := map[string]*[]AnnotatedFile{}
	var order []string

	for i := range files {
		key := clusterKey(files[i].Name)
		bucket, exists := clusters[key]
		if !exists {
			bucket = &[]AnnotatedFile{}
			clusters[key] = bucket
			order = append(order, key)
		}
		*bucket = append(*bucket, files[i])
	}

	groups := make([]GameGroup, 0, len(clusters))
	for _, key := range order {
		groups = append(groups, GameGroup{
			Name:    key,
			RootDir: parentDir,
			Files:   *clusters[key],
		})
	}
	return groups
}

var (
	reTrailingBinPart = regexp.MustCompile(`-\d+$`)
	reTrailingTrack   = regexp.MustCompile(`(?i)\s*\(Track\s*\d+\)$`)
	reTrailingDisc    = regexp.MustCompile(`(?i)\s*\(Disc\s*\d+\)$`)
)

// clusterKey strips the extension and common multi-file suffixes to
// produce a shared key for files that belong together.
func clusterKey(filename string) string {
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	stem = strings.ToLower(stem)
	stem = reTrailingBinPart.ReplaceAllString(stem, "")
	stem = reTrailingTrack.ReplaceAllString(stem, "")
	stem = reTrailingDisc.ReplaceAllString(stem, "")
	return stem
}
