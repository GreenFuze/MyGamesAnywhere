# @mygamesanywhere/game-identifier

Game identification and metadata matching for MyGamesAnywhere.

Identifies games from filenames and matches them to metadata from various sources including LaunchBox Games Database, IGDB, and Steam.

## Features

- **LaunchBox Games Database** - Local SQLite database with 100,000+ games
  - Downloads and parses Metadata.zip (450MB XML files)
  - Full-text search with FTS5
  - Fuzzy matching with Fuse.js
  - Platform-specific matching

- **Smart Name Extraction** - Cleans filenames to extract game names
  - Removes region codes (USA, Europe, Japan, etc.)
  - Extracts platform information (PS3, SNES, NES, etc.)
  - Handles multi-part files (.part1, .z01, .001, etc.)
  - Detects language indicators (En, Ja, Fr, etc.)
  - Removes installer prefixes (setup_, install_, etc.)

- **Confidence Scoring** - Match quality scoring
  - Name similarity (Levenshtein distance)
  - Platform matching
  - Metadata richness

- **Future Sources**
  - IGDB (Internet Game Database)
  - Steam Web API
  - Custom metadata providers

## Installation

```bash
npm install @mygamesanywhere/game-identifier
```

## Quick Start

```typescript
import { LaunchBoxIdentifier } from '@mygamesanywhere/game-identifier';

// Create identifier (auto-downloads metadata on first run)
const identifier = new LaunchBoxIdentifier({
  autoDownload: true,
  minConfidence: 0.5,
  maxResults: 10,
});

// Identify a game
const detectedGame = {
  id: '1',
  name: 'setup_alone_in_the_dark_3_1.0_cs_(28191).exe',
  type: 'installer_executable',
  path: '/downloads/game.exe',
  size: 512000000,
  confidence: 0.8,
};

const result = await identifier.identify(detectedGame);

if (result.metadata) {
  console.log(`Found: ${result.metadata.title}`);
  console.log(`Platform: ${result.metadata.platform}`);
  console.log(`Confidence: ${result.matchConfidence * 100}%`);
}

// Close when done
identifier.close();
```

## Manual Search

```typescript
// Search by name
const results = await identifier.search('God of War', 'PlayStation 3');

for (const game of results) {
  console.log(`${game.title} (${game.platform})`);
}
```

## Update Metadata

```typescript
// Update LaunchBox database (downloads latest metadata)
await identifier.update();
```

## Name Extraction

```typescript
import { NameExtractor } from '@mygamesanywhere/game-identifier';

const extractor = new NameExtractor();

const result = extractor.extract('Devil May Cry - HD Collection (USA) (En,Ja,Fr,De,Es,It).ps3');

console.log(result.cleanName);  // "Devil May Cry HD Collection"
console.log(result.platform);   // "PlayStation 3"
console.log(result.region);     // "USA"
console.log(result.languages);  // ["En", "Ja", "Fr", "De", "Es", "It"]
console.log(result.confidence); // 0.85
```

## Supported Filename Patterns

- **ROM files**: `Game Name (Region).extension`
- **Installers**: `setup_game_name_version.exe`
- **Archives**: `Game.Name.v1.0.zip`
- **Multi-part**: `game-1.bin`, `game.part1`, `game.z01`, `game.001`
- **Platform indicators**: `.ps3`, `.snes`, `.nes`, `.gba`, etc.
- **Region codes**: `(U)`, `(USA)`, `(E)`, `(EUR)`, `(J)`, `(JPN)`, etc.
- **Languages**: `(En,Ja,Fr,De,Es,It)`

## LaunchBox Games Database

### Attribution

Game metadata provided by [LaunchBox Games Database](https://gamesdb.launchbox-app.com/).

All data and credit goes to LaunchBox. MyGamesAnywhere does not own any rights to the LaunchBox metadata. The metadata is used for entertainment and game organization purposes only.

### Usage

- **Source**: https://gamesdb.launchbox-app.com/Metadata.zip
- **License**: Community-contributed database
- **Files**:
  - `Metadata.xml` (~450MB) - Main game database
  - `Platforms.xml` (~300KB) - Platform information
  - `Files.xml` (~1MB) - ROM checksums
  - `Mame.xml` (~36MB) - Arcade games

### Privacy

- Metadata is downloaded to `~/.mygamesanywhere/metadata/launchbox/`
- SQLite database stored at `~/.mygamesanywhere/metadata/launchbox/launchbox.db`
- No data is sent to LaunchBox servers
- All processing is local

## API Reference

### LaunchBoxIdentifier

```typescript
class LaunchBoxIdentifier implements GameIdentifier {
  constructor(config?: LaunchBoxIdentifierConfig);

  // Check if metadata is ready
  async isReady(): Promise<boolean>;

  // Update metadata database
  async update(): Promise<void>;

  // Identify a detected game
  async identify(detectedGame: DetectedGame): Promise<IdentifiedGame>;

  // Search for games by name
  async search(query: string, platform?: string): Promise<GameMetadata[]>;

  // Get database statistics
  getStats(): { games: number; platforms: number; genres: number; files: number };

  // Close database connection
  close(): void;
}
```

### NameExtractor

```typescript
class NameExtractor {
  // Extract clean game name from filename
  extract(filename: string): ExtractedName;
}

interface ExtractedName {
  cleanName: string;
  platform?: string;
  region?: string;
  version?: string;
  languages?: string[];
  isPart?: boolean;
  partNumber?: number;
  confidence: number;
}
```

## Testing

```bash
# Test with example filenames
npm run test:launchbox

# Run unit tests
npm test
```

## Storage Locations

- **Metadata.zip**: `~/.mygamesanywhere/metadata/launchbox/Metadata.zip`
- **Extracted XML**: `~/.mygamesanywhere/metadata/launchbox/extracted/`
- **SQLite Database**: `~/.mygamesanywhere/metadata/launchbox/launchbox.db`

## Performance

- **First run**: 5-10 minutes (downloads 450MB and parses XML)
- **Subsequent runs**: Instant (uses local SQLite database)
- **Search speed**: ~1-5ms per query (FTS5 + indexes)
- **Fuzzy matching**: ~10-50ms per query (Fuse.js)

## Roadmap

- [ ] IGDB integration
- [ ] Steam Web API integration
- [ ] Custom metadata providers
- [ ] Batch identification
- [ ] Caching improvements
- [ ] Multi-language support
- [ ] Cover image downloads

## License

GPL-3.0 - See LICENSE file for details.

## Acknowledgments

- **LaunchBox Games Database** - Game metadata
- **Fuse.js** - Fuzzy search
- **better-sqlite3** - SQLite database
- **fast-xml-parser** - XML parsing
