package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/google/uuid"
)

type sourceMoveStore struct {
	db core.Database
}

func NewSourceMoveStore(database core.Database) core.SourceMoveStore {
	return &sourceMoveStore{db: database}
}

func (s *sourceMoveStore) MarkInFlightJobsInterrupted(ctx context.Context) error {
	now := time.Now().UTC().Unix()
	_, err := s.db.GetDB().ExecContext(ctx, `UPDATE source_move_jobs
		SET recovery_phase=status,
			status=?,
			message='Move interrupted by server restart',
			error='Move interrupted by server restart',
			updated_at=?
		WHERE status IN (?, ?, ?, ?, ?, ?)`,
		core.SourceMoveStatusInterrupted, now,
		core.SourceMoveStatusQueued,
		core.SourceMoveStatusMaterializingSource,
		core.SourceMoveStatusStagingDestination,
		core.SourceMoveStatusDestinationCommitted,
		core.SourceMoveStatusDeletingSource,
		core.SourceMoveStatusRefreshingLibrary,
	)
	return err
}

func (s *sourceMoveStore) CreateJob(ctx context.Context, job *core.SourceMoveJob) error {
	if job == nil {
		return fmt.Errorf("source move job is required")
	}
	profileID := strings.TrimSpace(core.ProfileIDFromContext(ctx))
	if profileID == "" {
		return core.ErrProfileRequired
	}
	if strings.TrimSpace(job.ID) == "" {
		job.ID = uuid.NewString()
	}
	if strings.TrimSpace(job.TransferID) == "" {
		job.TransferID = uuid.NewString()
	}
	now := time.Now().UTC()
	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	}
	job.UpdatedAt = now
	job.ProfileID = profileID

	tx, err := s.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin source move create: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `INSERT INTO source_move_jobs (
		id, profile_id, transfer_id, canonical_game_id, canonical_title,
		source_game_id, source_title, source_integration_id, source_plugin_id, source_root_path,
		destination_integration_id, destination_plugin_id, destination_authority, destination_label, destination_path,
		status, message, error, recovery_phase, whole_directory, keep_both, progress_current, progress_total,
		created_at, updated_at, finished_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.ProfileID, job.TransferID, job.CanonicalGameID, job.CanonicalTitle,
		job.SourceGameID, job.SourceTitle, job.SourceIntegrationID, job.SourcePluginID, job.SourceRootPath,
		job.DestinationIntegrationID, job.DestinationPluginID, job.DestinationAuthority, job.DestinationLabel, job.DestinationPath,
		job.Status, job.Message, job.Error, job.RecoveryPhase, boolToInt(job.WholeDirectory), boolToInt(job.KeepBoth),
		job.ProgressCurrent, job.ProgressTotal, job.CreatedAt.Unix(), job.UpdatedAt.Unix(), nullableUnix(job.FinishedAt),
	); err != nil {
		return fmt.Errorf("insert source move job: %w", err)
	}
	if err := replaceSourceMoveFiles(ctx, tx, job.ID, job.Files); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit source move create: %w", err)
	}
	return nil
}

func (s *sourceMoveStore) UpdateJob(ctx context.Context, job *core.SourceMoveJob) error {
	if job == nil || strings.TrimSpace(job.ID) == "" {
		return fmt.Errorf("source move job id is required")
	}
	profileID := strings.TrimSpace(core.ProfileIDFromContext(ctx))
	if profileID == "" {
		return core.ErrProfileRequired
	}
	job.UpdatedAt = time.Now().UTC()
	result, err := s.db.GetDB().ExecContext(ctx, `UPDATE source_move_jobs SET
		status=?, message=?, error=?, recovery_phase=?, whole_directory=?, keep_both=?,
		progress_current=?, progress_total=?, updated_at=?, finished_at=?
		WHERE id=? AND profile_id=?`,
		job.Status, job.Message, job.Error, job.RecoveryPhase, boolToInt(job.WholeDirectory), boolToInt(job.KeepBoth),
		job.ProgressCurrent, job.ProgressTotal, job.UpdatedAt.Unix(), nullableUnix(job.FinishedAt),
		job.ID, profileID,
	)
	if err != nil {
		return fmt.Errorf("update source move job: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *sourceMoveStore) DeleteJob(ctx context.Context, jobID string) error {
	profileID := strings.TrimSpace(core.ProfileIDFromContext(ctx))
	if profileID == "" {
		return core.ErrProfileRequired
	}
	result, err := s.db.GetDB().ExecContext(ctx, `DELETE FROM source_move_jobs
		WHERE id=? AND profile_id=? AND status=?`, jobID, profileID, core.SourceMoveStatusQueued)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *sourceMoveStore) ReplaceFiles(ctx context.Context, jobID string, files []core.SourceMoveFile) error {
	profileID := strings.TrimSpace(core.ProfileIDFromContext(ctx))
	if profileID == "" {
		return core.ErrProfileRequired
	}
	tx, err := s.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT 1 FROM source_move_jobs WHERE id=? AND profile_id=?`, jobID, profileID).Scan(&exists); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM source_move_job_files WHERE job_id=?`, jobID); err != nil {
		return err
	}
	if err := replaceSourceMoveFiles(ctx, tx, jobID, files); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *sourceMoveStore) UpdateFile(ctx context.Context, jobID string, file core.SourceMoveFile) error {
	profileID := strings.TrimSpace(core.ProfileIDFromContext(ctx))
	if profileID == "" {
		return core.ErrProfileRequired
	}
	result, err := s.db.GetDB().ExecContext(ctx, `UPDATE source_move_job_files SET
		sha256=?, status=?, error=?
		WHERE job_id=? AND ordinal=? AND EXISTS (
			SELECT 1 FROM source_move_jobs WHERE id=? AND profile_id=?
		)`, file.SHA256, file.Status, file.Error, jobID, file.Ordinal, jobID, profileID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *sourceMoveStore) GetJob(ctx context.Context, jobID string) (*core.SourceMoveJob, error) {
	profileID := strings.TrimSpace(core.ProfileIDFromContext(ctx))
	if profileID == "" {
		return nil, core.ErrProfileRequired
	}
	job, err := scanSourceMoveJob(s.db.GetDB().QueryRowContext(ctx, sourceMoveJobSelect+` WHERE id=? AND profile_id=?`, jobID, profileID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	job.Files, err = s.loadFiles(ctx, job.ID)
	return job, err
}

func (s *sourceMoveStore) ListJobs(ctx context.Context, limit int) ([]*core.SourceMoveJob, error) {
	profileID := strings.TrimSpace(core.ProfileIDFromContext(ctx))
	if profileID == "" {
		return nil, core.ErrProfileRequired
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.db.GetDB().QueryContext(ctx, sourceMoveJobSelect+` WHERE profile_id=? ORDER BY created_at DESC, id DESC LIMIT ?`, profileID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]*core.SourceMoveJob, 0)
	for rows.Next() {
		job, err := scanSourceMoveJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, job := range jobs {
		job.Files, err = s.loadFiles(ctx, job.ID)
		if err != nil {
			return nil, err
		}
	}
	return jobs, nil
}

const sourceMoveJobSelect = `SELECT id, profile_id, transfer_id, canonical_game_id, canonical_title,
	source_game_id, source_title, source_integration_id, source_plugin_id, source_root_path,
	destination_integration_id, destination_plugin_id, destination_authority, destination_label, destination_path,
	status, message, error, recovery_phase, whole_directory, keep_both, progress_current, progress_total,
	created_at, updated_at, finished_at FROM source_move_jobs`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanSourceMoveJob(row rowScanner) (*core.SourceMoveJob, error) {
	var job core.SourceMoveJob
	var wholeDirectory, keepBoth int
	var createdAt, updatedAt int64
	var finishedAt sql.NullInt64
	if err := row.Scan(
		&job.ID, &job.ProfileID, &job.TransferID, &job.CanonicalGameID, &job.CanonicalTitle,
		&job.SourceGameID, &job.SourceTitle, &job.SourceIntegrationID, &job.SourcePluginID, &job.SourceRootPath,
		&job.DestinationIntegrationID, &job.DestinationPluginID, &job.DestinationAuthority, &job.DestinationLabel, &job.DestinationPath,
		&job.Status, &job.Message, &job.Error, &job.RecoveryPhase, &wholeDirectory, &keepBoth,
		&job.ProgressCurrent, &job.ProgressTotal, &createdAt, &updatedAt, &finishedAt,
	); err != nil {
		return nil, err
	}
	job.WholeDirectory = wholeDirectory != 0
	job.KeepBoth = keepBoth != 0
	job.CreatedAt = time.Unix(createdAt, 0).UTC()
	job.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	if finishedAt.Valid {
		value := time.Unix(finishedAt.Int64, 0).UTC()
		job.FinishedAt = &value
	}
	return &job, nil
}

func (s *sourceMoveStore) loadFiles(ctx context.Context, jobID string) ([]core.SourceMoveFile, error) {
	rows, err := s.db.GetDB().QueryContext(ctx, `SELECT ordinal, source_path, relative_path, size,
		object_id, revision, sha256, status, error FROM source_move_job_files
		WHERE job_id=? ORDER BY ordinal`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := make([]core.SourceMoveFile, 0)
	for rows.Next() {
		var file core.SourceMoveFile
		if err := rows.Scan(&file.Ordinal, &file.SourcePath, &file.RelativePath, &file.Size,
			&file.ObjectID, &file.Revision, &file.SHA256, &file.Status, &file.Error); err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

func replaceSourceMoveFiles(ctx context.Context, tx *sql.Tx, jobID string, files []core.SourceMoveFile) error {
	for index := range files {
		file := files[index]
		file.Ordinal = index
		if strings.TrimSpace(file.Status) == "" {
			file.Status = "pending"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO source_move_job_files (
			job_id, ordinal, source_path, relative_path, size, object_id, revision, sha256, status, error
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			jobID, file.Ordinal, file.SourcePath, file.RelativePath, file.Size,
			file.ObjectID, file.Revision, file.SHA256, file.Status, file.Error,
		); err != nil {
			return fmt.Errorf("insert source move file: %w", err)
		}
	}
	return nil
}

func nullableUnix(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Unix()
}
