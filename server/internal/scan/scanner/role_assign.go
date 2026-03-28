package scanner

import "github.com/GreenFuze/MyGamesAnywhere/server/internal/core"

// rootPriority defines which FileKinds can serve as a group's root file.
// Lower value = higher priority. Kinds not listed cannot be root.
var rootPriority = map[FileKind]int{
	FileKindDiscMeta:      1, // .cue references disc images — the entry point
	FileKindDiscImage:     2, // standalone .iso / .chd
	FileKindExecutable:    3, // .exe
	FileKindDOSExecutable: 4, // .com
	FileKindArchive:       5, // .zip (MAME ROM, packed archive)
}

var rawROMExtensions = map[string]bool{
	".nes": true,
	".fds": true,
	".smc": true,
	".sfc": true,
	".gb":  true,
	".gbc": true,
	".gba": true,
	".gen": true,
	".md":  true,
	".smd": true,
	".sms": true,
	".gg":  true,
	".32x": true,
}

// RoleAssigner assigns a GameFileRole to each file within a GameGroup.
type RoleAssigner struct{}

func NewRoleAssigner() *RoleAssigner { return &RoleAssigner{} }

// AssignAll assigns roles to every file in every group, in-place.
func (r *RoleAssigner) AssignAll(groups []GameGroup) {
	for i := range groups {
		r.assign(&groups[i])
	}
}

func (r *RoleAssigner) assign(g *GameGroup) {
	rootIdx := selectRoot(g.Platform, g.Files)

	for i := range g.Files {
		if i == rootIdx {
			g.Files[i].Role = core.GameFileRoleRoot
			continue
		}
		g.Files[i].Role = fileRole(g.GroupKind, g.Files[i].Kind)
	}
}

// selectRoot picks the single best root file index, or -1 if none qualifies.
func selectRoot(platform core.Platform, files []AnnotatedFile) int {
	bestIdx := -1
	bestPri := 999
	bestSize := int64(-1)

	for i, f := range files {
		pri, ok := rootPriority[f.Kind]
		if !ok {
			continue
		}
		if pri < bestPri || (pri == bestPri && f.Size > bestSize) {
			bestIdx = i
			bestPri = pri
			bestSize = f.Size
		}
	}
	if bestIdx >= 0 {
		return bestIdx
	}

	if platform == core.PlatformMSDOS {
		for i, f := range files {
			if f.Kind == FileKindScript && f.Extension == ".bat" {
				if f.Size > bestSize {
					bestIdx = i
					bestSize = f.Size
				}
			}
		}
		if bestIdx >= 0 {
			return bestIdx
		}
	}

	for i, f := range files {
		if rawROMExtensions[f.Extension] {
			if f.Size > bestSize {
				bestIdx = i
				bestSize = f.Size
			}
		}
	}
	return bestIdx
}

// fileRole decides required vs optional for non-root files.
func fileRole(gk core.GroupKind, fk FileKind) core.GameFileRole {
	switch gk {
	case core.GroupKindPacked:
		return core.GameFileRoleRequired
	case core.GroupKindExtras:
		return core.GameFileRoleOptional
	default:
		if fk == FileKindDocument {
			return core.GameFileRoleOptional
		}
		return core.GameFileRoleRequired
	}
}
