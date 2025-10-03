# Plugin System

MyGamesAnywhere uses a pluggable architecture where plugins run **client-side** as TypeScript modules. This document describes the plugin system design, interfaces, and how to create plugins.

## Overview

### Design Principles

1. **Client-Side Execution** - Plugins run on the CLIENT, not the server
2. **Privacy First** - User data (API keys, OAuth tokens, local paths) stays on client
3. **TypeScript Modules** - Plugins are TypeScript/JavaScript modules
4. **Capability-Based** - Plugins declare capabilities, not strict types
5. **Cross-Platform** - Works on desktop (Electron) and mobile (Capacitor)

### Why Client-Side?

**Privacy & Security:**
- User's API keys stored locally encrypted (never sent to server)
- OAuth tokens (Google Drive, etc.) stored on user's device only
- Local file paths (Steam library) never sent to server
- Server never sees user's credentials or local data

**Functionality:**
- Plugins can access user's local filesystem (scan Steam folder)
- Plugins can launch games on user's machine
- Plugins can use user's personal OAuth tokens
- Each user has independent API rate limits

**Performance:**
- Distributed compute (heavy work on user's device)
- Server stays ultra-lightweight (free tier viable)
- Direct downloads from source (no server proxy)

### Architecture

```
┌─────────────────────────────────────────────────────────┐
│  Client App (Desktop or Mobile)                         │
│                                                          │
│  ┌───────────────────────────────────────────────────┐  │
│  │  Plugin Manager                                   │  │
│  │  - Load plugins as TypeScript modules            │  │
│  │  - Initialize with encrypted config              │  │
│  │  - Execute plugin methods                        │  │
│  │  - Handle errors and failures                    │  │
│  └───────────────────────────────────────────────────┘  │
│         │                                                │
│         ├──> Steam Plugin (canScanInstalled, canLaunch) │
│         │    - Scans C:\Program Files\Steam\           │
│         │    - Uses user's Steam Web API key           │
│         │                                               │
│         ├──> Google Drive Plugin (canScanAvailable)     │
│         │    - Uses user's OAuth token                 │
│         │    - Downloads from user's GDrive            │
│         │                                               │
│         ├──> IGDB Plugin (canSearchMetadata)            │
│         │    - Uses user's IGDB API key                │
│         │    - Fetches metadata and media URLs         │
│         │                                               │
│         └──> Native Launcher (canLaunch)                │
│              - Executes .exe, .app, etc.               │
│              - Monitors game processes                  │
│                                                          │
│  Local Storage (Encrypted):                             │
│  ~/.mygamesanywhere/                                    │
│  ├── config.json              # User preferences        │
│  ├── secrets.encrypted        # API keys, OAuth tokens  │
│  ├── cache.db                 # SQLite cache            │
│  └── plugins/                 # Installed plugins       │
│      ├── steam/                                         │
│      ├── gdrive/                                        │
│      └── igdb/                                          │
└──────────────────────────────────────────────────────────┘
         │
         │ REST API (metadata sync only)
         │
┌─────────────────────────────────────────────────────────┐
│  Server (Ultra-Lightweight)                             │
│  - Stores game metadata (titles, IDs, platforms)        │
│  - Stores metadata URLs (cover URLs, screenshot URLs)   │
│  - Syncs library state across devices                   │
│  - NEVER sees user's API keys, OAuth tokens, or paths   │
└─────────────────────────────────────────────────────────┘
```

---

## Plugin Interface

### Base Plugin Interface

All plugins implement this base interface:

```typescript
interface Plugin {
  // Metadata
  id: string;
  name: string;
  version: string;
  description: string;
  author: string;
  homepage?: string;

  // Capabilities (what this plugin can do)
  capabilities: PluginCapabilities;

  // Configuration schema
  configSchema: ConfigSchema;

  // Lifecycle methods
  initialize(config: PluginConfig): Promise<void>;
  destroy(): Promise<void>;

  // Capability implementations (optional, based on capabilities)

  // Source capabilities
  scanInstalled?(config: ScanConfig): Promise<Game[]>;
  scanAvailable?(config: ScanConfig): Promise<Game[]>;
  download?(game: Game, destination: string): Promise<void>;

  // Metadata capabilities
  searchMetadata?(query: string): Promise<Metadata[]>;
  fetchMetadata?(gameId: string): Promise<Metadata>;

  // Media capabilities
  fetchMedia?(gameId: string, type: MediaType): Promise<string>;

  // Launcher capabilities
  launch?(game: Game, options: LaunchOptions): Promise<Process>;
  monitor?(processId: string): Promise<ProcessStatus>;
}
```

### Capability-Based Design

Plugins declare capabilities instead of being categorized into strict types:

```typescript
interface PluginCapabilities {
  // Source capabilities
  canScanInstalled?: boolean;    // Can scan for installed games
  canScanAvailable?: boolean;    // Can scan for available games
  canDownload?: boolean;         // Can download games

  // Metadata capabilities
  canSearchMetadata?: boolean;   // Can search for games by title
  canFetchMetadata?: boolean;    // Can fetch detailed metadata

  // Media capabilities
  canFetchMedia?: boolean;       // Can fetch covers, screenshots, videos

  // Launcher capabilities
  canLaunch?: boolean;           // Can launch games
  canMonitor?: boolean;          // Can monitor running games
}
```

**Why capability-based?**

Some plugins serve multiple purposes:
- **Steam Plugin:** `canScanInstalled` + `canSearchMetadata` + `canFetchMedia` + `canLaunch`
- **IGDB Plugin:** `canSearchMetadata` + `canFetchMetadata` + `canFetchMedia`
- **Native Launcher:** `canLaunch` + `canMonitor`
- **Google Drive Plugin:** `canScanAvailable` + `canDownload`

---

## Type Definitions

```typescript
// Game information
interface Game {
  id: string;                    // Unique ID
  title: string;
  platform: string;              // windows, linux, mac, ps2, xbox, etc.
  executionType: ExecutionType;  // native, emulated, streaming, cloud
  version?: string;
  installPath?: string;          // Local path if installed
  installSize?: number;          // Bytes
  source: string;                // Plugin ID that found this game
  metadata?: Metadata;
  lastPlayed?: Date;
}

enum ExecutionType {
  Native = 'native',             // .exe, .app, native binaries
  Emulated = 'emulated',         // ROMs via emulators
  Streaming = 'streaming',       // Cloud gaming (xCloud, GeForce Now)
  Cloud = 'cloud'                // Web-based games
}

// Metadata
interface Metadata {
  title: string;
  alternateTitles?: string[];
  platform: string;
  releaseDate?: Date;
  developer?: string;
  publisher?: string;
  genres?: string[];
  description?: string;
  rating?: number;               // 0-100
  playerCount?: string;          // "Single-player", "Multiplayer", etc.
  externalIds?: Record<string, string>; // { steam: "12345", igdb: "67890" }
  mediaUrls?: MediaUrls;
}

interface MediaUrls {
  coverArt?: string;
  screenshots?: string[];
  videos?: string[];
  background?: string;
  logo?: string;
  banner?: string;
}

// Plugin configuration
interface PluginConfig {
  enabled: boolean;
  settings: Record<string, any>;
}

interface ConfigSchema {
  [key: string]: ConfigField;
}

interface ConfigField {
  type: 'string' | 'number' | 'boolean' | 'password' | 'path' | 'oauth';
  label: string;
  description?: string;
  default?: any;
  required?: boolean;
  validation?: (value: any) => boolean | string;
}

// Launch options
interface LaunchOptions {
  fullscreen?: boolean;
  resolution?: string;           // "1920x1080" or empty for default
  arguments?: string[];
  environment?: Record<string, string>;
  workingDir?: string;
}

interface Process {
  processId: string;
  startTime: Date;
  game: Game;
}

interface ProcessStatus {
  isRunning: boolean;
  exitCode?: number;
  exitTime?: Date;
  playtime?: number;             // Seconds
}

// Scan configuration
interface ScanConfig {
  path?: string;                 // For local folder scans
  recursive?: boolean;
  filters?: string[];            // File extensions to include
}
```

---

## Example Plugins

### 1. Steam Plugin

Scans Steam library, fetches metadata from Steam API, and launches games.

```typescript
import { Plugin, PluginCapabilities, Game, Metadata, LaunchOptions, Process } from '@mygamesanywhere/plugin-sdk';
import { exec } from 'child_process';
import { readFile } from 'fs/promises';
import path from 'path';

export class SteamPlugin implements Plugin {
  id = 'steam';
  name = 'Steam';
  version = '1.0.0';
  description = 'Steam library integration';
  author = 'MyGamesAnywhere Team';

  capabilities: PluginCapabilities = {
    canScanInstalled: true,
    canSearchMetadata: true,
    canFetchMedia: true,
    canLaunch: true,
  };

  configSchema = {
    steamPath: {
      type: 'path' as const,
      label: 'Steam Installation Path',
      description: 'Path to Steam installation',
      default: 'C:\\Program Files (x86)\\Steam',
      required: true,
    },
    apiKey: {
      type: 'password' as const,
      label: 'Steam Web API Key',
      description: 'Get from https://steamcommunity.com/dev/apikey',
      required: true,
    },
  };

  private config: PluginConfig | null = null;

  async initialize(config: PluginConfig): Promise<void> {
    this.config = config;

    // Validate Steam path exists
    const steamPath = config.settings.steamPath;
    if (!await this.pathExists(steamPath)) {
      throw new Error(`Steam not found at ${steamPath}`);
    }

    // Validate API key format
    const apiKey = config.settings.apiKey;
    if (!apiKey || apiKey.length !== 32) {
      throw new Error('Invalid Steam Web API key');
    }
  }

  async destroy(): Promise<void> {
    this.config = null;
  }

  // Scan for installed Steam games
  async scanInstalled(scanConfig: ScanConfig): Promise<Game[]> {
    if (!this.config) throw new Error('Plugin not initialized');

    const steamPath = this.config.settings.steamPath;
    const steamappsPath = path.join(steamPath, 'steamapps');

    // Read libraryfolders.vdf
    const libraryFoldersPath = path.join(steamappsPath, 'libraryfolders.vdf');
    const libraryFolders = await this.parseLibraryFolders(libraryFoldersPath);

    // Scan all library folders for .acf files
    const games: Game[] = [];
    for (const folder of libraryFolders) {
      const folderGames = await this.scanLibraryFolder(folder);
      games.push(...folderGames);
    }

    return games;
  }

  private async scanLibraryFolder(libraryPath: string): Promise<Game[]> {
    const steamappsPath = path.join(libraryPath, 'steamapps');
    const files = await this.readDir(steamappsPath);

    const games: Game[] = [];
    for (const file of files) {
      if (file.endsWith('.acf')) {
        const acfPath = path.join(steamappsPath, file);
        const game = await this.parseAcfFile(acfPath);
        if (game) games.push(game);
      }
    }

    return games;
  }

  private async parseAcfFile(acfPath: string): Promise<Game | null> {
    const content = await readFile(acfPath, 'utf-8');

    // Parse VDF format (simplified)
    const appId = this.extractVdfValue(content, 'appid');
    const name = this.extractVdfValue(content, 'name');
    const installDir = this.extractVdfValue(content, 'installdir');

    if (!appId || !name) return null;

    return {
      id: `steam-${appId}`,
      title: name,
      platform: 'windows',
      executionType: 'native',
      installPath: path.join(path.dirname(acfPath), 'common', installDir),
      source: this.id,
    };
  }

  // Search Steam Store for metadata
  async searchMetadata(query: string): Promise<Metadata[]> {
    if (!this.config) throw new Error('Plugin not initialized');

    const apiKey = this.config.settings.apiKey;
    const url = `https://api.steampowered.com/ISteamApps/GetAppList/v2/`;

    const response = await fetch(url);
    const data = await response.json();

    // Filter apps matching query
    const matches = data.applist.apps
      .filter((app: any) => app.name.toLowerCase().includes(query.toLowerCase()))
      .slice(0, 10);

    // Fetch detailed info for each match
    const metadataPromises = matches.map((app: any) =>
      this.fetchMetadata(app.appid.toString())
    );

    return Promise.all(metadataPromises);
  }

  // Fetch detailed metadata for a game
  async fetchMetadata(gameId: string): Promise<Metadata> {
    if (!this.config) throw new Error('Plugin not initialized');

    const appId = gameId.replace('steam-', '');
    const url = `https://store.steampowered.com/api/appdetails?appids=${appId}`;

    const response = await fetch(url);
    const data = await response.json();
    const details = data[appId]?.data;

    if (!details) {
      throw new Error(`Game ${gameId} not found on Steam`);
    }

    return {
      title: details.name,
      platform: 'windows',
      releaseDate: new Date(details.release_date?.date),
      developer: details.developers?.[0],
      publisher: details.publishers?.[0],
      genres: details.genres?.map((g: any) => g.description),
      description: details.short_description,
      externalIds: { steam: appId },
      mediaUrls: {
        coverArt: details.header_image,
        screenshots: details.screenshots?.map((s: any) => s.path_full),
        background: details.background,
      },
    };
  }

  // Launch game via Steam
  async launch(game: Game, options: LaunchOptions): Promise<Process> {
    if (!this.config) throw new Error('Plugin not initialized');

    const appId = game.id.replace('steam-', '');
    const steamUrl = `steam://rungameid/${appId}`;

    // Launch via Steam protocol
    return new Promise((resolve, reject) => {
      exec(steamUrl, (error) => {
        if (error) {
          reject(new Error(`Failed to launch ${game.title}: ${error.message}`));
        } else {
          resolve({
            processId: `steam-${appId}-${Date.now()}`,
            startTime: new Date(),
            game,
          });
        }
      });
    });
  }

  // Helper methods
  private extractVdfValue(content: string, key: string): string {
    const regex = new RegExp(`"${key}"\\s+"([^"]+)"`);
    const match = content.match(regex);
    return match ? match[1] : '';
  }

  private async pathExists(path: string): Promise<boolean> {
    // Implementation depends on platform (Electron vs Capacitor)
    return true; // Placeholder
  }

  private async readDir(path: string): Promise<string[]> {
    // Implementation depends on platform
    return []; // Placeholder
  }

  private async parseLibraryFolders(path: string): Promise<string[]> {
    // Parse libraryfolders.vdf
    return []; // Placeholder
  }
}
```

---

### 2. IGDB Metadata Plugin

Fetches metadata from IGDB API.

```typescript
import { Plugin, PluginCapabilities, Metadata } from '@mygamesanywhere/plugin-sdk';

