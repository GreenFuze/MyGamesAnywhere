package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/contentdelivery"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type ContentRepository struct {
	db core.Database
}

func NewContentRepository(database core.Database) *ContentRepository {
	return &ContentRepository{db: database}
}

func (r *ContentRepository) GetCopy(ctx context.Context, copyID string) (*contentdelivery.Copy, error) {
	if r == nil || r.db == nil {
		return nil, fmt.Errorf("content repository database is required")
	}
	profileID := strings.TrimSpace(core.ProfileIDFromContext(ctx))
	copyID = strings.TrimSpace(copyID)
	if profileID == "" || copyID == "" {
		return nil, nil
	}

	var source core.SourceGame
	var canonicalGameID string
	var rootPath sql.NullString
	var lastSeenAt sql.NullInt64
	var createdAt int64
	err := r.db.GetDB().QueryRowContext(ctx, `
		SELECT l.canonical_id, sg.id, sg.integration_id, sg.plugin_id, sg.external_id,
		       sg.raw_title, sg.platform, sg.kind, sg.group_kind, sg.root_path,
		       sg.status, sg.last_seen_at, sg.created_at
		FROM source_games sg
		JOIN canonical_source_games_link l ON l.source_game_id = sg.id
		WHERE sg.id = ? AND sg.profile_id = ? AND sg.status = 'found'
		  AND IFNULL(sg.review_state, 'pending') != 'not_a_game'
		  AND (SELECT COUNT(*) FROM canonical_source_games_link identity_links
		       WHERE identity_links.source_game_id = sg.id) = 1
		LIMIT 1`, copyID, profileID).Scan(
		&canonicalGameID,
		&source.ID,
		&source.IntegrationID,
		&source.PluginID,
		&source.ExternalID,
		&source.RawTitle,
		&source.Platform,
		&source.Kind,
		&source.GroupKind,
		&rootPath,
		&source.Status,
		&lastSeenAt,
		&createdAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query content copy: %w", err)
	}
	source.RootPath = rootPath.String
	source.CreatedAt = time.Unix(createdAt, 0).UTC()
	if lastSeenAt.Valid {
		value := time.Unix(lastSeenAt.Int64, 0).UTC()
		source.LastSeenAt = &value
	}

	rows, err := r.db.GetDB().QueryContext(ctx, `
		SELECT path, file_name, role, file_kind, size, is_dir, object_id, revision, modified_at
		FROM game_files
		WHERE source_game_id = ?
		ORDER BY path, file_name`, source.ID)
	if err != nil {
		return nil, fmt.Errorf("query content files: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var file core.GameFile
		var isDir int
		var fileKind, objectID, revision sql.NullString
		var modifiedAt sql.NullInt64
		if err := rows.Scan(
			&file.Path,
			&file.FileName,
			&file.Role,
			&fileKind,
			&file.Size,
			&isDir,
			&objectID,
			&revision,
			&modifiedAt,
		); err != nil {
			return nil, fmt.Errorf("scan content file: %w", err)
		}
		file.GameID = source.ID
		file.FileKind = fileKind.String
		file.IsDir = isDir != 0
		file.ObjectID = objectID.String
		file.Revision = revision.String
		if modifiedAt.Valid {
			value := time.Unix(modifiedAt.Int64, 0).UTC()
			file.ModifiedAt = &value
		}
		source.Files = append(source.Files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate content files: %w", err)
	}

	return &contentdelivery.Copy{CanonicalGameID: canonicalGameID, SourceGame: &source}, nil
}
