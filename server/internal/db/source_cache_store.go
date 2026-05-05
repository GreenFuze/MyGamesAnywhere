package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/google/uuid"
)

type sourceCacheStore struct {
	db core.Database
}

func NewSourceCacheStore(db core.Database) core.SourceCacheStore {
	return &sourceCacheStore{db: db}
}

func (s *sourceCacheStore) MarkInFlightJobsInterrupted(ctx context.Context) error {
	now := time.Now().Unix()
	_, err := s.db.GetDB().ExecContext(ctx, `UPDATE source_cache_jobs
		SET status='failed',
			message='interrupted by server restart',
			error='interrupted by server restart',
			updated_at=?,
			finished_at=?
		WHERE status IN ('queued', 'running')`, now, now)
	return err
}

func (s *sourceCacheStore) GetEntryBySourceProfile(ctx context.Context, sourceGameID, profile string) (*core.SourceCacheEntry, error) {
	query := `SELECT id, cache_key, canonical_game_id, canonical_title, source_game_id, source_title,
		integration_id, plugin_id, profile, mode, status, source_path, file_count, size, created_at, updated_at, last_accessed_at
		FROM source_cache_entries WHERE source_game_id=? AND profile=?`
	args := []any{sourceGameID, profile}
	if filter, filterArgs := sourceCacheProfileFilter(ctx, "source_cache_entries.source_game_id"); filter != "" {
		query += filter
		args = append(args, filterArgs...)
	}
	row := s.db.GetDB().QueryRowContext(ctx, query, args...)
	entry, err := scanSourceCacheEntry(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	files, err := s.loadEntryFiles(ctx, entry.ID)
	if err != nil {
		return nil, err
	}
	entry.Files = files
	return entry, nil
}

func (s *sourceCacheStore) GetEntryFileBySourceProfile(ctx context.Context, sourceGameID, profile, path string) (*core.SourceCacheEntry, *core.SourceCacheEntryFile, error) {
	query := `SELECT e.id, e.cache_key, e.canonical_game_id, e.canonical_title, e.source_game_id, e.source_title,
		e.integration_id, e.plugin_id, e.profile, e.mode, e.status, e.source_path, e.file_count, e.size, e.created_at, e.updated_at, e.last_accessed_at,
		f.entry_id, f.path, f.local_path, f.object_id, f.revision, f.modified_at, f.size
		FROM source_cache_entries e
		JOIN source_cache_entry_files f ON f.entry_id = e.id
		WHERE e.source_game_id=? AND e.profile=? AND f.path=?`
	args := []any{sourceGameID, profile, path}
	if filter, filterArgs := sourceCacheProfileFilter(ctx, "e.source_game_id"); filter != "" {
		query += filter
		args = append(args, filterArgs...)
	}
	row := s.db.GetDB().QueryRowContext(ctx, query, args...)
	var entry core.SourceCacheEntry
	var file core.SourceCacheEntryFile
	var canonicalGameID, canonicalTitle, sourceTitle, sourcePath sql.NullString
	var lastAccessedAt sql.NullInt64
	var createdAt, updatedAt int64
	var objectID, revision sql.NullString
	var modifiedAt sql.NullInt64
	if err := row.Scan(
		&entry.ID, &entry.CacheKey, &canonicalGameID, &canonicalTitle, &entry.SourceGameID, &sourceTitle,
		&entry.IntegrationID, &entry.PluginID, &entry.Profile, &entry.Mode, &entry.Status, &sourcePath, &entry.FileCount, &entry.Size,
		&createdAt, &updatedAt, &lastAccessedAt,
		&file.EntryID, &file.Path, &file.LocalPath, &objectID, &revision, &modifiedAt, &file.Size,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	entry.CanonicalGameID = canonicalGameID.String
	entry.CanonicalTitle = canonicalTitle.String
	entry.SourceTitle = sourceTitle.String
	entry.SourcePath = sourcePath.String
	entry.CreatedAt = time.Unix(createdAt, 0).UTC()
	entry.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	if lastAccessedAt.Valid {
		t := time.Unix(lastAccessedAt.Int64, 0).UTC()
		entry.LastAccessedAt = &t
	}
	file.ObjectID = objectID.String
	file.Revision = revision.String
	if modifiedAt.Valid {
		t := time.Unix(modifiedAt.Int64, 0).UTC()
		file.ModifiedAt = &t
	}
	return &entry, &file, nil
}

func (s *sourceCacheStore) UpsertEntry(ctx context.Context, entry *core.SourceCacheEntry) error {
	if entry == nil {
		return fmt.Errorf("entry is required")
	}
	if entry.ID == "" {
		entry.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now
	var lastAccessed any
	if entry.LastAccessedAt != nil {
		lastAccessed = entry.LastAccessedAt.Unix()
	}
	_, err := s.db.GetDB().ExecContext(ctx, `INSERT INTO source_cache_entries
		(id, cache_key, canonical_game_id, canonical_title, source_game_id, source_title, integration_id, plugin_id, profile, mode, status, source_path, file_count, size, created_at, updated_at, last_accessed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_game_id, profile) DO UPDATE SET
			cache_key = excluded.cache_key,
			canonical_game_id = excluded.canonical_game_id,
			canonical_title = excluded.canonical_title,
			source_title = excluded.source_title,
			integration_id = excluded.integration_id,
			plugin_id = excluded.plugin_id,
			mode = excluded.mode,
			status = excluded.status,
			source_path = excluded.source_path,
			file_count = excluded.file_count,
			size = excluded.size,
			updated_at = excluded.updated_at,
			last_accessed_at = excluded.last_accessed_at`,
		entry.ID, entry.CacheKey, nullEmpty(entry.CanonicalGameID), nullEmpty(entry.CanonicalTitle), entry.SourceGameID, nullEmpty(entry.SourceTitle),
		entry.IntegrationID, entry.PluginID, entry.Profile, entry.Mode, entry.Status, nullEmpty(entry.SourcePath), entry.FileCount, entry.Size,
		entry.CreatedAt.Unix(), entry.UpdatedAt.Unix(), lastAccessed)
	return err
}

func (s *sourceCacheStore) ReplaceEntryFiles(ctx context.Context, entryID string, files []core.SourceCacheEntryFile) error {
	tx, err := s.db.GetDB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM source_cache_entry_files WHERE entry_id=?`, entryID); err != nil {
		return err
	}
	for _, file := range files {
		var modifiedAt any
		if file.ModifiedAt != nil {
			modifiedAt = file.ModifiedAt.Unix()
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO source_cache_entry_files
			(entry_id, path, local_path, object_id, revision, modified_at, size)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			entryID, file.Path, file.LocalPath, nullEmpty(file.ObjectID), nullEmpty(file.Revision), modifiedAt, file.Size); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *sourceCacheStore) TouchEntry(ctx context.Context, entryID string, at time.Time) error {
	_, err := s.db.GetDB().ExecContext(ctx, `UPDATE source_cache_entries SET last_accessed_at=?, updated_at=? WHERE id=?`, at.Unix(), at.Unix(), entryID)
	return err
}

func (s *sourceCacheStore) ListEntries(ctx context.Context) ([]*core.SourceCacheEntry, error) {
	query := `SELECT id, cache_key, canonical_game_id, canonical_title, source_game_id, source_title,
		integration_id, plugin_id, profile, mode, status, source_path, file_count, size, created_at, updated_at, last_accessed_at
		FROM source_cache_entries WHERE 1=1`
	var args []any
	if filter, filterArgs := sourceCacheProfileFilter(ctx, "source_cache_entries.source_game_id"); filter != "" {
		query += filter
		args = append(args, filterArgs...)
	}
	query += ` ORDER BY updated_at DESC, id DESC`
	rows, err := s.db.GetDB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*core.SourceCacheEntry
	for rows.Next() {
		entry, err := scanSourceCacheEntry(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *sourceCacheStore) DeleteEntry(ctx context.Context, entryID string) error {
	query := `DELETE FROM source_cache_entries WHERE id=?`
	args := []any{entryID}
	if filter, filterArgs := sourceCacheProfileFilter(ctx, "source_cache_entries.source_game_id"); filter != "" {
		query += filter
		args = append(args, filterArgs...)
	}
	_, err := s.db.GetDB().ExecContext(ctx, query, args...)
	return err
}

func (s *sourceCacheStore) ClearEntries(ctx context.Context) error {
	query := `DELETE FROM source_cache_entries WHERE 1=1`
	var args []any
	if filter, filterArgs := sourceCacheProfileFilter(ctx, "source_cache_entries.source_game_id"); filter != "" {
		query += filter
		args = append(args, filterArgs...)
	}
	_, err := s.db.GetDB().ExecContext(ctx, query, args...)
	return err
}

func (s *sourceCacheStore) CreateJob(ctx context.Context, job *core.SourceCacheJobStatus) error {
	if job == nil {
		return fmt.Errorf("job is required")
	}
	if job.JobID == "" {
		job.JobID = uuid.NewString()
	}
	now := time.Now().UTC()
	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	}
	job.UpdatedAt = now
	var finishedAt any
	if job.FinishedAt != nil {
		finishedAt = job.FinishedAt.Unix()
	}
	_, err := s.db.GetDB().ExecContext(ctx, `INSERT INTO source_cache_jobs
		(job_id, cache_key, canonical_game_id, canonical_title, source_game_id, source_title, integration_id, plugin_id, profile, status, message, error, entry_id, progress_current, progress_total, created_at, updated_at, finished_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.JobID, nullEmpty(job.CacheKey), nullEmpty(job.CanonicalGameID), nullEmpty(job.CanonicalTitle), job.SourceGameID, nullEmpty(job.SourceTitle),
		nullEmpty(job.IntegrationID), nullEmpty(job.PluginID), job.Profile, job.Status, nullEmpty(job.Message), nullEmpty(job.Error), nullEmpty(job.EntryID),
		job.ProgressCurrent, job.ProgressTotal, job.CreatedAt.Unix(), job.UpdatedAt.Unix(), finishedAt)
	return err
}

func (s *sourceCacheStore) UpdateJob(ctx context.Context, job *core.SourceCacheJobStatus) error {
	if job == nil {
		return fmt.Errorf("job is required")
	}
	job.UpdatedAt = time.Now().UTC()
	var finishedAt any
	if job.FinishedAt != nil {
		finishedAt = job.FinishedAt.Unix()
	}
	_, err := s.db.GetDB().ExecContext(ctx, `UPDATE source_cache_jobs
		SET cache_key=?, canonical_game_id=?, canonical_title=?, source_title=?, integration_id=?, plugin_id=?,
			status=?, message=?, error=?, entry_id=?, progress_current=?, progress_total=?, updated_at=?, finished_at=?
		WHERE job_id=?`,
		nullEmpty(job.CacheKey), nullEmpty(job.CanonicalGameID), nullEmpty(job.CanonicalTitle), nullEmpty(job.SourceTitle), nullEmpty(job.IntegrationID), nullEmpty(job.PluginID),
		job.Status, nullEmpty(job.Message), nullEmpty(job.Error), nullEmpty(job.EntryID), job.ProgressCurrent, job.ProgressTotal, job.UpdatedAt.Unix(), finishedAt, job.JobID)
	return err
}

func (s *sourceCacheStore) GetJob(ctx context.Context, jobID string) (*core.SourceCacheJobStatus, error) {
	query := `SELECT job_id, cache_key, canonical_game_id, canonical_title, source_game_id, source_title, integration_id, plugin_id,
		profile, status, message, error, entry_id, progress_current, progress_total, created_at, updated_at, finished_at
		FROM source_cache_jobs WHERE job_id=?`
	args := []any{jobID}
	if filter, filterArgs := sourceCacheProfileFilter(ctx, "source_cache_jobs.source_game_id"); filter != "" {
		query += filter
		args = append(args, filterArgs...)
	}
	row := s.db.GetDB().QueryRowContext(ctx, query, args...)
	job, err := scanSourceCacheJob(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return job, nil
}

func (s *sourceCacheStore) ListJobs(ctx context.Context, limit int) ([]*core.SourceCacheJobStatus, error) {
	if limit <= 0 {
		limit = 25
	}
	query := `SELECT job_id, cache_key, canonical_game_id, canonical_title, source_game_id, source_title, integration_id, plugin_id,
		profile, status, message, error, entry_id, progress_current, progress_total, created_at, updated_at, finished_at
		FROM source_cache_jobs WHERE 1=1`
	var args []any
	if filter, filterArgs := sourceCacheProfileFilter(ctx, "source_cache_jobs.source_game_id"); filter != "" {
		query += filter
		args = append(args, filterArgs...)
	}
	query += ` ORDER BY updated_at DESC, job_id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.GetDB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []*core.SourceCacheJobStatus
	for rows.Next() {
		job, err := scanSourceCacheJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (s *sourceCacheStore) FindActiveJobByCacheKey(ctx context.Context, cacheKey string) (*core.SourceCacheJobStatus, error) {
	query := `SELECT job_id, cache_key, canonical_game_id, canonical_title, source_game_id, source_title, integration_id, plugin_id,
		profile, status, message, error, entry_id, progress_current, progress_total, created_at, updated_at, finished_at
		FROM source_cache_jobs
		WHERE cache_key=? AND status IN ('queued', 'running')`
	args := []any{cacheKey}
	if filter, filterArgs := sourceCacheProfileFilter(ctx, "source_cache_jobs.source_game_id"); filter != "" {
		query += filter
		args = append(args, filterArgs...)
	}
	query += `
		ORDER BY updated_at DESC
		LIMIT 1`
	row := s.db.GetDB().QueryRowContext(ctx, query, args...)
	job, err := scanSourceCacheJob(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return job, nil
}

func sourceCacheProfileFilter(ctx context.Context, sourceGameColumn string) (string, []any) {
	profileID := core.ProfileIDFromContext(ctx)
	if profileID == "" {
		return "", nil
	}
	return fmt.Sprintf(` AND EXISTS (
		SELECT 1 FROM source_games sg
		WHERE sg.id = %s AND sg.profile_id = ?
	)`, sourceGameColumn), []any{profileID}
}

func (s *sourceCacheStore) loadEntryFiles(ctx context.Context, entryID string) ([]core.SourceCacheEntryFile, error) {
	rows, err := s.db.GetDB().QueryContext(ctx, `SELECT entry_id, path, local_path, object_id, revision, modified_at, size
		FROM source_cache_entry_files WHERE entry_id=? ORDER BY path`, entryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	files := make([]core.SourceCacheEntryFile, 0)
	for rows.Next() {
		var file core.SourceCacheEntryFile
		var objectID, revision sql.NullString
		var modifiedAt sql.NullInt64
		if err := rows.Scan(&file.EntryID, &file.Path, &file.LocalPath, &objectID, &revision, &modifiedAt, &file.Size); err != nil {
			return nil, err
		}
		file.ObjectID = objectID.String
		file.Revision = revision.String
		if modifiedAt.Valid {
			t := time.Unix(modifiedAt.Int64, 0).UTC()
			file.ModifiedAt = &t
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSourceCacheEntry(row scanner) (*core.SourceCacheEntry, error) {
	var entry core.SourceCacheEntry
	var canonicalGameID, canonicalTitle, sourceTitle, sourcePath sql.NullString
	var createdAt, updatedAt int64
	var lastAccessedAt sql.NullInt64
	if err := row.Scan(&entry.ID, &entry.CacheKey, &canonicalGameID, &canonicalTitle, &entry.SourceGameID, &sourceTitle,
		&entry.IntegrationID, &entry.PluginID, &entry.Profile, &entry.Mode, &entry.Status, &sourcePath, &entry.FileCount, &entry.Size,
		&createdAt, &updatedAt, &lastAccessedAt); err != nil {
		return nil, err
	}
	entry.CanonicalGameID = canonicalGameID.String
	entry.CanonicalTitle = canonicalTitle.String
	entry.SourceTitle = sourceTitle.String
	entry.SourcePath = sourcePath.String
	entry.CreatedAt = time.Unix(createdAt, 0).UTC()
	entry.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	if lastAccessedAt.Valid {
		t := time.Unix(lastAccessedAt.Int64, 0).UTC()
		entry.LastAccessedAt = &t
	}
	return &entry, nil
}

func scanSourceCacheJob(row scanner) (*core.SourceCacheJobStatus, error) {
	var job core.SourceCacheJobStatus
	var cacheKey, canonicalGameID, canonicalTitle, sourceTitle, integrationID, pluginID sql.NullString
	var message, jobError, entryID sql.NullString
	var createdAt, updatedAt int64
	var finishedAt sql.NullInt64
	if err := row.Scan(&job.JobID, &cacheKey, &canonicalGameID, &canonicalTitle, &job.SourceGameID, &sourceTitle, &integrationID, &pluginID,
		&job.Profile, &job.Status, &message, &jobError, &entryID, &job.ProgressCurrent, &job.ProgressTotal, &createdAt, &updatedAt, &finishedAt); err != nil {
		return nil, err
	}
	job.CacheKey = cacheKey.String
	job.CanonicalGameID = canonicalGameID.String
	job.CanonicalTitle = canonicalTitle.String
	job.SourceTitle = sourceTitle.String
	job.IntegrationID = integrationID.String
	job.PluginID = pluginID.String
	job.Message = message.String
	job.Error = jobError.String
	job.EntryID = entryID.String
	job.CreatedAt = time.Unix(createdAt, 0).UTC()
	job.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	if finishedAt.Valid {
		t := time.Unix(finishedAt.Int64, 0).UTC()
		job.FinishedAt = &t
	}
	return &job, nil
}
