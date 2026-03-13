package db

import (
	"context"
	"database/sql"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type gameRepository struct {
	db core.Database
}

func NewGameRepository(db core.Database) core.GameRepository {
	return &gameRepository{db: db}
}

func (r *gameRepository) UpsertGames(ctx context.Context, games []*core.Game, files []*core.GameFile) error {
	db := r.db.GetDB()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	for _, g := range games {
		if g == nil || g.ID == "" {
			continue
		}
		var lastSeen *int64
		if g.LastSeenAt != nil {
			u := g.LastSeenAt.Unix()
			lastSeen = &u
		} else {
			lastSeen = &now
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO games (id, title, platform, kind, parent_game_id, package_kind, root_path, integration_id, status, last_seen_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				title = excluded.title,
				platform = excluded.platform,
				kind = excluded.kind,
				parent_game_id = excluded.parent_game_id,
				package_kind = excluded.package_kind,
				root_path = excluded.root_path,
				integration_id = excluded.integration_id,
				status = 'found',
				last_seen_at = excluded.last_seen_at`,
			g.ID, nullEmptyStr(g.Title), string(g.Platform), string(g.Kind), nullEmptyStr(g.ParentGameID), string(g.GroupKind),
			nullEmptyStr(g.RootPath), nullEmptyStr(g.IntegrationID), g.Status, *lastSeen)
		if err != nil {
			return err
		}
	}

	// Replace game_files for each game that we upserted
	gameIDs := make(map[string]bool)
	for _, g := range games {
		if g != nil && g.ID != "" {
			gameIDs[g.ID] = true
		}
	}
	for gid := range gameIDs {
		if _, err := tx.ExecContext(ctx, "DELETE FROM game_files WHERE game_id = ?", gid); err != nil {
			return err
		}
	}
	for _, f := range files {
		if f == nil || f.GameID == "" || f.Path == "" {
			continue
		}
		if !gameIDs[f.GameID] {
			continue
		}
		isDir := 0
		if f.IsDir {
			isDir = 1
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO game_files (game_id, path, file_name, role, file_kind, size, is_dir)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			f.GameID, f.Path, f.FileName, string(f.Role), nullEmptyStr(f.FileKind), f.Size, isDir)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *gameRepository) MarkGamesNotFoundExcept(ctx context.Context, keepGameIDs []string) error {
	if len(keepGameIDs) == 0 {
		_, err := r.db.GetDB().ExecContext(ctx, "UPDATE games SET status = 'not_found'")
		return err
	}
	placeholders := buildPlaceholders(len(keepGameIDs))
	query := "UPDATE games SET status = 'not_found' WHERE id NOT IN (" + placeholders + ")"
	args := make([]any, len(keepGameIDs))
	for i, id := range keepGameIDs {
		args[i] = id
	}
	_, err := r.db.GetDB().ExecContext(ctx, query, args...)
	return err
}

func (r *gameRepository) MarkScanGamesNotFoundExcept(ctx context.Context, keepScanGameIDs []string) error {
	prefix := "scan:"
	if len(keepScanGameIDs) == 0 {
		_, err := r.db.GetDB().ExecContext(ctx, "UPDATE games SET status = 'not_found' WHERE id LIKE ?", prefix+"%")
		return err
	}
	placeholders := buildPlaceholders(len(keepScanGameIDs))
	query := "UPDATE games SET status = 'not_found' WHERE id LIKE ? AND id NOT IN (" + placeholders + ")"
	args := make([]any, 0, len(keepScanGameIDs)+1)
	args = append(args, prefix+"%")
	for _, id := range keepScanGameIDs {
		args = append(args, id)
	}
	_, err := r.db.GetDB().ExecContext(ctx, query, args...)
	return err
}

func (r *gameRepository) MarkPluginGamesNotFoundExcept(ctx context.Context, integrationID string, keepGameIDs []string) error {
	prefix := "plugin:" + integrationID + ":"
	if len(keepGameIDs) == 0 {
		_, err := r.db.GetDB().ExecContext(ctx, "UPDATE games SET status = 'not_found' WHERE id LIKE ?", prefix+"%")
		return err
	}
	placeholders := buildPlaceholders(len(keepGameIDs))
	query := "UPDATE games SET status = 'not_found' WHERE id LIKE ? AND id NOT IN (" + placeholders + ")"
	args := make([]any, 0, len(keepGameIDs)+1)
	args = append(args, prefix+"%")
	for _, id := range keepGameIDs {
		args = append(args, id)
	}
	_, err := r.db.GetDB().ExecContext(ctx, query, args...)
	return err
}

func (r *gameRepository) DeleteAllGames(ctx context.Context) error {
	db := r.db.GetDB()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "DELETE FROM game_files"); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM games"); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *gameRepository) GetGames(ctx context.Context) ([]*core.Game, error) {
	rows, err := r.db.GetDB().QueryContext(ctx, `SELECT id, title, platform, kind, parent_game_id, package_kind, root_path, integration_id, status, last_seen_at FROM games WHERE status = 'found'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*core.Game
	for rows.Next() {
		g, err := scanGame(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (r *gameRepository) GetGameByID(ctx context.Context, gameID string) (*core.Game, error) {
	row := r.db.GetDB().QueryRowContext(ctx, `SELECT id, title, platform, kind, parent_game_id, package_kind, root_path, integration_id, status, last_seen_at FROM games WHERE id = ?`, gameID)
	g, err := scanGameRow(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return g, nil
}

func (r *gameRepository) GetGameFiles(ctx context.Context, gameID string) ([]*core.GameFile, error) {
	rows, err := r.db.GetDB().QueryContext(ctx, `SELECT game_id, path, file_name, role, file_kind, size, is_dir FROM game_files WHERE game_id = ?`, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*core.GameFile
	for rows.Next() {
		var f core.GameFile
		var isDir int
		err := rows.Scan(&f.GameID, &f.Path, &f.FileName, &f.Role, &f.FileKind, &f.Size, &isDir)
		if err != nil {
			return nil, err
		}
		f.IsDir = isDir != 0
		if f.Role == "" {
			f.Role = core.GameFileRoleOptional
		}
		out = append(out, &f)
	}
	return out, rows.Err()
}

func scanGame(rows *sql.Rows) (*core.Game, error) {
	var g core.Game
	var lastSeen *int64
	var title, parentGameID, rootPath, integrationID sql.NullString
	var platformStr, kindStr, groupKindStr string
	err := rows.Scan(&g.ID, &title, &platformStr, &kindStr, &parentGameID, &groupKindStr, &rootPath, &integrationID, &g.Status, &lastSeen)
	if err != nil {
		return nil, err
	}
	g.Title = title.String
	g.Platform = core.Platform(platformStr)
	g.Kind = core.GameKind(kindStr)
	g.ParentGameID = parentGameID.String
	g.GroupKind = core.GroupKind(groupKindStr)
	g.RootPath = rootPath.String
	g.IntegrationID = integrationID.String
	if lastSeen != nil {
		t := time.Unix(*lastSeen, 0)
		g.LastSeenAt = &t
	}
	if g.Kind == "" {
		g.Kind = core.GameKindUnknown
	}
	if g.Platform == "" {
		g.Platform = core.PlatformUnknown
	}
	return &g, nil
}

func scanGameRow(row *sql.Row) (*core.Game, error) {
	var g core.Game
	var lastSeen *int64
	var title, parentGameID, rootPath, integrationID sql.NullString
	var platformStr, kindStr, groupKindStr string
	err := row.Scan(&g.ID, &title, &platformStr, &kindStr, &parentGameID, &groupKindStr, &rootPath, &integrationID, &g.Status, &lastSeen)
	if err != nil {
		return nil, err
	}
	g.Title = title.String
	g.Platform = core.Platform(platformStr)
	g.Kind = core.GameKind(kindStr)
	g.ParentGameID = parentGameID.String
	g.GroupKind = core.GroupKind(groupKindStr)
	g.RootPath = rootPath.String
	g.IntegrationID = integrationID.String
	if lastSeen != nil {
		t := time.Unix(*lastSeen, 0)
		g.LastSeenAt = &t
	}
	if g.Kind == "" {
		g.Kind = core.GameKindUnknown
	}
	if g.Platform == "" {
		g.Platform = core.PlatformUnknown
	}
	return &g, nil
}
