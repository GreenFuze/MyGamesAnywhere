package v1

import (
	"strings"
	"testing"
	"time"
)

func TestSaveDomainClaimProtocolIsBoundedToScummVM(t *testing.T) {
	request := SaveDomainClaimRequest{GameID: "game-1", SourceGameID: "source-1", Title: "Game", AdapterID: "scummvm", RouteKind: "emulator", EmulatorID: "scummvm", RouteFingerprint: strings.Repeat("a", 64)}
	if err := request.Validate(); err != nil {
		t.Fatal(err)
	}
	request.AdapterID = "retroarch"
	if err := request.Validate(); err == nil {
		t.Fatal("unresolved RetroArch route was accepted")
	}
	result := SaveDomainClaimResult{GameID: "game-1", SourceGameID: "source-1", LocalSaveDomainID: "local-save-1", AdapterID: "scummvm", RouteFingerprint: strings.Repeat("a", 64), State: "owned_here", GrantedAt: time.Now()}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
}