export class IGDBPlugin implements Plugin {
  id = 'igdb';
  name = 'IGDB';
  version = '1.0.0';
  description = 'IGDB metadata provider';
  author = 'MyGamesAnywhere Team';

  capabilities: PluginCapabilities = {
    canSearchMetadata: true,
    canFetchMetadata: true,
    canFetchMedia: true,
  };

  configSchema = {
    clientId: {
      type: 'password' as const,
      label: 'IGDB Client ID',
      description: 'Get from https://api.igdb.com',
      required: true,
    },
    clientSecret: {
      type: 'password' as const,
      label: 'IGDB Client Secret',
      required: true,
    },
  };

  private config: PluginConfig | null = null;
  private accessToken: string | null = null;
  private tokenExpiry: Date | null = null;

  async initialize(config: PluginConfig): Promise<void> {
    this.config = config;
    await this.refreshAccessToken();
  }

  async destroy(): Promise<void> {
    this.config = null;
    this.accessToken = null;
  }

  private async refreshAccessToken(): Promise<void> {
    if (!this.config) throw new Error('Plugin not initialized');

    const { clientId, clientSecret } = this.config.settings;

    const response = await fetch('https://id.twitch.tv/oauth2/token', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: `client_id=${clientId}&client_secret=${clientSecret}&grant_type=client_credentials`,
    });

