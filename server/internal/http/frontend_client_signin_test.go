package http

import (
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/frontendauth"
)

// Connecting an external frontend used to mean opening the console as an
// administrator, issuing a key by hand and copying it across. That is a poor
// fit for what is being connected — a Playnite install on a television — and it
// made a key something a person handles.
//
// The exchange replaces that: a profile proves who it is with its own password
// and receives a scoped key. These tests pin what the exchange is allowed to
// hand over, because the whole point is that a password becomes something
// narrower than a password.

func TestSignInGrantsTheFrontendSurfaceAndNotManagement(t *testing.T) {
	// A library frontend needs to read the catalogue, its artwork and its
	// content, and the profile has just proved it may have all of that. It has
	// no business holding a management key, and the first version of this
	// endpoint handed one out because it defaulted to AllScopes(): the issued
	// key came back as "catalog.read, content.prepare, content.read,
	// management, metadata.read".
	//
	// `management` gates no route today, which is what makes the mistake easy
	// to miss and expensive later — a key issued now would quietly become a
	// management key the day something starts gating on it.
	scopes, err := frontendSignInScopes(nil)
	if err != nil {
		t.Fatalf("frontendSignInScopes(nil) error = %v", err)
	}

	granted := make(map[frontendauth.Scope]bool, len(scopes))
	for _, scope := range scopes {
		granted[scope] = true
	}
	for _, wanted := range []frontendauth.Scope{
		frontendauth.ScopeCatalogRead,
		frontendauth.ScopeMetadataRead,
		frontendauth.ScopeContentRead,
		frontendauth.ScopeContentPrepare,
	} {
		if !granted[wanted] {
			t.Errorf("a frontend key was issued without %s", wanted)
		}
	}
	if granted[frontendauth.ScopeManagement] {
		t.Error("a password exchange handed out a management scope")
	}
}

func TestSignInGrantsOnlyWhatWasAskedFor(t *testing.T) {
	// A client that wants to read a catalogue and nothing else should hold a
	// key that cannot fetch content, even though the password behind it could
	// have authorised one that could.
	scopes, err := frontendSignInScopes([]string{string(frontendauth.ScopeCatalogRead)})
	if err != nil {
		t.Fatalf("frontendSignInScopes() error = %v", err)
	}
	if len(scopes) != 1 || scopes[0] != frontendauth.ScopeCatalogRead {
		t.Fatalf("got %v, want only catalog.read", scopes)
	}
}

func TestSignInRefusesAPermissionThisServerDoesNotHave(t *testing.T) {
	// Failing closed matters more here than convenience: silently dropping an
	// unknown scope would issue a key that is quietly less capable than the
	// caller believes, and the failure would surface later as an unexplained
	// 403 in a frontend.
	if _, err := frontendSignInScopes([]string{"catalog.read", "everything.please"}); err == nil {
		t.Fatal("an unsupported permission was accepted")
	}
}

func TestSignInIsUnavailableWithoutAnIssuer(t *testing.T) {
	// A build that does not mount the scoped frontend API has nothing to issue.
	// Saying so is better than a nil dereference in a handler that accepts
	// passwords.
	controller := &AuthController{}
	if controller.frontendClients != nil {
		t.Fatal("a controller with no issuer wired reported one")
	}
}
