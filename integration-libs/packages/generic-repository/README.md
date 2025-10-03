# @mygamesanywhere/generic-repository

Generic game repository scanner - detects and manages games from local and cloud storage directories.

## Overview

This package scans directories (local or cloud) and automatically detects games in various formats:

- **Installers** - Executable installers (.exe, .msi, .pkg, .deb, .rpm)
- **Portable Games** - Game directories with executables
- **ROMs** - Console/arcade game files requiring emulators
- **Archives** - Games in compressed formats (including multi-part archives)
- **Emulator Games** - Games requiring DOSBox, ScummVM, etc.

**Cross-platform support:** Windows, Linux, macOS, Android, iOS

## Installation

```bash
npm install @mygamesanywhere/generic-repository
```

## Quick Start

```typescript
import { scanLocalDirectory } from '@mygamesanywhere/generic-repository';

// Scan a local directory
const result = await scanLocalDirectory('/path/to/games');

console.log(`Found ${result.games.length} games`);
console.log(`Scanned ${result.filesScanned} files in ${result.duration}ms`);

// List detected games
for (const game of result.games) {
  console.log(`${game.name} (${game.type}) - confidence: ${game.confidence}`);
}
```

## Features

### Multi-Format Game Detection

**Installers:**
- Windows: `.exe`, `.msi`, `.bat`
- Linux: `.deb`, `.rpm`, `.run`, `.sh`
- macOS: `.pkg`, `.dmg`, `.app`

**ROMs:**
- Nintendo: NES, SNES, N64, GameBoy, GBA, DS, 3DS
- Sega: Genesis, Master System, Game Gear, 32X
- Sony: PlayStation, PlayStation 2, PSP
- Disc images: `.iso`, `.cue`

**Archives:**
- Single-part: `.zip`, `.7z`, `.rar`, `.tar.gz`
- Multi-part: `.part1.rar`, `.z01`, `.001`, `.r00`

### Intelligent Detection

- **Smart executable selection** - Identifies main game .exe vs config/uninstaller
- **Confidence scoring** - Each detection has a confidence level (0-1)
- **Game name extraction** - Cleans up filenames (removes version tags, regions, etc.)

### Repository Adapters

Supports multiple storage backends:

- **Local filesystem** - Scan local directories
- **Cloud storage** - Ready for Google Drive, OneDrive integration

## Usage Examples

### Basic Scanning

```typescript
import { LocalRepository, RepositoryScanner } from '@mygamesanywhere/generic-repository';

// Create repository adapter
const repository = new LocalRepository('/path/to/games');

// Create scanner
const scanner = new RepositoryScanner(repository);

// Scan
const result = await scanner.scan();
```

### Configure Scanning

```typescript
import { scanLocalDirectory, ScannerConfig } from '@mygamesanywhere/generic-repository';

const config: ScannerConfig = {
  maxDepth: 5,                    // Maximum directory depth
  includeHidden: false,           // Skip hidden files
  excludePatterns: ['*.tmp', '*~'], // Exclude patterns
  parallel: true,                 // Parallel scanning
  maxParallel: 5,                 // Max parallel operations
};

const result = await scanLocalDirectory('/path/to/games', config);
```

### Filter by Game Type

```typescript
import { GameType } from '@mygamesanywhere/generic-repository';

const result = await scanLocalDirectory('/path/to/games');

// Filter ROMs
const roms = result.games.filter(g => g.type === GameType.ROM);

// Filter installers
const installers = result.games.filter(g =>
  g.type === GameType.INSTALLER_EXECUTABLE ||
  g.type === GameType.INSTALLER_PLATFORM
);

// Filter archived games
const archived = result.games.filter(g => g.type === GameType.ARCHIVED);
```

### ROM System Detection

```typescript
import { FileClassifier } from '@mygamesanywhere/generic-repository';

const classifier = new FileClassifier();

// Detect ROM system from extension
const system = classifier.getROMSystem('snes');
console.log(system); // "SNES"

const system2 = classifier.getROMSystem('gba');
console.log(system2); // "Game Boy Advance"
```

### Archive Detection

```typescript
import { LocalRepository, ArchiveDetector } from '@mygamesanywhere/generic-repository';

const repository = new LocalRepository('/path/to/games');
const detector = new ArchiveDetector(repository);

// Check if multi-part archive
const isMultiPart = detector.isMultiPartName('game.part1.rar');
console.log(isMultiPart); // true

// Detect archive information
const classifiedFile = /* ... */;
const archiveInfo = await detector.detectArchive(classifiedFile);

if (archiveInfo) {
  console.log(`Archive type: ${archiveInfo.type}`);
  console.log(`Multi-part: ${archiveInfo.isMultiPart}`);
  if (archiveInfo.isMultiPart) {
    console.log(`Parts: ${archiveInfo.parts?.length}`);
  }
}
```

## API Reference

### Types

#### `GameType`

Game classification types:

```typescript
enum GameType {
  INSTALLER_EXECUTABLE = 'installer_executable',
  INSTALLER_PLATFORM = 'installer_platform',
  PORTABLE_GAME = 'portable_game',
  ROM = 'rom',
  REQUIRES_DOSBOX = 'requires_dosbox',
  REQUIRES_SCUMMVM = 'requires_scummvm',
  REQUIRES_EMULATOR = 'requires_emulator',
  ARCHIVED = 'archived',
  UNKNOWN = 'unknown',
}
```

