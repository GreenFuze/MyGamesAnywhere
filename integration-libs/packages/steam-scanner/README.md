# @mygamesanywhere/steam-scanner

Cross-platform Steam library scanner and client integration for MyGamesAnywhere.

## What This Package Does

### 🔍 Scanner (VDF Parser)
- Detects **installed games** from local Steam VDF files
- Fast, offline scanning of your Steam library folders
- No API key required
- **Only shows games currently installed on your machine**

### 🌐 Client (Steam Web API)
- Gets **all owned games** (including uninstalled) via Steam Web API
- Install/uninstall games through Steam client
- Launch games and open Steam pages
- **Requires Steam Web API key**

## Features

- 🎮 **Cross-Platform**: Works on Windows, macOS, and Linux
- 📦 **Minimal Dependencies**: Only requires Zod for validation
- 🚀 **Fast Scanner**: Scans 500-800 games/second offline
- 🔍 **Auto-Detection**: Automatically finds Steam installation
- 📊 **Complete Metadata**: Both local VDF files and Steam Web API
- 🎯 **Install/Uninstall**: Trigger Steam operations from your app
- 🛡️ **Type-Safe**: Full TypeScript support with Zod validation
- ✅ **Well-Tested**: 88 tests with 100% pass rate

## Installation

```bash
npm install @mygamesanywhere/steam-scanner
```

## Quick Start

```typescript
import { scanSteamLibrary } from '@mygamesanywhere/steam-scanner';

// Simple scan - auto-detects Steam and scans all games
const result = await scanSteamLibrary();

console.log(`Found ${result.games.length} games!`);
console.log(`Steam installed at: ${result.steamPath}`);
console.log(`Library folders: ${result.libraryFolders.length}`);

// Access game data
result.games.forEach(game => {
  console.log(`${game.name} (${game.appid})`);
  console.log(`  Size: ${game.sizeOnDisk} bytes`);
  console.log(`  Path: ${game.libraryPath}/${game.installdir}`);
});
```

## Scanner vs Client: When to Use Each

### Option 1: Scanner (VDF Files) - **Installed Games Only**

**Use when:** You want to detect games currently installed on the local machine

```typescript
import { scanSteamLibrary } from '@mygamesanywhere/steam-scanner';

const result = await scanSteamLibrary();
console.log(`${result.games.length} games installed locally`);
```

**Pros:**
- ✅ No API key required
- ✅ Works offline
- ✅ Fast (500-800 games/second)
- ✅ Includes local file paths and sizes

**Cons:**
- ❌ **Only shows installed games**
- ❌ Missing games the user owns but hasn't installed

---

### Option 2: Steam Web API - **All Owned Games**

**Use when:** You want to show the user's complete Steam library (including uninstalled games)

```typescript
import { SteamClient } from '@mygamesanywhere/steam-scanner';

const client = new SteamClient({
  apiKey: 'YOUR_STEAM_API_KEY',
  steamId: 'YOUR_STEAM_ID_64',
});

const allGames = await client.getOwnedGames();
console.log(`${allGames.length} games owned (installed + uninstalled)`);
```

**Pros:**
- ✅ Shows **ALL** games the user owns
- ✅ Includes playtime data
- ✅ Includes game icons/logos

**Cons:**
- ❌ Requires Steam Web API key
- ❌ Requires internet connection
- ❌ Doesn't include local file paths/sizes

---

### Best Approach: Use Both Together!

**Recommended pattern for MyGamesAnywhere:**

```typescript
import { scanSteamLibrary, SteamClient } from '@mygamesanywhere/steam-scanner';

// 1. Get ALL owned games from Web API
const client = new SteamClient({ apiKey, steamId });
const allOwnedGames = await client.getOwnedGames();

// 2. Get installed games from local scan
const scanResult = await scanSteamLibrary();
const installedGames = new Set(scanResult.games.map(g => g.appid));

// 3. Combine: Mark which games are installed
const library = allOwnedGames.map(game => ({
  ...game,
  isInstalled: installedGames.has(game.appid.toString()),
  localPath: scanResult.games.find(g => g.appid === game.appid.toString())?.libraryPath,
}));

console.log(`Total library: ${library.length} games`);
console.log(`Installed: ${library.filter(g => g.isInstalled).length}`);
console.log(`Not installed: ${library.filter(g => !g.isInstalled).length}`);
```

**This gives you:**
- ✅ Complete game library (owned but not installed)
- ✅ Installation status for each game
- ✅ Local file paths for installed games
- ✅ Can show "Install" button for uninstalled games

## Advanced Usage

### Custom Steam Path

```typescript
import { SteamScanner } from '@mygamesanywhere/steam-scanner';

const scanner = new SteamScanner();

// Initialize with custom Steam path
await scanner.initialize({
  steamPath: 'D:\\Custom\\Steam'
});

const result = await scanner.scan();
```

