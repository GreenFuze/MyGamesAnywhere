/**
 * Steam Library Scanner
 *
 * Scans Steam installation for installed games by:
 * 1. Finding Steam installation directory
 * 2. Reading libraryfolders.vdf to find all library folders
 * 3. Scanning each library folder for appmanifest_*.acf files
 * 4. Parsing each manifest to extract game information
 */

import * as fs from 'fs/promises';
import * as path from 'path';
import { platform } from 'os';
import {
  SteamGame,
  SteamLibraryFolder,
  SteamPaths,
  ScanConfig,
  ScanResult,
} from './types.js';
import {
  SteamNotFoundError,
  FileAccessError,
  VDFParseError,
} from './errors.js';
import { parseVDF } from './vdf-parser.js';
import { VDFObject } from './types.js';

/**
 * Main Steam Scanner class
 */
export class SteamScanner {
  private steamPaths: SteamPaths | null = null;

  /**
   * Detect Steam installation path for current platform
   */
  public async detectSteamPath(): Promise<string> {
    const platformName = platform();

    let possiblePaths: string[] = [];

    switch (platformName) {
      case 'win32':
        possiblePaths = [
          'C:\\Program Files (x86)\\Steam',
          'C:\\Program Files\\Steam',
          path.join(process.env.ProgramFiles || 'C:\\Program Files', 'Steam'),
          path.join(
            process.env['ProgramFiles(x86)'] || 'C:\\Program Files (x86)',
            'Steam'
          ),
        ];
        break;

      case 'darwin':
        possiblePaths = [
          path.join(
            process.env.HOME || '~',
            'Library/Application Support/Steam'
          ),
        ];
        break;

      case 'linux':
        possiblePaths = [
          path.join(process.env.HOME || '~', '.steam/steam'),
          path.join(process.env.HOME || '~', '.local/share/Steam'),
        ];
        break;

      default:
        throw new SteamNotFoundError(
          `Unsupported platform: ${platformName}`
        );
    }

    // Try each path
    for (const steamPath of possiblePaths) {
      try {
        const stats = await fs.stat(steamPath);
        if (stats.isDirectory()) {
          return steamPath;
        }
      } catch {
        // Path doesn't exist, try next
        continue;
      }
    }

    throw new SteamNotFoundError(
      `Steam installation not found. Tried paths: ${possiblePaths.join(', ')}`
    );
  }

  /**
   * Initialize scanner with Steam paths
   */
  public async initialize(config?: ScanConfig): Promise<void> {
    const steamPath =
      config?.steamPath || (await this.detectSteamPath());

    const platformName = platform();
    let libraryFoldersVdf: string;
    let steamAppsPath: string;

    if (platformName === 'win32') {
      libraryFoldersVdf = path.join(
        steamPath,
        'steamapps',
        'libraryfolders.vdf'
      );
      steamAppsPath = path.join(steamPath, 'steamapps');
    } else {
      libraryFoldersVdf = path.join(
        steamPath,
        'steamapps',
        'libraryfolders.vdf'
      );
      steamAppsPath = path.join(steamPath, 'steamapps');
    }

    // Verify paths exist
    try {
      await fs.access(steamPath);
      await fs.access(libraryFoldersVdf);
    } catch (error) {
      throw new FileAccessError(
        `Cannot access Steam installation at ${steamPath}`,
        steamPath,
        error instanceof Error ? error : undefined
      );
    }

    this.steamPaths = {
      steamPath,
      libraryFoldersVdf,
      steamAppsPath,
    };
  }

  /**
   * Scan Steam library for installed games
   */
  public async scan(config?: ScanConfig): Promise<ScanResult> {
    const startTime = Date.now();

    // Initialize if not already done
    if (!this.steamPaths) {
      await this.initialize(config);
    }

    if (!this.steamPaths) {
      throw new SteamNotFoundError('Steam paths not initialized');
    }

    // Read and parse libraryfolders.vdf
    const libraryFolders = await this.readLibraryFolders(
      this.steamPaths.libraryFoldersVdf
    );

    // Scan each library folder for games
    const games: SteamGame[] = [];

    for (const folder of libraryFolders) {
      const folderGames = await this.scanLibraryFolder(folder, config);
      games.push(...folderGames);
    }

    const scanDuration = Date.now() - startTime;

    return {
      games,
      libraryFolders,
      steamPath: this.steamPaths.steamPath,
      scanDuration,
    };
  }

