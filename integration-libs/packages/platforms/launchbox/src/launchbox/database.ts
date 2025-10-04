/**
 * LaunchBox Database Manager
 * Manages SQLite database for LaunchBox metadata
 */

import Database from 'better-sqlite3';
import * as fs from 'fs';
import * as path from 'path';
import { homedir } from 'os';
import type { LaunchBoxMetadata } from '../types/index.js';

/**
 * LaunchBox database configuration
 */
export interface LaunchBoxDBConfig {
  /**
   * Path to SQLite database file
   * Default: ~/.mygamesanywhere/metadata/launchbox/launchbox.db
   */
  dbPath?: string;
}

/**
 * LaunchBox database manager
 */
export class LaunchBoxDB {
  private db: Database.Database;
  private dbPath: string;

  constructor(config?: LaunchBoxDBConfig) {
    this.dbPath =
      config?.dbPath ||
      path.join(homedir(), '.mygamesanywhere', 'metadata', 'launchbox', 'launchbox.db');

    // Ensure directory exists
    const dbDir = path.dirname(this.dbPath);
    fs.mkdirSync(dbDir, { recursive: true });

    // Open database
    this.db = new Database(this.dbPath);
    this.db.pragma('journal_mode = WAL'); // Write-Ahead Logging for better performance

    // Initialize schema
    this.initSchema();
  }

  /**
   * Initialize database schema
   */
  private initSchema(): void {
    this.db.exec(`
      -- Games table (from Metadata.xml)
      CREATE TABLE IF NOT EXISTS games (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        platform_id TEXT,
        developer TEXT,
        publisher TEXT,
        release_date TEXT,
        overview TEXT,
        rating REAL,
        created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
      );

      -- Platforms table (from Platforms.xml)
      CREATE TABLE IF NOT EXISTS platforms (
        id TEXT PRIMARY KEY,
        name TEXT NOT NULL,
        category TEXT,
        developer TEXT,
        manufacturer TEXT,
        release_date TEXT
      );

      -- Genres table (many-to-many with games)
      CREATE TABLE IF NOT EXISTS genres (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT UNIQUE NOT NULL
      );

      CREATE TABLE IF NOT EXISTS game_genres (
        game_id TEXT,
        genre_id INTEGER,
        PRIMARY KEY (game_id, genre_id),
        FOREIGN KEY (game_id) REFERENCES games(id),
        FOREIGN KEY (genre_id) REFERENCES genres(id)
      );

      -- Files table (from Files.xml) - for ROM matching
      CREATE TABLE IF NOT EXISTS game_files (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        game_id TEXT,
        filename TEXT,
        crc TEXT,
        md5 TEXT,
        sha1 TEXT,
        FOREIGN KEY (game_id) REFERENCES games(id)
      );

      -- Metadata info (tracks last update)
      CREATE TABLE IF NOT EXISTS metadata_info (
        key TEXT PRIMARY KEY,
        value TEXT,
        updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
      );

      -- Indexes for fast searching
      CREATE INDEX IF NOT EXISTS idx_games_name ON games(name COLLATE NOCASE);
      CREATE INDEX IF NOT EXISTS idx_games_platform ON games(platform_id);
      CREATE INDEX IF NOT EXISTS idx_files_filename ON game_files(filename COLLATE NOCASE);
      CREATE INDEX IF NOT EXISTS idx_files_game ON game_files(game_id);

      -- Full-text search for game names
      CREATE VIRTUAL TABLE IF NOT EXISTS games_fts USING fts5(
        id,
        name,
        platform_id,
        content=games,
        content_rowid=rowid
      );

      -- Triggers to keep FTS in sync
      CREATE TRIGGER IF NOT EXISTS games_ai AFTER INSERT ON games BEGIN
        INSERT INTO games_fts(rowid, id, name, platform_id)
        VALUES (new.rowid, new.id, new.name, new.platform_id);
      END;

      CREATE TRIGGER IF NOT EXISTS games_ad AFTER DELETE ON games BEGIN
        DELETE FROM games_fts WHERE rowid = old.rowid;
      END;

      CREATE TRIGGER IF NOT EXISTS games_au AFTER UPDATE ON games BEGIN
        UPDATE games_fts SET id = new.id, name = new.name, platform_id = new.platform_id
        WHERE rowid = new.rowid;
      END;
    `);
  }

  /**
   * Get database path
   */
  getDbPath(): string {
    return this.dbPath;
  }

  /**
   * Check if database has been populated
   */
  isPopulated(): boolean {
    const result = this.db
      .prepare('SELECT COUNT(*) as count FROM games')
      .get() as { count: number };
    return result.count > 0;
  }

  /**
   * Get database statistics
   */
  getStats(): {
    games: number;
    platforms: number;
    genres: number;
    files: number;
  } {
    const games = this.db
      .prepare('SELECT COUNT(*) as count FROM games')
      .get() as { count: number };
    const platforms = this.db
      .prepare('SELECT COUNT(*) as count FROM platforms')
      .get() as { count: number };
    const genres = this.db
      .prepare('SELECT COUNT(*) as count FROM genres')
      .get() as { count: number };
    const files = this.db
      .prepare('SELECT COUNT(*) as count FROM game_files')
      .get() as { count: number };

    return {
      games: games.count,
      platforms: platforms.count,
      genres: genres.count,
      files: files.count,
    };
  }

  /**
   * Search games by name (full-text search)
   */
  searchByName(query: string, limit = 10): LaunchBoxMetadata[] {
    const stmt = this.db.prepare(`
      SELECT g.* FROM games_fts
      INNER JOIN games g ON games_fts.id = g.id
      WHERE games_fts MATCH ?
      ORDER BY rank
      LIMIT ?
    `);

    return stmt.all(query, limit) as LaunchBoxMetadata[];
  }