### Error Handling

```typescript
import {
  scanSteamLibrary,
  SteamNotFoundError,
  FileAccessError,
  VDFParseError
} from '@mygamesanywhere/steam-scanner';

try {
  const result = await scanSteamLibrary();
  console.log('Scan successful!');
} catch (error) {
  if (error instanceof SteamNotFoundError) {
    console.error('Steam is not installed on this system');
  } else if (error instanceof FileAccessError) {
    console.error('Cannot access Steam files:', error.filePath);
  } else if (error instanceof VDFParseError) {
    console.error('Failed to parse VDF file:', error.filePath);
  } else {
    console.error('Unexpected error:', error);
  }
}
```

### Manual Detection

```typescript
import { SteamScanner } from '@mygamesanywhere/steam-scanner';

const scanner = new SteamScanner();

// Just detect Steam path without scanning
const steamPath = await scanner.detectSteamPath();
console.log(`Steam found at: ${steamPath}`);

// Then scan later
await scanner.initialize();
const result = await scanner.scan();
```

### VDF Parser (Low-Level)

```typescript
import { parseVDF } from '@mygamesanywhere/steam-scanner';
import { readFile } from 'fs/promises';

// Parse any VDF file
const vdfContent = await readFile('libraryfolders.vdf', 'utf-8');
const parsed = parseVDF(vdfContent);

console.log(parsed);
```

### Steam Client Operations

```typescript
import { SteamClient } from '@mygamesanywhere/steam-scanner';

// Create client (no API key needed for basic operations)
const client = new SteamClient();

// Install a game (opens Steam install dialog)
await client.installGame('440'); // Team Fortress 2

// Uninstall a game (opens Steam uninstall dialog)
await client.uninstallGame('440');

// Launch a game
await client.launchGame('440');

// Open Steam store page
await client.openStorePage('440');

// Validate game files
await client.validateGameFiles('440');

// Check if Steam is running
const isRunning = await client.isSteamRunning();

// Get game images
const headerUrl = client.getGameHeaderUrl('440');
const heroUrl = client.getGameHeroUrl('440');
const capsuleUrl = client.getGameCapsuleUrl('440');
```

### Steam Web API (All Owned Games)

```typescript
import { SteamClient } from '@mygamesanywhere/steam-scanner';

// Create client with API credentials
// Get API key from: https://steamcommunity.com/dev/apikey
const client = new SteamClient({
  apiKey: 'YOUR_STEAM_API_KEY',
  steamId: 'YOUR_STEAM_ID_64', // Find at: https://steamid.io/
});

// Get all owned games (installed + uninstalled)
const allGames = await client.getOwnedGames();

allGames.forEach(game => {
  console.log(`${game.name} (${game.appid})`);
  console.log(`  Playtime: ${Math.floor(game.playtime_forever / 60)} hours`);
});

// Get recently played games
const recentGames = await client.getRecentlyPlayedGames(10);

// Get app details from Steam Store API (no auth required)
const appDetails = await client.getAppDetails('440');
```

## API Reference

### `scanSteamLibrary(config?: ScanConfig): Promise<ScanResult>`

High-level function that auto-detects Steam and scans all games.

**Returns:**
```typescript
interface ScanResult {
  games: SteamGame[];
  libraryFolders: SteamLibraryFolder[];
  steamPath: string;
  scanDuration: number;
}
```

### `SteamScanner`

Low-level class for more control over the scanning process.

**Methods:**
- `detectSteamPath(): Promise<string>` - Auto-detect Steam installation
- `initialize(config?: ScanConfig): Promise<void>` - Initialize scanner
- `scan(config?: ScanConfig): Promise<ScanResult>` - Scan Steam library

### `parseVDF(content: string): VDFObject`

Parse VDF file content into JavaScript object.

**Parameters:**
- `content` - VDF file content as string

**Returns:**
- Parsed object with VDF structure

**Throws:**
- `VDFParseError` - If VDF syntax is invalid

### `SteamClient`

Client for Steam operations and Web API integration.

**Constructor:**
```typescript
new SteamClient(webAPIConfig?: SteamWebAPIConfig)
```

**Game Management Methods:**
- `installGame(appId: string): Promise<void>` - Open Steam install dialog
- `uninstallGame(appId: string): Promise<void>` - Open Steam uninstall dialog
- `launchGame(appId: string): Promise<void>` - Launch game via Steam
- `openStorePage(appId: string): Promise<void>` - Open Steam store page
- `validateGameFiles(appId: string): Promise<void>` - Validate/repair game files

**Steam Client Methods:**
- `isSteamRunning(): Promise<boolean>` - Check if Steam client is running
- `openLibrary(): Promise<void>` - Open Steam library
- `openDownloads(): Promise<void>` - Open Steam downloads page
- `openSettings(): Promise<void>` - Open Steam settings

