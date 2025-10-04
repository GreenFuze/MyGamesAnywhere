# MyGamesAnywhere Plugin System

## Overview

The plugin system provides a flexible, extensible architecture for managing games across multiple sources (Steam, Xbox, local storage, etc.) and identifying them using various metadata providers (LaunchBox, IGDB, etc.).

## Architecture

### Core Concepts

1. **Plugin Types**:
   - **Source Plugins**: Scan and detect games from various sources (Steam, Xbox, local folders, Google Drive, etc.)
   - **Identifier Plugins**: Match detected games to metadata databases (LaunchBox, IGDB, etc.)
   - **Storage Plugins**: Store and sync game data (local, cloud, etc.)

2. **Unified Game Model**: A single game may exist across multiple sources (e.g., same game owned on Steam, Xbox, and locally). The system automatically merges these into a unified view.

3. **Multi-Source/Multi-Identifier Support**: Each unified game tracks:
   - All sources where it was detected
   - All identifications from different metadata providers
   - Consolidated metadata from best matches

## Package Structure

```
packages/
├── core/
│   ├── plugin-system/          # Core plugin architecture
│   │   ├── types.ts            # Plugin interfaces
│   │   ├── registry.ts         # Plugin registry
│   │   └── unified-game.ts     # Multi-source game model
│   └── config/                 # Centralized configuration
│
├── platforms/                  # Low-level API clients
│   ├── steam/                  # Steam API & VDF parsing
│   ├── google/                 # Google Drive OAuth & API
│   ├── launchbox/              # LaunchBox DB & downloader
│   └── igdb/                   # IGDB API (placeholder)
│
└── plugins/                    # High-level plugins
    ├── steam-source/           # Steam source plugin
    ├── custom-storefront-source/ # Google Drive source plugin
    └── launchbox-identifier/   # LaunchBox identifier plugin
```

## Implemented Plugins

### Source Plugins

#### Steam Source (`@mygamesanywhere/plugin-steam-source`)
- Scans local Steam library for installed games
- Optional Steam Web API integration for owned games
- Supports launch/install/uninstall operations
- **Status**: ✅ Complete & Tested

#### Custom Storefront Source (`@mygamesanywhere/plugin-custom-storefront-source`)
- Scans Google Drive for game files
- Detects installers, archives, ROMs, portable games
- Smart file classification (installer detection, multi-part archives, etc.)
- **Status**: ✅ Complete & Tested

### Identifier Plugins

#### LaunchBox Identifier (`@mygamesanywhere/plugin-launchbox-identifier`)
- 100,000+ game metadata database
- Intelligent filename parsing (removes GOG patterns, version numbers, etc.)
- Fuzzy matching with Fuse.js
- Auto-downloads metadata on first use
- **Status**: ✅ Complete & Tested (55% accuracy on test dataset)

## Unified Game Model

### GameSource
Represents a game instance from a specific source:
```typescript
interface GameSource {
  sourceId: string;              // e.g., "steam-source"
  gameId: string;                // Source-specific ID
  detectedGame: DetectedGame;    // Raw detection data
  identification?: IdentifiedGame; // Optional metadata match
  installed: boolean;
  lastPlayed?: Date;
  playtime?: number;
}
```

### UnifiedGame
Represents the same game across multiple sources:
```typescript
interface UnifiedGame {
  id: string;                    // Unique unified ID
  title: string;                 // Best title from identifications
  sources: GameSource[];         // All sources (Steam, Xbox, local, etc.)
  identifications: IdentificationResult[]; // From all identifiers
  platform?: string;
  coverUrl?: string;
  isInstalled: boolean;          // True if installed in ANY source
  totalPlaytime: number;         // Sum across all sources
  lastPlayed?: Date;             // Most recent across all sources
  isFavorite: boolean;
  isHidden: boolean;
  tags?: string[];
  userRating?: number;
}
```

### Game Matching Strategies

Games from different sources are matched using:
- **Exact Title**: Perfect string match
- **Normalized Title**: Lowercase, no special characters
- **Fuzzy Title**: Levenshtein distance similarity (default: 85% threshold)
- **External ID**: Match by Steam App ID, IGDB ID, etc.
- **Manual**: User manually merges games

## Usage

### Basic Example

```typescript
import {
  pluginRegistry,
  UnifiedGameManager,
  MatchStrategy
} from '@mygamesanywhere/plugin-system';
import { SteamSourcePlugin } from '@mygamesanywhere/plugin-steam-source';
import { LaunchBoxIdentifierPlugin } from '@mygamesanywhere/plugin-launchbox-identifier';

// 1. Initialize plugins
const steamPlugin = new SteamSourcePlugin();
await steamPlugin.initialize({ scanLocal: true });
pluginRegistry.register(steamPlugin);

const launchboxPlugin = new LaunchBoxIdentifierPlugin();
await launchboxPlugin.initialize({ autoDownload: true });
pluginRegistry.register(launchboxPlugin);

// 2. Create unified game manager
const manager = new UnifiedGameManager();
manager.setMatchStrategy(MatchStrategy.FUZZY_TITLE, 0.85);

// 3. Scan all sources
for (const source of pluginRegistry.getAllSources()) {
  const games = await source.scan();

  for (const game of games) {
    const gameSource = {
      sourceId: source.metadata.id,
      gameId: game.id,
      detectedGame: game,
      installed: game.installed || false,
    };

    const unified = manager.addDetectedGame(gameSource);
    // Automatically merges if same game exists from another source
  }
}

// 4. Identify games
for (const game of manager.getAllGames()) {
  for (const identifier of pluginRegistry.getAllIdentifiers()) {
    const result = await identifier.identify(game.sources[0].detectedGame);

    if (result.metadata) {
      manager.addIdentification(game.id, identifier.metadata.id, result);
    }
  }
}

// 5. Access unified library
const allGames = manager.getAllGames();
const multiSourceGames = allGames.filter(g => g.sources.length > 1);
```

