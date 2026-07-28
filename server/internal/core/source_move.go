package core

import (
	"context"
	"time"
)

const (
	SourceMoveStatusQueued                = "queued"
	SourceMoveStatusMaterializingSource   = "materializing_source"
	SourceMoveStatusStagingDestination    = "staging_destination"
	SourceMoveStatusDestinationCommitted  = "destination_committed"
	SourceMoveStatusDeletingSource        = "deleting_source"
	SourceMoveStatusRefreshingLibrary     = "refreshing_library"
	SourceMoveStatusCompleted             = "completed"
	SourceMoveStatusFailedBeforeCommit    = "failed_before_commit"
	SourceMoveStatusSourceCleanupRequired = "source_cleanup_required"
	SourceMoveStatusInterrupted           = "interrupted"
)

type SourceMoveSelection struct {
	CanonicalGameID          string `json:"canonical_game_id"`
	SourceGameID             string `json:"source_game_id"`
	DestinationIntegrationID string `json:"destination_integration_id"`
	DestinationPath          string `json:"destination_path"`
}

type SourceMovePreviewRequest struct {
	Items []SourceMoveSelection `json:"items"`
}

type SourceMovePreviewItem struct {
	CanonicalGameID          string           `json:"canonical_game_id"`
	CanonicalTitle           string           `json:"canonical_title"`
	SourceGameID             string           `json:"source_game_id"`
	SourceTitle              string           `json:"source_title"`
	SourceIntegrationID      string           `json:"source_integration_id"`
	SourcePluginID           string           `json:"source_plugin_id"`
	SourceRootPath           string           `json:"source_root_path"`
	DestinationIntegrationID string           `json:"destination_integration_id"`
	DestinationPluginID      string           `json:"destination_plugin_id"`
	DestinationAuthority     string           `json:"-"`
	DestinationLabel         string           `json:"destination_label"`
	DestinationPath          string           `json:"destination_path"`
	FileCount                int              `json:"file_count"`
	TotalSize                int64            `json:"total_size"`
	WholeDirectory           bool             `json:"whole_directory"`
	CanMove                  bool             `json:"can_move"`
	Reason                   string           `json:"reason,omitempty"`
	Warnings                 []string         `json:"warnings,omitempty"`
	SourceAction             string           `json:"source_action,omitempty"`
	SourceSummary            string           `json:"source_summary,omitempty"`
	Files                    []SourceMoveFile `json:"files,omitempty"`
}

type SourceMovePreview struct {
	Items []SourceMovePreviewItem `json:"items"`
}

type SourceMoveDestination struct {
	IntegrationID string `json:"integration_id"`
	PluginID      string `json:"plugin_id"`
	Label         string `json:"label"`
	SuggestedRoot string `json:"suggested_root,omitempty"`
}

type SourceMoveStartRequest struct {
	Items []SourceMoveSelection `json:"items"`
}

type SourceMoveFile struct {
	Ordinal      int    `json:"ordinal"`
	SourcePath   string `json:"source_path"`
	RelativePath string `json:"relative_path"`
	Size         int64  `json:"size"`
	ObjectID     string `json:"object_id,omitempty"`
	Revision     string `json:"revision,omitempty"`
	SHA256       string `json:"sha256,omitempty"`
	Status       string `json:"status"`
	Error        string `json:"error,omitempty"`
}

type SourceMoveJob struct {
	ID                       string           `json:"id"`
	ProfileID                string           `json:"profile_id,omitempty"`
	TransferID               string           `json:"transfer_id"`
	CanonicalGameID          string           `json:"canonical_game_id"`
	CanonicalTitle           string           `json:"canonical_title"`
	SourceGameID             string           `json:"source_game_id"`
	SourceTitle              string           `json:"source_title"`
	SourceIntegrationID      string           `json:"source_integration_id"`
	SourcePluginID           string           `json:"source_plugin_id"`
	SourceRootPath           string           `json:"source_root_path"`
	DestinationIntegrationID string           `json:"destination_integration_id"`
	DestinationPluginID      string           `json:"destination_plugin_id"`
	DestinationAuthority     string           `json:"-"`
	DestinationLabel         string           `json:"destination_label"`
	DestinationPath          string           `json:"destination_path"`
	Status                   string           `json:"status"`
	Message                  string           `json:"message,omitempty"`
	Error                    string           `json:"error,omitempty"`
	RecoveryPhase            string           `json:"recovery_phase,omitempty"`
	WholeDirectory           bool             `json:"whole_directory"`
	KeepBoth                 bool             `json:"keep_both"`
	ProgressCurrent          int              `json:"progress_current"`
	ProgressTotal            int              `json:"progress_total"`
	CreatedAt                time.Time        `json:"created_at"`
	UpdatedAt                time.Time        `json:"updated_at"`
	FinishedAt               *time.Time       `json:"finished_at,omitempty"`
	Files                    []SourceMoveFile `json:"files,omitempty"`
}

type SourceMoveStore interface {
	MarkInFlightJobsInterrupted(ctx context.Context) error
	CreateJob(ctx context.Context, job *SourceMoveJob) error
	DeleteJob(ctx context.Context, jobID string) error
	UpdateJob(ctx context.Context, job *SourceMoveJob) error
	ReplaceFiles(ctx context.Context, jobID string, files []SourceMoveFile) error
	UpdateFile(ctx context.Context, jobID string, file SourceMoveFile) error
	GetJob(ctx context.Context, jobID string) (*SourceMoveJob, error)
	ListJobs(ctx context.Context, limit int) ([]*SourceMoveJob, error)
}

type SourceMoveService interface {
	ListDestinations(ctx context.Context) ([]SourceMoveDestination, error)
	Preview(ctx context.Context, req SourceMovePreviewRequest) (*SourceMovePreview, error)
	Start(ctx context.Context, req SourceMoveStartRequest) ([]*SourceMoveJob, error)
	GetJob(ctx context.Context, jobID string) (*SourceMoveJob, error)
	ListJobs(ctx context.Context, limit int) ([]*SourceMoveJob, error)
	Retry(ctx context.Context, jobID string) (*SourceMoveJob, error)
	CleanupStage(ctx context.Context, jobID string) (*SourceMoveJob, error)
	KeepBoth(ctx context.Context, jobID string) (*SourceMoveJob, error)
}
