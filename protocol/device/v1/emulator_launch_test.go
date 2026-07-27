package v1

import "testing"

func TestEmulatorLaunchRequestRejectsExecutionAndPathEscapes(t *testing.T) {
	valid := EmulatorLaunchRequest{
		GameID: "game", SourceGameID: "source", Title: "Game", Platform: "scummvm", EmulatorID: "scummvm",
		Artifacts: []EmulatorContentArtifact{{Path: "data/game.dat", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", DownloadURL: "/api/device-transfers/content", DownloadToken: "token"}},
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, unsafe := range []string{"../game.dat", "/game.dat", `C:\game.dat`, `data\game.dat`} {
		request := valid
		request.Artifacts = append([]EmulatorContentArtifact(nil), valid.Artifacts...)
		request.Artifacts[0].Path = unsafe
		if err := request.Validate(); err == nil {
			t.Fatalf("unsafe path %q was accepted", unsafe)
		}
	}
}

func TestEmulatorLaunchRequestAcceptsTypedDuckStationContent(t *testing.T) {
	request := EmulatorLaunchRequest{
		GameID: "game", SourceGameID: "source", Title: "Game", Platform: "ps1",
		EmulatorID: "duckstation", ContentPath: "disc/game.cue",
		Artifacts: []EmulatorContentArtifact{{Path: "disc/game.cue", SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", DownloadURL: "/api/device-transfers/content", DownloadToken: "token"}},
	}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	request.CoreID = "not-allowed"
	if err := request.Validate(); err == nil {
		t.Fatal("DuckStation core override was accepted")
	}
}