    const data = await response.json();
    this.accessToken = data.access_token;
    this.tokenExpiry = new Date(Date.now() + data.expires_in * 1000);
  }

  private async ensureValidToken(): Promise<void> {
    if (!this.accessToken || !this.tokenExpiry || this.tokenExpiry < new Date()) {
      await this.refreshAccessToken();
    }
  }

  async searchMetadata(query: string): Promise<Metadata[]> {
    await this.ensureValidToken();

    const response = await fetch('https://api.igdb.com/v4/games', {
      method: 'POST',
      headers: {
        'Client-ID': this.config!.settings.clientId,
        'Authorization': `Bearer ${this.accessToken}`,
      },
      body: `search "${query}"; fields name,summary,cover.url,screenshots.url,release_dates.date,genres.name,involved_companies.company.name; limit 10;`,
    });

    const games = await response.json();
    return games.map((game: any) => this.mapToMetadata(game));
  }

  async fetchMetadata(gameId: string): Promise<Metadata> {
    await this.ensureValidToken();

    const igdbId = gameId.replace('igdb-', '');

    const response = await fetch('https://api.igdb.com/v4/games', {
      method: 'POST',
      headers: {
        'Client-ID': this.config!.settings.clientId,
        'Authorization': `Bearer ${this.accessToken}`,
      },
      body: `where id = ${igdbId}; fields name,summary,cover.url,screenshots.url,release_dates.date,genres.name,involved_companies.company.name,rating;`,
    });

    const games = await response.json();
    if (games.length === 0) {
      throw new Error(`Game ${gameId} not found in IGDB`);
    }

    return this.mapToMetadata(games[0]);
  }

  private mapToMetadata(game: any): Metadata {
    return {
      title: game.name,
      description: game.summary,
      releaseDate: game.release_dates?.[0]?.date ? new Date(game.release_dates[0].date * 1000) : undefined,
      genres: game.genres?.map((g: any) => g.name),
      developer: game.involved_companies?.find((c: any) => c.developer)?.company.name,
      publisher: game.involved_companies?.find((c: any) => c.publisher)?.company.name,
      rating: game.rating,
      externalIds: { igdb: game.id.toString() },
      mediaUrls: {
        coverArt: game.cover?.url?.replace('t_thumb', 't_cover_big'),
        screenshots: game.screenshots?.map((s: any) => s.url?.replace('t_thumb', 't_screenshot_big')),
      },
    };
  }
}
```

---

### 3. Google Drive Plugin

Scans Google Drive for games and downloads them.

```typescript
import { Plugin, PluginCapabilities, Game, ScanConfig } from '@mygamesanywhere/plugin-sdk';