#### `DetectedGame`

```typescript
interface DetectedGame {
  id: string;                    // Unique identifier
  name: string;                  // Detected game name
  type: GameType;                // Game type
  location: GameLocation;        // Location information
  metadata?: GameMetadata;       // Metadata (if available)
  installation?: InstallationInfo; // Installation info
  confidence: number;            // Detection confidence (0-1)
  detectedAt: Date;             // Detection timestamp
}
```

#### `ScanResult`

```typescript
interface ScanResult {
  repositoryPath: string;        // Scanned path
  games: DetectedGame[];         // Detected games
  duration: number;              // Scan duration (ms)
  filesScanned: number;          // Files scanned
  directoriesScanned: number;    // Directories scanned
  errors: ScanError[];           // Errors encountered
}
```

### Classes

#### `RepositoryScanner`

Main scanner class.

```typescript
class RepositoryScanner {
  constructor(adapter: RepositoryAdapter, config?: ScannerConfig);
  async scan(path?: string): Promise<ScanResult>;
}
```

#### `LocalRepository`

Local filesystem repository adapter.

```typescript
class LocalRepository extends BaseRepositoryAdapter {
  constructor(rootPath: string);
  async listFiles(path: string): Promise<string[]>;
  async getFileInfo(path: string): Promise<FileInfo>;
  async exists(path: string): Promise<boolean>;
  async isDirectory(path: string): Promise<boolean>;
  getRootPath(): string;
}
```

#### `FileClassifier`

File type classifier.

```typescript
class FileClassifier {
  classify(fileInfo: FileInfo): ClassifiedFile;
  isROM(file: ClassifiedFile): boolean;
  isArchive(file: ClassifiedFile): boolean;
  isInstaller(file: ClassifiedFile): boolean;
  isGameExecutable(file: ClassifiedFile): boolean;
  getROMSystem(extension: string): string | null;
}
```

#### `ArchiveDetector`

Archive detection and multi-part handling.

```typescript
class ArchiveDetector {
  constructor(adapter: RepositoryAdapter);
  async detectArchive(file: ClassifiedFile): Promise<ArchiveInfo | null>;
  isMultiPartName(filename: string): boolean;
  groupArchiveParts(files: ClassifiedFile[]): Map<string, ClassifiedFile[]>;
}
```

## Development Phases

### Phase 1: Core Scanning & Detection ✅

- [x] Repository adapter interface
- [x] Local filesystem adapter
- [x] Recursive directory walker
- [x] File type classification
- [x] Archive detection (single & multi-part)
- [x] Game type detection
- [x] Confidence scoring

### Phase 2: Archive Extraction (Planned)

- [ ] Archive extractor
- [ ] Multi-part archive extraction
- [ ] Extraction progress tracking
- [ ] Temporary extraction management
- [ ] Post-extraction game detection

### Phase 3: Installation Management (Planned)

- [ ] Installation manager
- [ ] Installer execution & tracking
- [ ] Portable game copying
- [ ] Installation state database
- [ ] Launcher script generation

### Phase 4: Metadata & Sync (Planned)

- [ ] IGDB metadata fetching
- [ ] Sidecar file support (.yaml, .json)
- [ ] Executable metadata extraction
- [ ] Save file detection
- [ ] Cloud save sync

## Cross-Platform Support

### File Extensions Supported

**Windows:**
- Executables: `.exe`, `.bat`, `.cmd`
- Installers: `.msi`

**Linux:**
- Executables: `.sh`, `.run`
- Installers: `.deb`, `.rpm`

**macOS:**
- Executables: `.app`
- Installers: `.pkg`, `.dmg`

**ROMs (All platforms):**
- Nintendo: `.nes`, `.snes`, `.n64`, `.gb`, `.gba`, `.nds`, `.3ds`
- Sega: `.smd`, `.gen`, `.sms`, `.gg`, `.32x`
- Sony: `.ps1`, `.ps2`, `.psp`
- Disc images: `.iso`, `.cue`, `.bin`

**Archives (All platforms):**
- `.zip`, `.7z`, `.rar`, `.tar`, `.gz`, `.bz2`, `.xz`

## Error Handling

The scanner gracefully handles errors:

```typescript
const result = await scanLocalDirectory('/path/to/games');

// Check for errors
if (result.errors.length > 0) {
  console.error(`Encountered ${result.errors.length} errors:`);
  for (const error of result.errors) {
    console.error(`  ${error.path}: ${error.message}`);
  }
}

// Games are still detected even if some files fail
console.log(`Successfully detected ${result.games.length} games`);
```

## Performance

- **Parallel scanning** - Optional parallel file processing
- **Configurable depth** - Limit directory traversal
- **Pattern exclusion** - Skip unwanted files/directories
- **Fast classification** - Extension-based detection

Typical performance: ~500 games/second on SSD

## License

GPL-3.0

## Contributing

See the main project [CONTRIBUTING.md](../../../CONTRIBUTING.md) for guidelines.

## Links

- [Project Documentation](../../../docs/)
- [Architecture](../../../docs/ARCHITECTURE.md)
- [Roadmap](../../../docs/ROADMAP.md)
