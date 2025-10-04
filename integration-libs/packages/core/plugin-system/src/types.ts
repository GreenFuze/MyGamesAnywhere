/**
 * Plugin System Types
 * Core interfaces for the MyGamesAnywhere plugin architecture
 */

/**
 * Plugin metadata
 */
export interface PluginMetadata {
  /** Unique plugin identifier */
  id: string;
  /** Human-readable name */
  name: string;
  /** Plugin version */
  version: string;
  /** Plugin description */
  description: string;
  /** Plugin author */
  author?: string;
}

/**
 * Plugin configuration schema
 */
export interface PluginConfigSchema {
  /** Configuration properties */
  properties: Record<string, any>;
  /** Required properties */
  required?: string[];
}

/**
 * Plugin type enumeration
 */
export enum PluginType {
  SOURCE = 'source',
  IDENTIFIER = 'identifier',
  STORAGE = 'storage',
}

/**
 * Base plugin interface
 */
export interface Plugin {
  /** Plugin metadata */
  readonly metadata: PluginMetadata;

  /** Plugin type */
  readonly type: PluginType;

  /** Configuration schema */
  readonly configSchema?: PluginConfigSchema;

  /**
   * Initialize the plugin
   * @param config Plugin configuration
   */
  initialize(config?: Record<string, any>): Promise<void>;

  /**
   * Validate plugin configuration
   * @param config Configuration to validate
   */
  validateConfig?(config: Record<string, any>): boolean;

  /**
   * Check if plugin is ready to use
   */
  isReady(): Promise<boolean>;

  /**
   * Cleanup plugin resources
   */
  cleanup?(): Promise<void>;
}

/**
 * Detected game from a source
 */
export interface DetectedGame {
  /** Source plugin that detected this game */
  sourceId: string;

  /** Unique identifier within the source */
  id: string;

  /** Detected game name (filename, title, etc.) */
  name: string;

  /** File path or location */
  path?: string;

  /** Directory name (for context) */
  directoryName?: string;

  /** File size in bytes */
  size?: number;

  /** Installation status */
  installed?: boolean;

  /** Last played timestamp */
  lastPlayed?: Date;

  /** Platform hint (if known) */
  platform?: string;

  /** Additional metadata */
  metadata?: Record<string, any>;
}

/**
 * Identified game metadata
 */
export interface GameMetadata {
  /** Game ID from metadata source */
  id: string;

  /** Official game title */
  title: string;

  /** Platform */
  platform?: string;

  /** Developer */
  developer?: string;

  /** Publisher */
  publisher?: string;

  /** Release date */
  releaseDate?: string;

  /** Description */
  description?: string;

  /** Genres */
  genres?: string[];

  /** Rating (0-100) */
  rating?: number;

  /** Cover image URL */
  coverUrl?: string;

  /** Metadata source */
  source: string;
}

/**
 * Identified game result
 */
export interface IdentifiedGame {
  /** Original detected game */
  detectedGame: DetectedGame;

  /** Identified metadata (if found) */
  metadata?: GameMetadata;

  /** Match confidence (0-1) */
  matchConfidence: number;

  /** Metadata source */
  source?: string;
}

/**
 * Source plugin interface
 * Scans and detects games from various sources
 */
export interface SourcePlugin extends Plugin {
  type: PluginType.SOURCE;

  /**
   * Scan for games
   * @returns Array of detected games
   */
  scan(): Promise<DetectedGame[]>;

  /**
   * Get a specific game by ID
   * @param gameId Game identifier
   */
  getGame?(gameId: string): Promise<DetectedGame | null>;

  /**
   * Launch a game
   * @param gameId Game identifier
   */
  launch?(gameId: string): Promise<void>;

  /**
   * Install a game
   * @param gameId Game identifier
   */
  install?(gameId: string): Promise<void>;

  /**
   * Uninstall a game
   * @param gameId Game identifier
   */
  uninstall?(gameId: string): Promise<void>;
}

/**
 * Identifier plugin interface
 * Identifies games and enriches with metadata
 */
export interface IdentifierPlugin extends Plugin {
  type: PluginType.IDENTIFIER;

  /**
   * Identify a detected game
   * @param game Detected game to identify
   */
  identify(game: DetectedGame): Promise<IdentifiedGame>;

  /**
   * Search for games by name
   * @param query Search query
   * @param platform Optional platform filter
   */
  search(query: string, platform?: string): Promise<GameMetadata[]>;

  /**
   * Update metadata database
   */
  update?(): Promise<void>;
}

/**
 * Storage plugin interface
 * Stores and synchronizes game data
 */
export interface StoragePlugin extends Plugin {
  type: PluginType.STORAGE;

  /**
   * Save game data
   * @param gameId Game identifier
   * @param data Game data to save
   */
  save(gameId: string, data: any): Promise<void>;

  /**
   * Load game data
   * @param gameId Game identifier
   */
  load(gameId: string): Promise<any>;

  /**
   * Delete game data
   * @param gameId Game identifier
   */
  delete(gameId: string): Promise<void>;

  /**
   * List all stored games
   */
  list(): Promise<string[]>;

  /**
   * Synchronize with remote storage
   */
  sync?(): Promise<void>;
}