export class GoogleDrivePlugin implements Plugin {
  id = 'gdrive';
  name = 'Google Drive';
  version = '1.0.0';
  description = 'Google Drive game repository';
  author = 'MyGamesAnywhere Team';

  capabilities: PluginCapabilities = {
    canScanAvailable: true,
    canDownload: true,
  };

  configSchema = {
    folder: {
      type: 'string' as const,
      label: 'Google Drive Folder',
      description: 'Folder ID or name containing games',
      default: 'MyGames',
    },
    oauth: {
      type: 'oauth' as const,
      label: 'Google Account',
      description: 'Connect your Google account',
      required: true,
    },
  };

  private config: PluginConfig | null = null;
  private accessToken: string | null = null;

  async initialize(config: PluginConfig): Promise<void> {
    this.config = config;
    this.accessToken = config.settings.oauth?.access_token;

    if (!this.accessToken) {
      throw new Error('Google Drive not authenticated');
    }
  }

  async destroy(): Promise<void> {
    this.config = null;
    this.accessToken = null;
  }

  async scanAvailable(scanConfig: ScanConfig): Promise<Game[]> {
    if (!this.accessToken) throw new Error('Not authenticated');

    const folderId = await this.findFolder(this.config!.settings.folder);
    if (!folderId) {
      throw new Error(`Folder "${this.config!.settings.folder}" not found`);
    }

    // List files in folder
    const response = await fetch(
      `https://www.googleapis.com/drive/v3/files?q='${folderId}'+in+parents&fields=files(id,name,size,mimeType)`,
      {
        headers: { Authorization: `Bearer ${this.accessToken}` },
      }
    );

    const data = await response.json();
    const files = data.files || [];

    // Filter game files (.exe, .zip, .7z, etc.)
    const gameFiles = files.filter((file: any) =>
      this.isGameFile(file.name)
    );

    return gameFiles.map((file: any) => ({
      id: `gdrive-${file.id}`,
      title: this.extractTitle(file.name),
      platform: 'windows',
      executionType: 'native',
      installSize: parseInt(file.size),
      source: this.id,
    }));
  }