  /**
   * Search games by exact name
   */
  searchByExactName(name: string, platformId?: string): LaunchBoxMetadata[] {
    if (platformId) {
      const stmt = this.db.prepare(`
        SELECT * FROM games
        WHERE name = ? AND platform_id = ?
        LIMIT 1
      `);
      return stmt.all(name, platformId) as LaunchBoxMetadata[];
    } else {
      const stmt = this.db.prepare(`
        SELECT * FROM games
        WHERE name = ?
      `);
      return stmt.all(name) as LaunchBoxMetadata[];
    }
  }

  /**
   * Search games by fuzzy name match
   */
  searchByFuzzyName(name: string, platformId?: string, limit = 10): LaunchBoxMetadata[] {
    const searchPattern = `%${name}%`;

    if (platformId) {
      const stmt = this.db.prepare(`
        SELECT * FROM games
        WHERE name LIKE ? AND platform_id = ?
        ORDER BY LENGTH(name) ASC
        LIMIT ?
      `);
      return stmt.all(searchPattern, platformId, limit) as LaunchBoxMetadata[];
    } else {
      const stmt = this.db.prepare(`
        SELECT * FROM games
        WHERE name LIKE ?
        ORDER BY LENGTH(name) ASC
        LIMIT ?
      `);
      return stmt.all(searchPattern, limit) as LaunchBoxMetadata[];
    }
  }

  /**
   * Get game by ID
   */
  getGameById(id: string): LaunchBoxMetadata | null {
    const stmt = this.db.prepare('SELECT * FROM games WHERE id = ?');
    return (stmt.get(id) as LaunchBoxMetadata) || null;
  }

  /**
   * Get platform by ID
   */
  getPlatformById(id: string): any {
    const stmt = this.db.prepare('SELECT * FROM platforms WHERE id = ?');
    return stmt.get(id);
  }

  /**
   * Get all platforms
   */
  getAllPlatforms(): any[] {
    const stmt = this.db.prepare('SELECT * FROM platforms ORDER BY name');
    return stmt.all();
  }

  /**
   * Insert or update game
   */
  upsertGame(game: Partial<LaunchBoxMetadata>): void {
    const stmt = this.db.prepare(`
      INSERT INTO games (id, name, platform_id, developer, publisher, release_date, overview, rating)
      VALUES (?, ?, ?, ?, ?, ?, ?, ?)
      ON CONFLICT(id) DO UPDATE SET
        name = excluded.name,
        platform_id = excluded.platform_id,
        developer = excluded.developer,
        publisher = excluded.publisher,
        release_date = excluded.release_date,
        overview = excluded.overview,
        rating = excluded.rating,
        updated_at = CURRENT_TIMESTAMP
    `);

    stmt.run(
      game.id,
      game.name,
      game.platform || null,
      game.developer || null,
      game.publisher || null,
      game.releaseDate || null,
      game.overview || null,
      game.rating || null
    );
  }

  /**
   * Insert or update platform
   */
  upsertPlatform(platform: any): void {
    const stmt = this.db.prepare(`
      INSERT INTO platforms (id, name, category, developer, manufacturer, release_date)
      VALUES (?, ?, ?, ?, ?, ?)
      ON CONFLICT(id) DO UPDATE SET
        name = excluded.name,
        category = excluded.category,
        developer = excluded.developer,
        manufacturer = excluded.manufacturer,
        release_date = excluded.release_date
    `);

    stmt.run(
      platform.id,
      platform.name,
      platform.category || null,
      platform.developer || null,
      platform.manufacturer || null,
      platform.releaseDate || null
    );
  }

  /**
   * Insert genre
   */
  insertGenre(name: string): number {
    const stmt = this.db.prepare(`
      INSERT OR IGNORE INTO genres (name) VALUES (?)
    `);
    stmt.run(name);

    const result = this.db
      .prepare('SELECT id FROM genres WHERE name = ?')
      .get(name) as { id: number };
    return result.id;
  }

  /**
   * Link game to genre
   */
  linkGameGenre(gameId: string, genreId: number): void {
    const stmt = this.db.prepare(`
      INSERT OR IGNORE INTO game_genres (game_id, genre_id) VALUES (?, ?)
    `);
    stmt.run(gameId, genreId);
  }

  /**
   * Set metadata info
   */
  setMetadataInfo(key: string, value: string): void {
    const stmt = this.db.prepare(`
      INSERT INTO metadata_info (key, value, updated_at)
      VALUES (?, ?, CURRENT_TIMESTAMP)
      ON CONFLICT(key) DO UPDATE SET
        value = excluded.value,
        updated_at = CURRENT_TIMESTAMP
    `);
    stmt.run(key, value);
  }

  /**
   * Get metadata info
   */
  getMetadataInfo(key: string): string | null {
    const stmt = this.db.prepare('SELECT value FROM metadata_info WHERE key = ?');
    const result = stmt.get(key) as { value: string } | undefined;
    return result?.value || null;
  }

  /**
   * Begin transaction
   */
  beginTransaction(): void {
    this.db.exec('BEGIN TRANSACTION');
  }

  /**
   * Commit transaction
   */
  commitTransaction(): void {
    this.db.exec('COMMIT');
  }

  /**
   * Rollback transaction
   */
  rollbackTransaction(): void {
    this.db.exec('ROLLBACK');
  }

  /**
   * Close database
   */
  close(): void {
    this.db.close();
  }
}
