package frontenddist

import "embed"

// Files contains the built frontend dist so the server can fall back to embedded assets.
//
//go:embed all:dist
var Files embed.FS
