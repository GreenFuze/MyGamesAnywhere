/**
 * Game Identifier
 * Identifies games and matches them to metadata from various sources
 */

// Types
export * from './types/index.js';

// Parsers
export { NameExtractor } from './parsers/name-extractor.js';

// Identifiers
export { LaunchBoxIdentifier, type LaunchBoxIdentifierConfig } from './identifiers/launchbox-identifier.js';

// Re-export platform types
export type { LaunchBoxDB, LaunchBoxDownloader, LaunchBoxStreamingParser } from '@mygamesanywhere/platform-launchbox';