  async download(game: Game, destination: string): Promise<void> {
    if (!this.accessToken) throw new Error('Not authenticated');

    const fileId = game.id.replace('gdrive-', '');

    const response = await fetch(
      `https://www.googleapis.com/drive/v3/files/${fileId}?alt=media`,
      {
        headers: { Authorization: `Bearer ${this.accessToken}` },
      }
    );

    const blob = await response.blob();

    // Write to destination (platform-specific)
    // Implementation depends on Electron/Capacitor APIs
  }

  private async findFolder(folderName: string): Promise<string | null> {
    const response = await fetch(
      `https://www.googleapis.com/drive/v3/files?q=name='${folderName}'+and+mimeType='application/vnd.google-apps.folder'&fields=files(id)`,
      {
        headers: { Authorization: `Bearer ${this.accessToken}` },
      }
    );

    const data = await response.json();
    return data.files?.[0]?.id || null;
  }

  private isGameFile(filename: string): boolean {
    const gameExtensions = ['.exe', '.zip', '.7z', '.rar', '.iso'];
    return gameExtensions.some(ext => filename.toLowerCase().endsWith(ext));
  }

  private extractTitle(filename: string): string {
    // Remove extension and clean up
    return filename.replace(/\.[^.]+$/, '').replace(/[._-]/g, ' ');
  }
}
```

---

### 4. Native Launcher Plugin

Launches native executables on Windows, macOS, and Linux.

```typescript
import { Plugin, PluginCapabilities, Game, LaunchOptions, Process, ProcessStatus } from '@mygamesanywhere/plugin-sdk';
import { exec, ChildProcess } from 'child_process';

