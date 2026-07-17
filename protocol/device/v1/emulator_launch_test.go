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
