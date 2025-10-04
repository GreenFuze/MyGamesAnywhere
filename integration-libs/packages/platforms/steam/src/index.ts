/**
 * @mygamesanywhere/steam-scanner
 *
 * Steam library scanner for MyGamesAnywhere
 *
 * Detects Steam installation, parses VDF files, and extracts installed game information.
 */

// Export main scanner
export { SteamScanner, scanSteamLibrary } from './scanner.js';

// Export Steam client
export { SteamClient } from './steam-client.js';

// Export VDF parser
export { VDFParser, parseVDF } from './vdf-parser.js';

// Export types
export type {
  SteamGame,
  SteamLibraryFolder,
  SteamPaths,
  ScanConfig,
  ScanResult,
  VDFObject,
  VDFValue,
} from './types.js';

export type { SteamWebAPIConfig, SteamWebGame } from './steam-client.js';

// Export errors
export {
  SteamScannerError,
  SteamNotFoundError,
  VDFParseError,
  FileAccessError,
  InvalidConfigError,
} from './errors.js';