export class NativeLauncherPlugin implements Plugin {
  id = 'native-launcher';
  name = 'Native Launcher';
  version = '1.0.0';
  description = 'Launch native executables';
  author = 'MyGamesAnywhere Team';

  capabilities: PluginCapabilities = {
    canLaunch: true,
    canMonitor: true,
  };

  configSchema = {};

  private runningProcesses = new Map<string, ChildProcess>();

  async initialize(config: PluginConfig): Promise<void> {
    // No configuration needed
  }

  async destroy(): Promise<void> {
    // Kill all running processes
    for (const [processId, process] of this.runningProcesses) {
      process.kill();
    }
    this.runningProcesses.clear();
  }

  async launch(game: Game, options: LaunchOptions): Promise<Process> {
    if (!game.installPath) {
      throw new Error(`Game ${game.title} has no install path`);
    }

    // Find executable
    const executable = await this.findExecutable(game.installPath);
    if (!executable) {
      throw new Error(`No executable found for ${game.title}`);
    }

    // Build command
    const args = options.arguments || [];
    const env = { ...process.env, ...(options.environment || {}) };
    const cwd = options.workingDir || game.installPath;

    // Launch process
    const childProcess = exec(
      `"${executable}" ${args.join(' ')}`,
      { cwd, env },
      (error) => {
        if (error) {
          console.error(`Process exited with error: ${error}`);
        }
      }
    );

    const processId = `native-${Date.now()}`;
    this.runningProcesses.set(processId, childProcess);

    // Clean up when process exits
    childProcess.on('exit', () => {
      this.runningProcesses.delete(processId);
    });

    return {
      processId,
      startTime: new Date(),
      game,
    };
  }

  async monitor(processId: string): Promise<ProcessStatus> {
    const process = this.runningProcesses.get(processId);

    if (!process) {
      return {
        isRunning: false,
        exitCode: 0,
        exitTime: new Date(),
      };
    }

    return {
      isRunning: !process.killed && process.exitCode === null,
    };
  }

  private async findExecutable(installPath: string): Promise<string | null> {
    // Platform-specific logic to find .exe (Windows), .app (macOS), or binary (Linux)
    // Placeholder implementation
    return installPath;
  }
}
```

---

## Plugin Lifecycle

### 1. Installation

Plugins are distributed as npm packages:

```bash
npm install @mygamesanywhere/plugin-steam
```

Or installed via the app UI:
```typescript
// App downloads plugin from registry
await pluginManager.installPlugin('steam', '1.0.0');
```

### 2. Registration

Plugins are registered in the client's plugin directory:

```
~/.mygamesanywhere/plugins/
├── steam/
│   ├── package.json
│   ├── index.js
│   └── config.json
├── gdrive/
│   ├── package.json
│   ├── index.js
│   └── config.json
└── igdb/
    ├── package.json
    ├── index.js
    └── config.json
