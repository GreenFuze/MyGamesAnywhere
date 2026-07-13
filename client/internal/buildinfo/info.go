package buildinfo

// These values are replaced by release builds through -ldflags.
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Info is immutable build metadata displayed by the CLI and reported during
// the future device handshake.
type Info struct {
	Version   string
	Commit    string
	BuildDate string
}

// Current returns a snapshot so callers do not depend on mutable package vars.
func Current() Info {
	return Info{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
	}
}