  /**
   * Read and parse libraryfolders.vdf
   */
  private async readLibraryFolders(
    vdfPath: string
  ): Promise<SteamLibraryFolder[]> {
    try {
      const content = await fs.readFile(vdfPath, 'utf-8');
      const parsed = parseVDF(content);

      // libraryfolders.vdf structure:
      // "libraryfolders"
      // {
      //     "0"
      //     {
      //         "path"  "C:\\Program Files (x86)\\Steam"
      //         "label"  ""
      //         "contentid"  "..."
      //         "totalsize"  "..."
      //     }
      //     "1" { ... }
      // }

      const folders: SteamLibraryFolder[] = [];

      // Get the root object (usually "libraryfolders")
      const rootKey = Object.keys(parsed)[0];
      const libraryFoldersObj = parsed[rootKey] as VDFObject;

      if (!libraryFoldersObj || typeof libraryFoldersObj !== 'object') {
        throw new VDFParseError(
          'Invalid libraryfolders.vdf format',
          vdfPath
        );
      }

      // Each key is a folder index ("0", "1", etc.)
      for (const key of Object.keys(libraryFoldersObj)) {
        const folderObj = libraryFoldersObj[key];

        if (typeof folderObj === 'object' && folderObj !== null) {
          const folder = folderObj as VDFObject;

          folders.push({
            path: String(folder.path || ''),
            label: String(folder.label || ''),
            contentid: String(folder.contentid || ''),
            totalsize: String(folder.totalsize || '0'),
          });
        }
      }

      return folders;
    } catch (error) {
      if (
        error instanceof VDFParseError ||
        error instanceof FileAccessError
      ) {
        throw error;
      }
      throw new FileAccessError(
        `Failed to read library folders: ${error instanceof Error ? error.message : String(error)}`,
        vdfPath,
        error instanceof Error ? error : undefined
      );
    }
  }

  /**
   * Scan a library folder for installed games
   */
  private async scanLibraryFolder(
    folder: SteamLibraryFolder,
    config?: ScanConfig
  ): Promise<SteamGame[]> {
    const games: SteamGame[] = [];
    const steamappsPath = path.join(folder.path, 'steamapps');

    try {
      // Read all files in steamapps folder
      const files = await fs.readdir(steamappsPath);

      // Filter for appmanifest_*.acf files
      const manifestFiles = files.filter((file) =>
        file.match(/^appmanifest_\d+\.acf$/)
      );

      // Parse each manifest
      for (const manifestFile of manifestFiles) {
        const manifestPath = path.join(steamappsPath, manifestFile);

        try {
          const game = await this.parseGameManifest(
            manifestPath,
            folder.path,
            config
          );
          if (game) {
            games.push(game);
          }
        } catch (error) {
          // Log error but continue scanning other manifests
          console.warn(
            `Failed to parse ${manifestFile}: ${error instanceof Error ? error.message : String(error)}`
          );
        }
      }
    } catch (error) {
      // Library folder might not exist or be accessible
      console.warn(
        `Failed to scan library folder ${folder.path}: ${error instanceof Error ? error.message : String(error)}`
      );
    }

    return games;
  }

  /**
   * Parse a game manifest file (appmanifest_*.acf)
   */
  private async parseGameManifest(
    manifestPath: string,
    libraryPath: string,
    _config?: ScanConfig
  ): Promise<SteamGame | null> {
    try {
      const content = await fs.readFile(manifestPath, 'utf-8');
      const parsed = parseVDF(content);

      // appmanifest structure:
      // "AppState"
      // {
      //     "appid"  "440"
      //     "name"  "Team Fortress 2"
      //     "installdir"  "Team Fortress 2"
      //     "LastUpdated"  "1234567890"
      //     "SizeOnDisk"  "123456789"
      //     "buildid"  "..."
      //     "LastOwner"  "..."
      // }

      const appState = parsed.AppState as VDFObject;

      if (!appState || typeof appState !== 'object') {
        throw new VDFParseError(
          'Invalid appmanifest format',
          manifestPath
        );
      }

      const game: SteamGame = {
        appid: String(appState.appid || ''),
        name: String(appState.name || ''),
        installdir: String(appState.installdir || ''),
        libraryPath,
        lastUpdated: Number(appState.LastUpdated || 0),
        sizeOnDisk: String(appState.SizeOnDisk || '0'),
        buildId: appState.buildid
          ? String(appState.buildid)
          : undefined,
        lastOwner: appState.LastOwner
          ? String(appState.LastOwner)
          : undefined,
      };

      return game;
    } catch (error) {
      if (error instanceof VDFParseError) {
        throw error;
      }
      throw new FileAccessError(
        `Failed to parse game manifest: ${error instanceof Error ? error.message : String(error)}`,
        manifestPath,
        error instanceof Error ? error : undefined
      );
    }
  }
}

/**
 * Convenience function to scan Steam library
 */
export async function scanSteamLibrary(
  config?: ScanConfig
): Promise<ScanResult> {
  const scanner = new SteamScanner();
  return scanner.scan(config);
}