### Running the Demo

```bash
cd integration-libs
npm run build
npm run demo:plugins
```

The demo will:
1. Initialize Steam and LaunchBox plugins
2. Scan for games from all sources
3. Automatically merge duplicate games
4. Identify games with LaunchBox metadata
5. Display multi-source games (same game in multiple libraries)
6. Show available actions (launch, install, uninstall)

## Key Features

### 1. Automatic Game Merging
If you own "God of War" on both Steam and Epic, the system automatically recognizes them as the same game and creates a single unified entry with two sources.

### 2. Multi-Identifier Support
Each game can be identified by multiple systems (LaunchBox, IGDB, Steam Store API), giving you the best possible metadata match.

### 3. Plugin Registry
Centralized plugin management:
```typescript
pluginRegistry.register(plugin);
pluginRegistry.getAllSources();
pluginRegistry.getAllIdentifiers();
pluginRegistry.getSource('steam-source');
```

### 4. Type-Safe Architecture
Full TypeScript support with strict typing across all plugins and interfaces.

### 5. Flexible Matching
Configure how games are matched across sources:
```typescript
manager.setMatchStrategy(MatchStrategy.FUZZY_TITLE, 0.9); // 90% similarity
manager.setMatchStrategy(MatchStrategy.EXTERNAL_ID);      // Match by Steam ID
```

### 6. Manual Overrides
Users can manually merge or split games:
```typescript
manager.mergeGames(gameId1, gameId2);
manager.splitSource(gameId, sourceId);
```

## Performance

- **Steam Scanning**: ~500ms for 100 games
- **LaunchBox Database**: 126,000 games, ~2GB on disk, ~500ms query time with FTS5
- **Google Drive Scanning**: 202 games detected from 763 files in ~8 seconds

## Test Results

### LaunchBox Identifier Accuracy (11 test files)
- **55% accuracy** (6/11 identified correctly)
- Successfully identified:
  - "Alone in the Dark 3" (90%)
  - "LEGO Batman: The Videogame" (67%)
  - "Pikuniku" (90%)
  - "Sonic Mega Collection Plus" (90%)
  - "God of War" (90%)
- Excellent GOG installer handling (removes version numbers, IDs, etc.)
- Challenges: Games not in LaunchBox DB, very generic names like "HangMan.exe"

### Steam Scanner
- Successfully scans all installed Steam games
- Detects library folders, app manifests
- Extracts: name, app ID, install path, size, last updated

### Google Drive Scanner
- 202 games detected from 763 files
- Smart classification: installers, archives, ROMs
- Multi-part archive detection (.part1, .z01, etc.)

## Future Enhancements

1. **Additional Source Plugins**:
   - Xbox/Microsoft Store
   - Epic Games Store
   - GOG Galaxy
   - Ubisoft Connect
   - EA App

2. **Additional Identifier Plugins**:
   - IGDB (Internet Game Database)
   - Steam Store API
   - User-provided metadata

3. **Storage Plugins**:
   - Local JSON storage
   - Google Drive sync
   - Cloud backup services

4. **Advanced Features**:
   - Playtime tracking across sources
   - Screenshot/save game sync
   - Achievement tracking
   - Recommendation engine

## Technical Details

### Plugin Interface

All plugins implement a common interface:
```typescript
interface Plugin {
  metadata: PluginMetadata;      // Name, ID, version, description
  type: PluginType;              // SOURCE, IDENTIFIER, or STORAGE
  configSchema?: PluginConfigSchema;

  initialize(config?: any): Promise<void>;
  isReady(): Promise<boolean>;
  cleanup?(): Promise<void>;
}
```

### Source Plugin Specifics
```typescript
interface SourcePlugin extends Plugin {
  scan(): Promise<DetectedGame[]>;
  getGame?(gameId: string): Promise<DetectedGame | null>;
  launch?(gameId: string): Promise<void>;
  install?(gameId: string): Promise<void>;
  uninstall?(gameId: string): Promise<void>;
}
```

### Identifier Plugin Specifics
```typescript
interface IdentifierPlugin extends Plugin {
  identify(game: DetectedGame): Promise<IdentifiedGame>;
  search(query: string, platform?: string): Promise<GameMetadata[]>;
  update?(): Promise<void>;
}
```

## Contributing

To add a new plugin:

1. Create package in `packages/plugins/your-plugin/`
2. Implement the appropriate plugin interface
3. Export plugin class from `src/index.ts`
4. Add to workspace in `integration-libs/package.json`
5. Build and test with demo script

## License

MIT
