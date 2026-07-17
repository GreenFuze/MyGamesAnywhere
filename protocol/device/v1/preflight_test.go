package v1

import "testing"

func TestInstallationPreflightContracts(t *testing.T) {
	request := InstallationPreflightRequest{
		SchemaVersion: InstallationPreflightSchemaVersion,
		GameID:        "game-1", SourceGameID: "source-1",
		Category:        InstallationCategoryStorefront,
		DestinationRoot: `%USERPROFILE%\Games`,
		Requirements:    []PrerequisiteRequirement{{ID: "storefront.steam", Name: "Steam", Kind: PrerequisiteKindStorefront, Required: true}},
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	result := InstallationPreflightResult{
		SchemaVersion: InstallationPreflightSchemaVersion,
		CanInstall:    false,
		Checks:        []InstallationPreflightCheck{{ID: "storefront.steam", Name: "Steam", Kind: "storefront", Status: PreflightCheckMissing, Required: true, Message: "Steam was not found."}},
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("valid result rejected: %v", err)
	}
}

func TestInstallationPreflightRejectsUnsafeOrInconsistentValues(t *testing.T) {
	request := InstallationPreflightRequest{SchemaVersion: 1, GameID: "game-1", SourceGameID: "source-1", Category: InstallationCategoryStorefront, DestinationRoot: `C:\Games`}
	if err := request.Validate(); err == nil {
		t.Fatal("storefront request without requirement accepted")
	}
	result := InstallationPreflightResult{
		SchemaVersion: 1,
		CanInstall:    true,
		Checks:        []InstallationPreflightCheck{{ID: "storage", Name: "Storage", Kind: "storage", Status: PreflightCheckMissing, Required: true, Message: "Not enough space."}},
	}
	if err := result.Validate(); err == nil {
		t.Fatal("inconsistent result accepted")
	}
}