```

### 3. Loading

When app starts:

```typescript
class PluginManager {
  private plugins = new Map<string, Plugin>();

  async loadPlugins(): Promise<void> {
    // Scan plugin directory
    const pluginDirs = await this.scanPluginDirectory();

    // Load each plugin
    for (const dir of pluginDirs) {
      try {
        const plugin = await this.loadPlugin(dir);

        // Validate plugin
        this.validatePlugin(plugin);

        // Initialize plugin
        const config = await this.getPluginConfig(plugin.id);
        await plugin.initialize(config);

        // Register plugin
        this.plugins.set(plugin.id, plugin);

        console.log(`Loaded plugin: ${plugin.name} v${plugin.version}`);
      } catch (error) {
        console.error(`Failed to load plugin from ${dir}:`, error);
      }
    }
  }

  private async loadPlugin(pluginDir: string): Promise<Plugin> {
    const packagePath = path.join(pluginDir, 'package.json');
    const pkg = JSON.parse(await fs.readFile(packagePath, 'utf-8'));

    // Dynamically import plugin module
    const module = await import(path.join(pluginDir, pkg.main || 'index.js'));

    // Instantiate plugin
    const PluginClass = module.default || module[Object.keys(module)[0]];
    return new PluginClass();
  }

  private validatePlugin(plugin: Plugin): void {
    if (!plugin.id || !plugin.name || !plugin.version) {
      throw new Error('Plugin missing required fields');
    }

    if (!plugin.capabilities || Object.keys(plugin.capabilities).length === 0) {
      throw new Error('Plugin must declare at least one capability');
    }

    if (!plugin.initialize || !plugin.destroy) {
      throw new Error('Plugin must implement lifecycle methods');
    }
  }
}
```

### 4. Configuration

Plugins are configured via UI or config file:

```typescript
// User configures Steam plugin
await pluginManager.configurePlugin('steam', {
  enabled: true,
  settings: {
    steamPath: 'C:\\Program Files (x86)\\Steam',
    apiKey: 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxx',
  },
});
```

Configuration is stored encrypted:

```typescript
// Encrypt sensitive fields (API keys, OAuth tokens)
class ConfigManager {
  async savePluginConfig(pluginId: string, config: PluginConfig): Promise<void> {
    const plugin = this.getPlugin(pluginId);

    // Encrypt sensitive fields
    const encryptedConfig = { ...config };
    for (const [key, field] of Object.entries(plugin.configSchema)) {
      if (field.type === 'password' || field.type === 'oauth') {
        encryptedConfig.settings[key] = await this.encrypt(config.settings[key]);
      }
    }

    // Save to local storage
    await this.storage.set(`plugin:${pluginId}:config`, encryptedConfig);
  }

  private async encrypt(value: string): Promise<string> {
    // Use OS keychain (DPAPI on Windows, Keychain on macOS, libsecret on Linux)
    return await this.keychain.encrypt(value);
  }
}
```

### 5. Execution

When user performs an action:

```typescript
// User clicks "Scan Steam Library"
async function scanSteamLibrary(): Promise<void> {
  const plugin = pluginManager.getPlugin('steam');

  if (!plugin || !plugin.capabilities.canScanInstalled) {
    throw new Error('Steam plugin not available');
  }

  try {
    // Call plugin method
    const games = await plugin.scanInstalled({});

    // Sync results to server
    for (const game of games) {
      await api.addGame(game);
    }

    // Update UI
    store.setGames(games);
  } catch (error) {
    // Handle error
    console.error('Steam scan failed:', error);
    notifications.show('Failed to scan Steam library', 'error');
  }
}
```

---

## Error Handling

### Plugin Errors

All plugin methods should throw detailed errors:

```typescript
class PluginError extends Error {
  constructor(
    public pluginId: string,
    public code: string,
    message: string,
    public context?: any
  ) {
    super(message);
    this.name = 'PluginError';
  }
}

