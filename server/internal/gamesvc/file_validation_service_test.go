package gamesvc

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type fileValidationRepoStub struct {
	integration *core.Integration
	err         error
}

func (f *fileValidationRepoStub) GetByID(context.Context, string) (*core.Integration, error) {
	return f.integration, f.err
}

type fileValidationStoreStub struct {
	sourceGames []*core.SourceGame
	loadErr     error
	deleteErr   error
	deletedIDs  []string
}

func (f *fileValidationStoreStub) GetFoundSourceGameRecords(context.Context, []string) ([]*core.SourceGame, error) {
	return f.sourceGames, f.loadErr
}

func (f *fileValidationStoreStub) DeleteSourceGamesByID(_ context.Context, sourceGameIDs []string) error {
	f.deletedIDs = append([]string(nil), sourceGameIDs...)
	return f.deleteErr
}

type fileValidationCallerStub struct {
	files []map[string]any
	err   error
}

func (f *fileValidationCallerStub) Call(_ context.Context, _ string, method string, _ any, result any) error {
	if f.err != nil {
		return f.err
	}
	if method != sourceFilesystemListMethod {
		return errors.New("unexpected method: " + method)
	}
	payload, err := json.Marshal(map[string]any{"files": f.files})
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, result)
}

func newFileValidationTestService(store *fileValidationStoreStub, caller *fileValidationCallerStub) FileValidationService {
	return NewFileValidationService(
		&fileValidationRepoStub{integration: &core.Integration{
			ID:         "drive-1",
			PluginID:   "game-source-google-drive",
			Label:      "Orr's Games",
			ConfigJSON: `{"include_paths":[{"path":"Games","object_id":"folder-1","recursive":true}]}`,
		}},
		store,
		caller,
	)
}

func TestFileValidationMatchesStableObjectIDBeforeChangedPath(t *testing.T) {
	store := &fileValidationStoreStub{sourceGames: []*core.SourceGame{{
		ID:            "source-1",
		IntegrationID: "drive-1",
		PluginID:      "game-source-google-drive",
		RawTitle:      "Plasma Pong",
		RootPath:      "Games/Plasma Pong",
		Files: []core.GameFile{{
			Path:     "Games/Plasma Pong/game.zip",
			ObjectID: "file-1",
		}},
	}}}
	service := newFileValidationTestService(store, &fileValidationCallerStub{files: []map[string]any{{
		"path":      "Games/Renamed Plasma Pong/game.zip",
		"object_id": "file-1",
	}}})

	report, err := service.Validate(context.Background(), "drive-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalChecked != 1 || report.FilesChecked != 1 {
		t.Fatalf("counts = games %d files %d", report.TotalChecked, report.FilesChecked)
	}
	if len(report.Missing) != 0 || report.MissingFileCount != 0 {
		t.Fatalf("stable object ID should match moved file: %+v", report)
	}
}

func TestFileValidationReportsMissingFilesAndUnverifiableRecords(t *testing.T) {
	store := &fileValidationStoreStub{sourceGames: []*core.SourceGame{
		{
			ID:            "source-1",
			IntegrationID: "drive-1",
			PluginID:      "game-source-google-drive",
			RawTitle:      "Missing Game",
			RootPath:      "Games/Missing",
			Files: []core.GameFile{
				{Path: "Games/Missing/present.rom"},
				{Path: "Games/Missing/gone.rom"},
			},
		},
		{
			ID:            "source-2",
			IntegrationID: "drive-1",
			PluginID:      "game-source-google-drive",
			RawTitle:      "Needs Rescan",
		},
	}}
	service := newFileValidationTestService(store, &fileValidationCallerStub{files: []map[string]any{{
		"path": "Games/Missing/present.rom",
	}}})

	report, err := service.Validate(context.Background(), "drive-1")
	if err != nil {
		t.Fatal(err)
	}
	if report.MissingFileCount != 1 || len(report.Missing) != 1 {
		t.Fatalf("missing report = %+v", report)
	}
	if got := report.Missing[0].MissingFiles[0].Path; got != "Games/Missing/gone.rom" {
		t.Fatalf("missing path = %q", got)
	}
	if len(report.Failures) != 1 || report.Failures[0].SourceGameID != "source-2" {
		t.Fatalf("failures = %+v", report.Failures)
	}
}

func TestFileValidationFailsWithoutReclassifyingWhenConnectionListingFails(t *testing.T) {
	service := newFileValidationTestService(
		&fileValidationStoreStub{sourceGames: []*core.SourceGame{{ID: "source-1"}}},
		&fileValidationCallerStub{err: errors.New("OAuth token expired")},
	)

	report, err := service.Validate(context.Background(), "drive-1")
	if err == nil || report != nil {
		t.Fatalf("expected listing failure, report=%+v err=%v", report, err)
	}
}

func TestRemoveMissingRecordsRevalidatesAndNeverDeletesSourceFiles(t *testing.T) {
	store := &fileValidationStoreStub{sourceGames: []*core.SourceGame{{
		ID:            "source-1",
		IntegrationID: "drive-1",
		PluginID:      "game-source-google-drive",
		RawTitle:      "Missing Game",
		Files:         []core.GameFile{{Path: "Games/Missing/gone.rom"}},
	}}}
	service := newFileValidationTestService(store, &fileValidationCallerStub{})

	result, err := service.RemoveMissingRecords(context.Background(), "drive-1", []string{"source-1", "source-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(store.deletedIDs, []string{"source-1"}) {
		t.Fatalf("deleted IDs = %v", store.deletedIDs)
	}
	if !reflect.DeepEqual(result.RemovedSourceGameIDs, []string{"source-1"}) || result.RemainingMissing != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestRemoveMissingRecordsRejectsRecordThatIsPresent(t *testing.T) {
	store := &fileValidationStoreStub{sourceGames: []*core.SourceGame{{
		ID:            "source-1",
		IntegrationID: "drive-1",
		PluginID:      "game-source-google-drive",
		RawTitle:      "Present Game",
		Files:         []core.GameFile{{Path: "Games/Present/game.rom"}},
	}}}
	service := newFileValidationTestService(store, &fileValidationCallerStub{files: []map[string]any{{
		"path": "Games/Present/game.rom",
	}}})

	result, err := service.RemoveMissingRecords(context.Background(), "drive-1", []string{"source-1"})
	if !errors.Is(err, ErrFileValidationSelection) || result != nil {
		t.Fatalf("expected selection error, result=%+v err=%v", result, err)
	}
	if len(store.deletedIDs) != 0 {
		t.Fatalf("present record was deleted: %v", store.deletedIDs)
	}
}
