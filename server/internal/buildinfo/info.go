package buildinfo

import "github.com/GreenFuze/MyGamesAnywhere/server/internal/core"

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

var authorCredits = []string{"GreenFuze"}

func AboutInfo() core.AboutInfo {
	return core.AboutInfo{
		Version:       Version,
		Commit:        Commit,
		BuildDate:     BuildDate,
		AuthorCredits: append([]string(nil), authorCredits...),
	}
}
