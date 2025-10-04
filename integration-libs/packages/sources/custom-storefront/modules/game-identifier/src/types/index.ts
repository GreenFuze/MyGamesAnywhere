/**
 * Game Identifier Types
 * Types for game identification and metadata matching
 */

/**
 * Detected game from repository scanner
 */
export interface DetectedGame {
  id: string;
  name: string;
  type: string;
  path: string;
  size: number;
  confidence: number;
  files?: string[];
  metadata?: Record<string, unknown>;
}

/**
 * Identified game with metadata
 */
export interface IdentifiedGame {
  detectedGame: DetectedGame;
  metadata?: GameMetadata;
  matchConfidence: number;
  source?: 'launchbox' | 'igdb' | 'steam' | 'manual';
}

/**
 * Game metadata from various sources
 */
export interface GameMetadata {
  id: string;
  title: string;
  platform?: string;
  developer?: string;
  publisher?: string;
  releaseDate?: string;
  description?: string;
  genres?: string[];
  rating?: number;
  coverUrl?: string;
  source: 'launchbox' | 'igdb' | 'steam';
}

/**
 * Game identifier interface
 */
export interface GameIdentifier {
  /**
   * Identify a detected game and return metadata
   */
  identify(detectedGame: DetectedGame): Promise<IdentifiedGame>;

  /**
   * Search for games by name
   */
  search(query: string, platform?: string): Promise<GameMetadata[]>;

  /**
   * Check if metadata source is ready
   */
  isReady(): Promise<boolean>;

  /**
   * Update metadata database
   */
  update(): Promise<void>;
}

/**
 * LaunchBox metadata record
 */
export interface LaunchBoxMetadata {
  id: string;
  name: string;
  platform: string;
  developer?: string;
  publisher?: string;
  releaseDate?: string;
  overview?: string;
  genres?: string[];
  rating?: number;
  // Add more fields as needed from XML
}

/**
 * Name extraction result
 */
export interface ExtractedName {
  cleanName: string;
  platform?: string;
  region?: string;
  version?: string;
  languages?: string[];
  isPart?: boolean;
  partNumber?: number;
  confidence: number;
}