**Steam Web API Methods (requires API key):**
- `getOwnedGames(): Promise<SteamWebGame[]>` - Get all owned games
- `getRecentlyPlayedGames(count?: number): Promise<SteamWebGame[]>` - Get recently played
- `getAppDetails(appId: string): Promise<unknown>` - Get app details from Store API

**Image URL Helpers:**
- `getGameHeaderUrl(appId: string): string` - Get header image URL
- `getGameHeroUrl(appId: string): string` - Get library hero image URL
- `getGameCapsuleUrl(appId: string): string` - Get library capsule image URL
- `getGameIconUrl(appId: string, iconHash: string): string` - Get icon URL
- `getGameLogoUrl(appId: string, logoHash: string): string` - Get logo URL

## Data Types

### `SteamGame`

```typescript
interface SteamGame {
  appid: string;           // Steam App ID
  name: string;            // Game name
  installdir: string;      // Installation directory name
  libraryPath: string;     // Full path to library folder
  lastUpdated: number;     // Unix timestamp
  sizeOnDisk: string;      // Size in bytes (as string)
  buildId?: string;        // Build ID (optional)
  lastOwner?: string;      // Last owner Steam ID (optional)
}
```

### `SteamLibraryFolder`

```typescript
interface SteamLibraryFolder {
  path: string;            // Full path to library
  label: string;           // User-defined label
  contentid: string;       // Content ID
  totalsize: string;       // Total size in bytes
}
```

## Error Types

All errors extend `SteamScannerError`:

- **`SteamNotFoundError`** - Steam installation not found
- **`FileAccessError`** - Cannot access Steam files
- **`VDFParseError`** - VDF file parsing failed
- **`InvalidConfigError`** - Invalid configuration provided

Each error has a `code` property for programmatic handling:
```typescript
try {
  await scanSteamLibrary();
} catch (error) {
  if (error.code === 'STEAM_NOT_FOUND') {
    // Handle Steam not found
  }
}
```

## Platform Support

### Windows
- Default paths:
  - `C:\Program Files (x86)\Steam`
  - `C:\Program Files\Steam`

### macOS
- Default path:
  - `~/Library/Application Support/Steam`

### Linux
- Default paths:
  - `~/.steam/steam`
  - `~/.local/share/Steam`

## Performance

Typical scan performance:
- **Scan Speed**: 500-800 games/second
- **Average per game**: 1-2ms
- **Memory**: ~10-50MB depending on library size

Example from integration tests:
```
Games Found: 4
Scan Duration: 5ms
Games/second: 800.00
```

## Testing

```bash
# Run all tests
npm test

# Run tests in watch mode
npm run test:watch

# Run tests with coverage
npm run test:coverage
```

Test coverage:
- **88 tests** across 5 test suites
- Unit tests with mocking
- Edge case tests (30+ scenarios)
- Integration tests with real Steam installation
- Error handling tests

## Examples

### Example: Find games by size

```typescript
import { scanSteamLibrary } from '@mygamesanywhere/steam-scanner';

const result = await scanSteamLibrary();

// Find games larger than 10GB
const largeGames = result.games.filter(game => {
  const sizeInGB = parseInt(game.sizeOnDisk) / (1024 ** 3);
  return sizeInGB > 10;
});

console.log(`Found ${largeGames.length} games larger than 10GB`);
```

### Example: Group games by library

```typescript
import { scanSteamLibrary } from '@mygamesanywhere/steam-scanner';

const result = await scanSteamLibrary();

const gamesByLibrary = new Map<string, typeof result.games>();

result.games.forEach(game => {
  const library = game.libraryPath;
  if (!gamesByLibrary.has(library)) {
    gamesByLibrary.set(library, []);
  }
  gamesByLibrary.get(library)!.push(game);
});

gamesByLibrary.forEach((games, library) => {
  console.log(`\n${library}: ${games.length} games`);
  games.forEach(game => console.log(`  - ${game.name}`));
});
```

### Example: Check if specific game is installed

```typescript
import { scanSteamLibrary } from '@mygamesanywhere/steam-scanner';

async function isGameInstalled(appId: string): Promise<boolean> {
  const result = await scanSteamLibrary();
  return result.games.some(game => game.appid === appId);
}

// Check if Team Fortress 2 (440) is installed
const hasTF2 = await isGameInstalled('440');
console.log(`TF2 installed: ${hasTF2}`);
```

## Part of MyGamesAnywhere

This package is part of the MyGamesAnywhere project - a cross-platform game launcher and manager.

- **GitHub:** https://github.com/GreenFuze/MyGamesAnywhere

## License

GPL-3.0

## Contributing

Contributions welcome! This is part of the larger MyGamesAnywhere project.

## Roadmap

- [ ] Steam Cloud save file detection
- [ ] Workshop content detection
- [ ] Proton compatibility data (Linux)
- [ ] Shader cache detection
- [ ] Screenshot folder detection
