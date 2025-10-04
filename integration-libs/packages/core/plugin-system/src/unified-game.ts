/**
 * Unified Game Model
 * Represents a game that may exist across multiple sources
 */

import type { DetectedGame, GameMetadata, IdentifiedGame } from './types.js';

/**
 * Source-specific game instance
 * A game as detected by a specific source plugin
 */
export interface GameSource {
  /** Source plugin ID */
  sourceId: string;

  /** Source-specific game ID */
  gameId: string;

  /** Detected game information */
  detectedGame: DetectedGame;

  /** Identification results (if identified) */
  identification?: IdentifiedGame;

  /** Installation status in this source */
  installed: boolean;

  /** Last played timestamp in this source */
  lastPlayed?: Date;

  /** Playtime in minutes in this source */
  playtime?: number;

  /** Source-specific metadata */
  metadata?: Record<string, any>;
}

/**
 * Unified game across all sources
 * Represents the same game that may exist in Steam, Xbox, locally, etc.
 */
export interface UnifiedGame {
  /** Unique unified game ID */
  id: string;

  /** Primary game title (from best identification) */
  title: string;

  /** All sources where this game was detected */
  sources: GameSource[];

  /** Consolidated game metadata (from all identifiers) */
  identifications: IdentificationResult[];

  /** Primary platform */
  platform?: string;

  /** Cover image URL (from best source) */
  coverUrl?: string;

  /** Unified installation status (installed in ANY source) */
  isInstalled: boolean;

  /** Total playtime across all sources (in minutes) */
  totalPlaytime: number;

  /** Most recent play date across all sources */
  lastPlayed?: Date;

  /** User tags */
  tags?: string[];

  /** User notes */
  notes?: string;

  /** User rating (1-5) */
  userRating?: number;

  /** Favorite flag */
  isFavorite: boolean;

  /** Hidden flag */
  isHidden: boolean;

  /** Creation timestamp */
  createdAt: Date;

  /** Last updated timestamp */
  updatedAt: Date;
}

/**
 * Identification result from a specific identifier
 */
export interface IdentificationResult {
  /** Identifier plugin ID */
  identifierId: string;

  /** Identified metadata */
  metadata: GameMetadata;

  /** Match confidence (0-1) */
  confidence: number;

  /** Identification timestamp */
  identifiedAt: Date;
}

/**
 * Game matching strategy
 * How to determine if two detected games are the same game
 */
export enum MatchStrategy {
  /** Match by exact title */
  EXACT_TITLE = 'exact_title',

  /** Match by normalized title (lowercase, no special chars) */
  NORMALIZED_TITLE = 'normalized_title',

  /** Match by fuzzy title similarity */
  FUZZY_TITLE = 'fuzzy_title',

  /** Match by external ID (IGDB, Steam App ID, etc.) */
  EXTERNAL_ID = 'external_id',

  /** Match manually by user */
  MANUAL = 'manual',
}

/**
 * Game matcher
 * Determines if two detected games represent the same game
 */
export class GameMatcher {
  /**
   * Normalize title for comparison
   */
  static normalizeTitle(title: string): string {
    return title
      .toLowerCase()
      .replace(/[^a-z0-9]/g, '')
      .trim();
  }

  /**
   * Calculate title similarity (Levenshtein distance based)
   */
  static titleSimilarity(title1: string, title2: string): number {
    const norm1 = this.normalizeTitle(title1);
    const norm2 = this.normalizeTitle(title2);

    if (norm1 === norm2) return 1.0;

    const distance = this.levenshteinDistance(norm1, norm2);
    const maxLength = Math.max(norm1.length, norm2.length);

    return 1 - distance / maxLength;
  }

  /**
   * Levenshtein distance algorithm
   */
  private static levenshteinDistance(str1: string, str2: string): number {
    const matrix: number[][] = [];

    for (let i = 0; i <= str2.length; i++) {
      matrix[i] = [i];
    }

    for (let j = 0; j <= str1.length; j++) {
      matrix[0][j] = j;
    }

    for (let i = 1; i <= str2.length; i++) {
      for (let j = 1; j <= str1.length; j++) {
        if (str2.charAt(i - 1) === str1.charAt(j - 1)) {
          matrix[i][j] = matrix[i - 1][j - 1];
        } else {
          matrix[i][j] = Math.min(
            matrix[i - 1][j - 1] + 1,
            matrix[i][j - 1] + 1,
            matrix[i - 1][j] + 1
          );
        }
      }
    }

    return matrix[str2.length][str1.length];
  }

  /**
   * Check if two games match using a specific strategy
   */
  static matches(
    game1: DetectedGame,
    game2: DetectedGame,
    strategy: MatchStrategy = MatchStrategy.NORMALIZED_TITLE,
    threshold: number = 0.9
  ): boolean {
    switch (strategy) {
      case MatchStrategy.EXACT_TITLE:
        return game1.name === game2.name;

      case MatchStrategy.NORMALIZED_TITLE:
        return this.normalizeTitle(game1.name) === this.normalizeTitle(game2.name);

      case MatchStrategy.FUZZY_TITLE:
        return this.titleSimilarity(game1.name, game2.name) >= threshold;

      case MatchStrategy.EXTERNAL_ID:
        // Check for matching external IDs in metadata
        if (game1.metadata?.steamAppId && game2.metadata?.steamAppId) {
          return game1.metadata.steamAppId === game2.metadata.steamAppId;
        }
        if (game1.metadata?.igdbId && game2.metadata?.igdbId) {
          return game1.metadata.igdbId === game2.metadata.igdbId;
        }
        return false;

      case MatchStrategy.MANUAL:
        // Manual matching handled externally
        return false;

      default:
        return false;
    }
  }