// Usage in plugin
async scanInstalled(): Promise<Game[]> {
  const steamPath = this.config.settings.steamPath;

  if (!await this.pathExists(steamPath)) {
    throw new PluginError(
      this.id,
      'STEAM_NOT_FOUND',
      `Steam not found at ${steamPath}`,
      { steamPath, timestamp: new Date() }
    );
  }

  // ...
}
```

### Error Codes

- `PLUGIN_NOT_FOUND` - Plugin doesn't exist
- `PLUGIN_NOT_INITIALIZED` - Plugin not initialized
- `CONFIG_INVALID` - Invalid configuration
- `AUTH_REQUIRED` - Authentication required
- `AUTH_FAILED` - Authentication failed
- `RATE_LIMITED` - Too many API requests
- `API_ERROR` - External API error
- `FILE_NOT_FOUND` - File or directory not found
- `PERMISSION_DENIED` - Insufficient permissions

---

## Best Practices

### For Plugin Developers

1. **Fail fast with detailed errors** - Use PluginError with context
2. **Validate all inputs** - Never trust configuration or parameters
3. **Encrypt sensitive data** - Use ConfigField types correctly
4. **Handle rate limits** - Implement backoff for external APIs
5. **Test on all platforms** - Desktop (Windows, macOS, Linux) and mobile (iOS, Android)
6. **Version your plugin** - Use semantic versioning
7. **Document configuration** - Provide clear config schema
8. **Minimize dependencies** - Keep plugins lightweight
9. **No silent fallbacks** - Always throw errors, don't swallow them
10. **Use TypeScript** - Type safety catches errors early

### For App Developers

1. **Isolate plugins** - Catch all plugin errors
2. **Set timeouts** - All plugin calls have timeouts
3. **Rate limiting** - Prevent plugin abuse
4. **User feedback** - Show clear error messages
5. **Graceful degradation** - App works without failed plugin
6. **Log everything** - Plugin errors, calls, performance

---

## Platform Compatibility

### Desktop (Electron)

Plugins have access to:
- **Node.js APIs:** fs, child_process, path, etc.
- **Electron APIs:** shell, dialog, notification
- **OS Integration:** File system, process spawning, system tray

### Mobile (Capacitor)

Plugins have access to:
- **Capacitor Plugins:** Filesystem, HTTP, Storage
- **Platform APIs:** Limited due to sandboxing
- **Considerations:**
  - No direct filesystem access (use Capacitor.Filesystem)
  - No process spawning (can't launch games on mobile)
  - Limited background execution

**Plugin Capability Matrix:**

| Capability | Desktop | Mobile |
|------------|---------|--------|
| canScanInstalled | ✅ Yes | ❌ No (no local games) |
| canScanAvailable | ✅ Yes | ✅ Yes (cloud sources) |
| canDownload | ✅ Yes | ⚠️ Limited (storage constraints) |
| canSearchMetadata | ✅ Yes | ✅ Yes |
| canFetchMetadata | ✅ Yes | ✅ Yes |
| canFetchMedia | ✅ Yes | ✅ Yes |
| canLaunch | ✅ Yes | ❌ No (can't execute binaries) |
| canMonitor | ✅ Yes | ❌ No |

---

## Future Enhancements

1. **Plugin Marketplace** - Browse and install community plugins
2. **Hot Reload** - Update plugins without app restart
3. **Plugin Analytics** - Track usage and performance
4. **Plugin Testing Framework** - Automated testing tools
5. **Plugin Signing** - Verify plugin authenticity
6. **WebAssembly Plugins** - Run compiled code safely
7. **Shared Plugin SDK** - Common utilities for plugin development

---

## Summary

The plugin system is the core extensibility mechanism for MyGamesAnywhere:

- **Client-side TypeScript modules** - Runs on user's device
- **Privacy-first** - User data stays local, encrypted
- **Capability-based** - Flexible, plugins serve multiple purposes
- **Cross-platform** - Works on desktop and mobile (with platform-specific limitations)
- **Type-safe** - TypeScript interfaces prevent errors

**Key Plugins:**
- **Steam** - Scan installed games, fetch metadata, launch
- **Google Drive** - Scan cloud games, download
- **IGDB** - Metadata provider
- **Native Launcher** - Execute games on user's machine

See [ARCHITECTURE.md](./ARCHITECTURE.md) for overall system design and [DESIGN_DECISIONS.md](./DESIGN_DECISIONS.md) for rationale.