  /**
   * Find best match for a game in a list of games
   */
  static findBestMatch(
    targetGame: DetectedGame,
    candidates: DetectedGame[],
    strategy: MatchStrategy = MatchStrategy.FUZZY_TITLE,
    threshold: number = 0.9
  ): DetectedGame | null {
    let bestMatch: DetectedGame | null = null;
    let bestScore = 0;

    for (const candidate of candidates) {
      if (strategy === MatchStrategy.FUZZY_TITLE) {
        const score = this.titleSimilarity(targetGame.name, candidate.name);
        if (score >= threshold && score > bestScore) {
          bestScore = score;
          bestMatch = candidate;
        }
      } else if (this.matches(targetGame, candidate, strategy, threshold)) {
        return candidate; // Exact match found
      }
    }

    return bestMatch;
  }
}

/**
 * Unified game manager
 * Manages merging of games from multiple sources
 */
export class UnifiedGameManager {
  private games: Map<string, UnifiedGame> = new Map();
  private matchStrategy: MatchStrategy = MatchStrategy.FUZZY_TITLE;
  private matchThreshold: number = 0.9;

  /**
   * Set matching strategy
   */
  setMatchStrategy(strategy: MatchStrategy, threshold?: number): void {
    this.matchStrategy = strategy;
    if (threshold !== undefined) {
      this.matchThreshold = threshold;
    }
  }

  /**
   * Add a detected game from a source
   */
  addDetectedGame(source: GameSource): UnifiedGame {
    // Try to find existing unified game that matches
    const existingGame = this.findMatchingUnifiedGame(source.detectedGame);

    if (existingGame) {
      // Add as new source to existing game
      existingGame.sources.push(source);
      existingGame.updatedAt = new Date();

      // Update consolidated data
      this.updateConsolidatedData(existingGame);

      return existingGame;
    } else {
      // Create new unified game
      const unifiedGame: UnifiedGame = {
        id: this.generateId(),
        title: source.detectedGame.name,
        sources: [source],
        identifications: [],
        platform: source.detectedGame.platform,
        isInstalled: source.installed,
        totalPlaytime: source.playtime || 0,
        lastPlayed: source.lastPlayed,
        isFavorite: false,
        isHidden: false,
        createdAt: new Date(),
        updatedAt: new Date(),
      };

      this.games.set(unifiedGame.id, unifiedGame);
      return unifiedGame;
    }
  }

  /**
   * Add identification result to a unified game
   */
  addIdentification(gameId: string, identifierId: string, result: IdentifiedGame): void {
    const game = this.games.get(gameId);
    if (!game || !result.metadata) return;

    const identification: IdentificationResult = {
      identifierId,
      metadata: result.metadata,
      confidence: result.matchConfidence,
      identifiedAt: new Date(),
    };

    game.identifications.push(identification);
    game.updatedAt = new Date();

    // Update title to best identification
    this.updateConsolidatedData(game);
  }

  /**
   * Find matching unified game
   */
  private findMatchingUnifiedGame(detectedGame: DetectedGame): UnifiedGame | null {
    for (const game of this.games.values()) {
      // Check if any source in this unified game matches
      for (const source of game.sources) {
        if (
          GameMatcher.matches(
            detectedGame,
            source.detectedGame,
            this.matchStrategy,
            this.matchThreshold
          )
        ) {
          return game;
        }
      }
    }
    return null;
  }

  /**
   * Update consolidated data for a unified game
   */
  private updateConsolidatedData(game: UnifiedGame): void {
    // Use best identification for title
    if (game.identifications.length > 0) {
      const bestId = game.identifications.reduce((best, current) =>
        current.confidence > best.confidence ? current : best
      );
      game.title = bestId.metadata.title;
      game.coverUrl = bestId.metadata.coverUrl;
      game.platform = bestId.metadata.platform || game.platform;
    }

    // Consolidate installation status
    game.isInstalled = game.sources.some((s) => s.installed);

    // Consolidate playtime
    game.totalPlaytime = game.sources.reduce((total, s) => total + (s.playtime || 0), 0);

    // Consolidate last played
    const lastPlayedDates = game.sources
      .map((s) => s.lastPlayed)
      .filter((d): d is Date => d !== undefined);
    if (lastPlayedDates.length > 0) {
      game.lastPlayed = new Date(Math.max(...lastPlayedDates.map((d) => d.getTime())));
    }
  }

  /**
   * Get all unified games
   */
  getAllGames(): UnifiedGame[] {
    return Array.from(this.games.values());
  }

  /**
   * Get game by ID
   */
  getGame(id: string): UnifiedGame | undefined {
    return this.games.get(id);
  }

  /**
   * Generate unique ID
   */
  private generateId(): string {
    return `unified-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
  }

  /**
   * Manually merge two unified games
   */
  mergeGames(gameId1: string, gameId2: string): UnifiedGame | null {
    const game1 = this.games.get(gameId1);
    const game2 = this.games.get(gameId2);

    if (!game1 || !game2) return null;

    // Merge sources
    game1.sources.push(...game2.sources);
    game1.identifications.push(...game2.identifications);

    // Update consolidated data
    this.updateConsolidatedData(game1);

    // Remove game2
    this.games.delete(gameId2);

    return game1;
  }

  /**
   * Manually split a source into a separate unified game
   */
  splitSource(gameId: string, sourceId: string): UnifiedGame | null {
    const game = this.games.get(gameId);
    if (!game) return null;

    const sourceIndex = game.sources.findIndex((s) => s.sourceId === sourceId);
    if (sourceIndex === -1) return null;

    const [source] = game.sources.splice(sourceIndex, 1);

    // Create new unified game for the split source
    return this.addDetectedGame(source);
  }
}
