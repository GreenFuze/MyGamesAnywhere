# Phase 1: Core Integrations - Detailed Specification

**Goal:** Build and test core integration libraries as standalone TypeScript packages. Prove that the hardest integrations work BEFORE building UI or server.

**Duration:** 5-6 weeks

**Outcome:** Four working integration libraries with unit tests:
1. **Steam Scanner** - Scans installed Steam games, reads VDF files
2. **Google Drive Client** - OAuth authentication, file discovery, download
3. **IGDB Metadata Client** - Search games, fetch metadata, handle rate limits
4. **Native Launcher** - Launch executables, monitor processes

**Platform:** Desktop (Windows focus, with macOS/Linux compatibility where possible)

**NO UI, NO Server** - Just prove the integrations work!

---

## Why This Approach?

**Risk Mitigation:**
- Integrations are the hardest and most unknown part
- Building UI first risks discovering integrations don't work
- Standalone packages are easier to test and debug
- Can reuse these packages later when adding plugin system

**Benefits:**
- Validate Steam VDF parsing works
- Validate Google Drive OAuth flow works
- Validate IGDB API integration works
- Validate game launching works
- Build confidence before committing to full architecture

---

## Technology Stack

### Core
- **Language:** TypeScript 5
- **Runtime:** Node.js 18+
- **Package Manager:** npm or pnpm
- **Testing:** Vitest
- **Validation:** Zod

### Integration-Specific
- **File Parsing:** Custom VDF parser (Steam)
- **OAuth:** google-auth-library (Google Drive)
- **HTTP Client:** axios
- **Process Management:** Node.js child_process
- **File System:** Node.js fs/promises

### Development Tools
- **Linting:** ESLint + Prettier
- **Type Checking:** tsc --noEmit
- **Test Coverage:** Vitest coverage
- **Monorepo:** Nx (optional, can use simple workspace structure)

---

## Package Structure

```
integration-libs/
├── packages/
│   ├── steam-scanner/
│   │   ├── src/
│   │   │   ├── index.ts           # Public API
│   │   │   ├── scanner.ts         # Main scanner logic
│   │   │   ├── vdf-parser.ts      # VDF file parser
│   │   │   ├── types.ts           # TypeScript types
│   │   │   └── errors.ts          # Error classes
│   │   ├── tests/
│   │   │   ├── scanner.test.ts
│   │   │   ├── vdf-parser.test.ts
│   │   │   └── fixtures/          # Sample .acf files
│   │   ├── package.json
│   │   ├── tsconfig.json
│   │   └── README.md
│   │
│   ├── gdrive-client/
│   │   ├── src/
│   │   │   ├── index.ts           # Public API
│   │   │   ├── client.ts          # Google Drive client
│   │   │   ├── auth.ts            # OAuth flow
│   │   │   ├── types.ts
│   │   │   └── errors.ts
│   │   ├── tests/
│   │   │   ├── client.test.ts
│   │   │   ├── auth.test.ts
│   │   │   └── mocks/             # Mocked API responses
│   │   ├── package.json
│   │   └── README.md
│   │
│   ├── igdb-client/
│   │   ├── src/
│   │   │   ├── index.ts           # Public API
│   │   │   ├── client.ts          # IGDB client
│   │   │   ├── auth.ts            # Twitch OAuth
│   │   │   ├── rate-limiter.ts    # Rate limiting
│   │   │   ├── types.ts
│   │   │   └── errors.ts
│   │   ├── tests/
│   │   │   ├── client.test.ts
│   │   │   ├── rate-limiter.test.ts
│   │   │   └── mocks/
│   │   ├── package.json
│   │   └── README.md
│   │
│   └── native-launcher/
│       ├── src/
│       │   ├── index.ts           # Public API
│       │   ├── launcher.ts        # Process spawning
│       │   ├── monitor.ts         # Process monitoring
│       │   ├── platform.ts        # Platform detection
│       │   ├── types.ts
│       │   └── errors.ts
│       ├── tests/
│       │   ├── launcher.test.ts
│       │   ├── monitor.test.ts
│       │   └── fixtures/          # Test executables
│       ├── package.json
│       └── README.md
│
├── package.json              # Workspace root
├── tsconfig.base.json        # Shared TypeScript config
└── README.md
```

---

## Package 1: Steam Scanner

### Purpose
Scan user's Steam installation and extract installed game information from `.acf` files.

### Features
- Detect Steam installation path (Windows, macOS, Linux)
- Parse `libraryfolders.vdf` to find all library folders
- Scan `.acf` files in each library folder
- Extract game info: app ID, title, install directory, install size
- Handle parsing errors gracefully

### Public API

```typescript
// src/index.ts
export { SteamScanner } from './scanner';
export { SteamGame, ScanOptions, ScanResult } from './types';
export { SteamScannerError } from './errors';

// Usage
import { SteamScanner } from '@mygamesanywhere/steam-scanner';

const scanner = new SteamScanner({
  steamPath: 'C:\\Program Files (x86)\\Steam',
});

const result = await scanner.scanInstalled();
// result.games = [{ appId: '730', title: 'Counter-Strike 2', ... }, ...]
```

### Types

```typescript
// src/types.ts
export interface SteamGame {
  appId: string;
  title: string;
  installDir: string;
  installPath: string;  // Full path to installation
  sizeOnDisk?: number;  // Bytes
  lastUpdated?: Date;
  buildId?: string;
}

export interface ScanOptions {
  steamPath?: string;   // Auto-detect if not provided
}

export interface ScanResult {
  games: SteamGame[];
  libraryPaths: string[];
  errors: Error[];      // Non-fatal errors during scan
}
```

### VDF Parser

```typescript
// src/vdf-parser.ts
export class VDFParser {
  /**
   * Parse VDF (Valve Data Format) file
   * Example:
   * "AppState"
   * {
   *   "appid"  "730"
   *   "name"   "Counter-Strike 2"
   *   "installdir"  "Counter-Strike Global Offensive"
   * }
   */
  static parse(content: string): Record<string, any> {
    // Implementation
  }

  static parseFile(filePath: string): Promise<Record<string, any>> {
    // Read file and parse
  }
}
```

### Tests

```typescript
// tests/vdf-parser.test.ts
import { describe, it, expect } from 'vitest';
import { VDFParser } from '../src/vdf-parser';

describe('VDFParser', () => {
  it('should parse simple VDF', () => {
    const vdf = `
      "AppState"
      {
        "appid"  "730"
        "name"   "Counter-Strike 2"
      }
    `;

    const result = VDFParser.parse(vdf);
    expect(result.AppState.appid).toBe('730');
    expect(result.AppState.name).toBe('Counter-Strike 2');
  });

  it('should parse nested VDF', () => {
    // Test nested objects
  });

  it('should handle quoted values with spaces', () => {
    // Test values like "Program Files"
  });
});

// tests/scanner.test.ts
describe('SteamScanner', () => {
  it('should detect Steam installation path', async () => {
    const scanner = new SteamScanner();
    const path = await scanner.detectSteamPath();
    expect(path).toBeTruthy();
  });

  it('should parse libraryfolders.vdf', async () => {
    // Test with fixture file
  });

  it('should scan installed games', async () => {
    // Test with real Steam installation or mocked files
  });

  it('should handle missing Steam gracefully', async () => {
    const scanner = new SteamScanner({ steamPath: '/nonexistent' });
    await expect(scanner.scanInstalled()).rejects.toThrow();
  });
});
```

### Definition of Done
- [ ] Can detect Steam installation path on Windows
- [ ] Can parse `libraryfolders.vdf` correctly
- [ ] Can parse `.acf` files and extract game info
- [ ] Can scan all library folders
- [ ] Returns array of games with correct data
- [ ] Handles missing Steam installation
- [ ] Handles corrupted VDF files
- [ ] Unit tests pass (90%+ coverage)
- [ ] Type-safe API
- [ ] README with usage examples

---

## Package 2: Google Drive Client

### Purpose
Authenticate with Google Drive OAuth and scan for game files.

### Features
- OAuth 2.0 authentication flow
- Token storage and refresh
- Search for files in specific folder
- Download files
- Handle API rate limits
- Retry on transient failures

### Public API

```typescript
// src/index.ts
export { GoogleDriveClient } from './client';
export { GoogleDriveAuth } from './auth';
export { DriveFile, AuthCredentials } from './types';
export { GoogleDriveError } from './errors';

// Usage
import { GoogleDriveClient, GoogleDriveAuth } from '@mygamesanywhere/gdrive-client';

// Step 1: Authenticate
const auth = new GoogleDriveAuth({
  clientId: 'YOUR_CLIENT_ID',
  clientSecret: 'YOUR_CLIENT_SECRET',
  redirectUri: 'http://localhost:3000/callback',
});

const authUrl = auth.getAuthUrl();
// User visits authUrl and authorizes
const tokens = await auth.handleCallback(callbackCode);

// Step 2: Use client
const client = new GoogleDriveClient(tokens.access_token);
const files = await client.listFiles({ folderId: 'folder-id' });
// files = [{ id: 'abc123', name: 'Game.exe', size: 1024000 }, ...]
```

### Types

```typescript
// src/types.ts
export interface DriveFile {
  id: string;
  name: string;
  mimeType: string;
  size: number;          // Bytes
  createdTime: Date;
  modifiedTime: Date;
  parents?: string[];    // Parent folder IDs
}

export interface AuthCredentials {
  access_token: string;
  refresh_token: string;
  expiry_date: number;   // Unix timestamp
}

export interface ListOptions {
  folderId?: string;     // List files in this folder
  query?: string;        // Search query
  pageSize?: number;     // Max results (default 100)
  pageToken?: string;    // For pagination
}

export interface DownloadOptions {
  onProgress?: (progress: DownloadProgress) => void;
}

export interface DownloadProgress {
  bytesDownloaded: number;
  totalBytes: number;
  percentage: number;
}
```

### OAuth Flow

```typescript
// src/auth.ts
export class GoogleDriveAuth {
  constructor(private config: AuthConfig) {}

  /**
   * Get authorization URL for user to visit
   */
  getAuthUrl(): string {
    const params = new URLSearchParams({
      client_id: this.config.clientId,
      redirect_uri: this.config.redirectUri,
      response_type: 'code',
      scope: 'https://www.googleapis.com/auth/drive.readonly',
      access_type: 'offline',
      prompt: 'consent',
    });

    return `https://accounts.google.com/o/oauth2/v2/auth?${params}`;
  }

  /**
   * Exchange authorization code for tokens
   */
  async handleCallback(code: string): Promise<AuthCredentials> {
    const response = await fetch('https://oauth2.googleapis.com/token', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({
        code,
        client_id: this.config.clientId,
        client_secret: this.config.clientSecret,
        redirect_uri: this.config.redirectUri,
        grant_type: 'authorization_code',
      }),
    });

    const data = await response.json();
    return {
      access_token: data.access_token,
      refresh_token: data.refresh_token,
      expiry_date: Date.now() + data.expires_in * 1000,
    };
  }

  /**
   * Refresh access token
   */
  async refreshToken(refreshToken: string): Promise<AuthCredentials> {
    // Implementation
  }
}
```

### Tests

```typescript
// tests/auth.test.ts
describe('GoogleDriveAuth', () => {
  it('should generate valid auth URL', () => {
    const auth = new GoogleDriveAuth({ /* config */ });
    const url = auth.getAuthUrl();

    expect(url).toContain('accounts.google.com');
    expect(url).toContain('scope=https%3A%2F%2Fwww.googleapis.com');
  });

  it('should exchange code for tokens', async () => {
    // Mock fetch
    const auth = new GoogleDriveAuth({ /* config */ });
    const tokens = await auth.handleCallback('test-code');

    expect(tokens.access_token).toBeTruthy();
    expect(tokens.refresh_token).toBeTruthy();
  });
});

// tests/client.test.ts
describe('GoogleDriveClient', () => {
  it('should list files in folder', async () => {
    // Mock API response
    const client = new GoogleDriveClient('test-token');
    const files = await client.listFiles({ folderId: 'test-folder' });

    expect(files).toBeInstanceOf(Array);
    expect(files[0]).toHaveProperty('id');
    expect(files[0]).toHaveProperty('name');
  });

  it('should handle rate limiting', async () => {
    // Mock 429 response
    // Should retry with backoff
  });

  it('should download file', async () => {
    // Test file download
  });
});
```

### Definition of Done
- [ ] OAuth flow works (generate URL, exchange code, get tokens)
- [ ] Can refresh expired tokens
- [ ] Can list files in a folder
- [ ] Can search for files by name
- [ ] Can download files with progress callback
- [ ] Handles rate limiting (429) with retry
- [ ] Handles expired tokens and auto-refreshes
- [ ] Unit tests pass (85%+ coverage, mocked API)
- [ ] Type-safe API
- [ ] README with OAuth setup guide

---

## Package 3: IGDB Metadata Client

### Purpose
Search for games and fetch metadata from IGDB API.

### Features
- Twitch OAuth authentication (required for IGDB)
- Search games by title
- Fetch detailed game metadata
- Fetch media URLs (covers, screenshots)
- Rate limiting (4 requests/second)
- Token auto-refresh
- Error handling

### Public API

```typescript
// src/index.ts
export { IGDBClient } from './client';
export { Game, SearchOptions, SearchResult } from './types';
export { IGDBError } from './errors';

// Usage
import { IGDBClient } from '@mygamesanywhere/igdb-client';

const client = new IGDBClient({
  clientId: 'YOUR_TWITCH_CLIENT_ID',
  clientSecret: 'YOUR_TWITCH_CLIENT_SECRET',
});

await client.initialize();  // Gets access token

const results = await client.search('Half-Life 2');
// results = [{ id: 123, name: 'Half-Life 2', ... }, ...]

const game = await client.getGame(123);
// game = { id: 123, name: 'Half-Life 2', summary: '...', cover: {...}, ... }
```

### Types

```typescript
// src/types.ts
export interface Game {
  id: number;
  name: string;
  summary?: string;
  storyline?: string;
  releaseDate?: Date;
  rating?: number;         // 0-100
  genres?: Genre[];
  platforms?: Platform[];
  developers?: Company[];
  publishers?: Company[];
  cover?: Image;
  screenshots?: Image[];
  videos?: Video[];
}

export interface Image {
  id: string;
  url: string;
  width: number;
  height: number;
}

export interface Genre {
  id: number;
  name: string;
}

export interface Platform {
  id: number;
  name: string;
}

export interface Company {
  id: number;
  name: string;
}

export interface SearchOptions {
  limit?: number;          // Default 10
  offset?: number;
  platforms?: number[];    // Platform IDs to filter
}
```

### Rate Limiter

```typescript
// src/rate-limiter.ts
export class RateLimiter {
  private queue: Array<() => Promise<any>> = [];
  private processing = false;
  private lastRequestTime = 0;
  private minInterval = 250; // 4 requests/second = 250ms between requests

  async execute<T>(fn: () => Promise<T>): Promise<T> {
    return new Promise((resolve, reject) => {
      this.queue.push(async () => {
        try {
          const result = await fn();
          resolve(result);
        } catch (error) {
          reject(error);
        }
      });

      this.processQueue();
    });
  }

  private async processQueue() {
    if (this.processing || this.queue.length === 0) return;

    this.processing = true;

    while (this.queue.length > 0) {
      // Wait for rate limit
      const now = Date.now();
      const timeSinceLastRequest = now - this.lastRequestTime;
      if (timeSinceLastRequest < this.minInterval) {
        await this.sleep(this.minInterval - timeSinceLastRequest);
      }

      const task = this.queue.shift();
      if (task) {
        this.lastRequestTime = Date.now();
        await task();
      }
    }

    this.processing = false;
  }

  private sleep(ms: number): Promise<void> {
    return new Promise(resolve => setTimeout(resolve, ms));
  }
}
```

### Tests

```typescript
// tests/rate-limiter.test.ts
describe('RateLimiter', () => {
  it('should limit requests to 4 per second', async () => {
    const limiter = new RateLimiter();
    const start = Date.now();

    // Execute 8 requests
    const promises = Array(8).fill(0).map(() =>
      limiter.execute(async () => 'done')
    );

    await Promise.all(promises);
    const duration = Date.now() - start;

    // Should take at least 2 seconds (8 requests / 4 per second)
    expect(duration).toBeGreaterThan(1900);
  });
});

// tests/client.test.ts
describe('IGDBClient', () => {
  it('should authenticate with Twitch OAuth', async () => {
    const client = new IGDBClient({ /* config */ });
    await client.initialize();

    expect(client.isAuthenticated()).toBe(true);
  });

  it('should search for games', async () => {
    const client = new IGDBClient({ /* config */ });
    await client.initialize();

    const results = await client.search('Half-Life');
    expect(results).toBeInstanceOf(Array);
    expect(results[0]).toHaveProperty('id');
    expect(results[0]).toHaveProperty('name');
  });

  it('should fetch game details', async () => {
    const client = new IGDBClient({ /* config */ });
    await client.initialize();

    const game = await client.getGame(123);
    expect(game).toHaveProperty('name');
    expect(game).toHaveProperty('summary');
  });

  it('should respect rate limits', async () => {
    // Test that requests are throttled
  });

  it('should refresh token when expired', async () => {
    // Mock token expiry and test auto-refresh
  });
});
```

### Definition of Done
- [ ] Can authenticate with Twitch OAuth
- [ ] Can search games by title
- [ ] Can fetch detailed game metadata
- [ ] Can fetch media URLs
- [ ] Rate limiter works (max 4 req/sec)
- [ ] Auto-refreshes expired tokens
- [ ] Handles API errors gracefully
- [ ] Unit tests pass (85%+ coverage, mocked API)
- [ ] Type-safe API
- [ ] README with IGDB setup guide

---

## Package 4: Native Launcher

### Purpose
Launch native game executables and monitor running processes.

### Features
- Detect platform (Windows, macOS, Linux)
- Find game executable in install directory
- Launch game process
- Monitor process (running/stopped)
- Track playtime
- Handle process exit
- Kill running processes

### Public API

```typescript
// src/index.ts
export { NativeLauncher } from './launcher';
export { ProcessMonitor } from './monitor';
export { LaunchOptions, Process, ProcessStatus } from './types';
export { LauncherError } from './errors';

// Usage
import { NativeLauncher } from '@mygamesanywhere/native-launcher';

const launcher = new NativeLauncher();

const process = await launcher.launch({
  executable: 'C:\\Games\\MyGame\\game.exe',
  args: ['--fullscreen'],
  cwd: 'C:\\Games\\MyGame',
});

// process.id = unique process ID
// process.pid = OS process ID

const status = await launcher.getStatus(process.id);
// status.isRunning = true
// status.playtime = 123 (seconds)

await launcher.stop(process.id);
```

### Types

```typescript
// src/types.ts
export interface LaunchOptions {
  executable: string;              // Full path to .exe, .app, or binary
  args?: string[];                 // Command-line arguments
  cwd?: string;                    // Working directory
  env?: Record<string, string>;    // Environment variables
  detached?: boolean;              // Run detached from parent
}

export interface Process {
  id: string;                      // Our unique ID
  pid: number;                     // OS process ID
  executable: string;
  startTime: Date;
}

export interface ProcessStatus {
  id: string;
  isRunning: boolean;
  startTime: Date;
  endTime?: Date;
  playtime: number;                // Seconds
  exitCode?: number;
}

export enum Platform {
  Windows = 'windows',
  MacOS = 'macos',
  Linux = 'linux',
}
```

### Platform Detection

```typescript
// src/platform.ts
export function detectPlatform(): Platform {
  switch (process.platform) {
    case 'win32':
      return Platform.Windows;
    case 'darwin':
      return Platform.MacOS;
    case 'linux':
      return Platform.Linux;
    default:
      throw new Error(`Unsupported platform: ${process.platform}`);
  }
}

export function findExecutable(directory: string, platform: Platform): string | null {
  // Windows: Find .exe files
  // macOS: Find .app bundles
  // Linux: Find executables (chmod +x)
}
```

### Process Launcher

```typescript
// src/launcher.ts
import { spawn, ChildProcess } from 'child_process';

export class NativeLauncher {
  private processes = new Map<string, ChildProcess>();
  private startTimes = new Map<string, Date>();

  async launch(options: LaunchOptions): Promise<Process> {
    // Validate executable exists
    if (!await this.fileExists(options.executable)) {
      throw new LauncherError(
        'EXECUTABLE_NOT_FOUND',
        `Executable not found: ${options.executable}`
      );
    }

    // Spawn process
    const childProcess = spawn(options.executable, options.args || [], {
      cwd: options.cwd || path.dirname(options.executable),
      env: { ...process.env, ...options.env },
      detached: options.detached ?? true,
    });

    const id = this.generateId();
    const startTime = new Date();

    this.processes.set(id, childProcess);
    this.startTimes.set(id, startTime);

    // Handle process exit
    childProcess.on('exit', (code) => {
      console.log(`Process ${id} exited with code ${code}`);
    });

    return {
      id,
      pid: childProcess.pid!,
      executable: options.executable,
      startTime,
    };
  }

  async getStatus(id: string): Promise<ProcessStatus> {
    const process = this.processes.get(id);
    const startTime = this.startTimes.get(id);

    if (!startTime) {
      throw new LauncherError('PROCESS_NOT_FOUND', `Process ${id} not found`);
    }

    const now = new Date();
    const playtime = Math.floor((now.getTime() - startTime.getTime()) / 1000);

    if (!process || process.killed || process.exitCode !== null) {
      return {
        id,
        isRunning: false,
        startTime,
        endTime: new Date(),
        playtime,
        exitCode: process?.exitCode ?? undefined,
      };
    }

    return {
      id,
      isRunning: true,
      startTime,
      playtime,
    };
  }

  async stop(id: string): Promise<void> {
    const process = this.processes.get(id);
    if (!process) {
      throw new LauncherError('PROCESS_NOT_FOUND', `Process ${id} not found`);
    }

    process.kill();
    this.processes.delete(id);
  }

  private generateId(): string {
    return `process-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
  }

  private async fileExists(path: string): Promise<boolean> {
    try {
      await fs.access(path);
      return true;
    } catch {
      return false;
    }
  }
}
```

### Tests

```typescript
// tests/launcher.test.ts
describe('NativeLauncher', () => {
  it('should launch executable', async () => {
    const launcher = new NativeLauncher();

    // Use a simple command like 'notepad.exe' on Windows
    const process = await launcher.launch({
      executable: 'notepad.exe',
    });

    expect(process.id).toBeTruthy();
    expect(process.pid).toBeGreaterThan(0);

    const status = await launcher.getStatus(process.id);
    expect(status.isRunning).toBe(true);

    await launcher.stop(process.id);
  });

  it('should track playtime', async () => {
    const launcher = new NativeLauncher();

    const process = await launcher.launch({
      executable: 'notepad.exe',
    });

    // Wait 2 seconds
    await new Promise(resolve => setTimeout(resolve, 2000));

    const status = await launcher.getStatus(process.id);
    expect(status.playtime).toBeGreaterThanOrEqual(2);

    await launcher.stop(process.id);
  });

  it('should handle non-existent executable', async () => {
    const launcher = new NativeLauncher();

    await expect(launcher.launch({
      executable: '/nonexistent/game.exe',
    })).rejects.toThrow('EXECUTABLE_NOT_FOUND');
  });

  it('should detect process exit', async () => {
    // Launch short-lived process
    // Check that status.isRunning becomes false
  });
});

// tests/platform.test.ts
describe('Platform Detection', () => {
  it('should detect current platform', () => {
    const platform = detectPlatform();
    expect(Object.values(Platform)).toContain(platform);
  });

  it('should find Windows executables', () => {
    // Test finding .exe in directory
  });
});
```

### Definition of Done
- [ ] Can detect platform (Windows, macOS, Linux)
- [ ] Can find executable in install directory
- [ ] Can launch executable with arguments
- [ ] Can monitor process status
- [ ] Can track playtime accurately
- [ ] Can stop running process
- [ ] Handles missing executable gracefully
- [ ] Handles process crashes
- [ ] Unit tests pass (80%+ coverage)
- [ ] Type-safe API
- [ ] README with usage examples

---

## Week-by-Week Breakdown

### Week 1: Project Setup + Steam Scanner

**Monday-Tuesday:**
- Set up workspace structure
- Configure TypeScript, ESLint, Prettier
- Set up Vitest
- Create package scaffolding
- Write README templates

**Wednesday-Friday:**
- Implement VDF parser
- Write VDF parser tests
- Implement Steam path detection
- Implement libraryfolders.vdf parsing
- Implement .acf file scanning

**Weekend:**
- Complete Steam scanner implementation
- Write comprehensive tests
- Test with real Steam installation

**Deliverable:** Working Steam scanner with tests

---

### Week 2: Steam Scanner Polish + Google Drive Start

**Monday-Tuesday:**
- Fix Steam scanner bugs
- Add error handling
- Improve test coverage
- Write Steam scanner README

**Wednesday-Friday:**
- Set up Google Drive OAuth
- Implement auth flow (generate URL, exchange code)
- Implement token refresh
- Write auth tests

**Weekend:**
- Test OAuth flow end-to-end
- Document OAuth setup process

**Deliverable:** Steam scanner complete, Google Drive auth working

---

### Week 3: Google Drive Client

**Monday-Wednesday:**
- Implement file listing
- Implement file search
- Implement pagination
- Write client tests

**Thursday-Friday:**
- Implement file download
- Implement progress tracking
- Handle rate limiting
- Handle errors

**Weekend:**
- Test with real Google Drive account
- Polish and document

**Deliverable:** Complete Google Drive client with tests

---

### Week 4: IGDB Client

**Monday-Tuesday:**
- Set up Twitch OAuth for IGDB
- Implement token acquisition
- Implement token refresh
- Write auth tests

**Wednesday-Thursday:**
- Implement game search
- Implement game details fetch
- Implement media URL fetch
- Write client tests

**Friday:**
- Implement rate limiter
- Test rate limiting
- Handle API errors

**Weekend:**
- Test with real IGDB account
- Polish and document

**Deliverable:** Complete IGDB client with tests

---

### Week 5: Native Launcher

**Monday-Tuesday:**
- Implement platform detection
- Implement executable finder
- Write platform tests

**Wednesday-Thursday:**
- Implement process launcher
- Implement process monitoring
- Implement playtime tracking
- Write launcher tests

**Friday:**
- Implement process stop
- Handle edge cases
- Error handling

**Weekend:**
- Test on Windows (primary)
- Test on macOS/Linux if available
- Document platform differences

**Deliverable:** Complete Native Launcher with tests

---

### Week 6: Integration & Documentation

**Monday-Tuesday:**
- Fix any remaining bugs
- Improve error messages
- Add JSDoc comments
- Type cleanup

**Wednesday-Thursday:**
- Write integration examples
- Create example scripts showing packages working together
- Update all READMEs
- Write main workspace README

**Friday:**
- Final testing
- Measure test coverage
- Create demo videos/screenshots
- Prepare presentation

**Weekend:**
- Buffer time for any issues
- Code review

**Deliverable:** All 4 packages complete, tested, documented

---

## Testing Strategy

### Unit Tests (Primary Focus)

Each package has comprehensive unit tests:
- **Steam Scanner:** Test VDF parsing, file reading, path detection
- **Google Drive:** Mock OAuth flow, mock API responses
- **IGDB:** Mock OAuth, mock API, test rate limiter
- **Native Launcher:** Test process spawning, monitoring, platform detection

**Coverage Goals:**
- Overall: 85%+
- Critical paths: 95%+
- Error handling: 100%

### Integration Tests (Manual)

Since these are integration libraries, manual testing with real services is essential:
- Test Steam scanner with real Steam installation
- Test Google Drive with real Google account
- Test IGDB with real IGDB API credentials
- Test Native Launcher with real game executables

**Create test accounts:**
- Google account for testing
- IGDB developer account
- Document setup process

### Test Fixtures

Provide sample data for offline testing:
- Sample .acf files (Steam)
- Sample libraryfolders.vdf
- Sample API responses (Google Drive, IGDB)
- Simple test executables

---

## Development Environment

### Required

- **Node.js:** 18 LTS or higher
- **npm:** 9+ or pnpm 8+
- **TypeScript:** 5.x
- **VS Code** (recommended IDE)

### Optional

- **Steam:** For testing Steam scanner
- **Google Account:** For testing Google Drive
- **IGDB Account:** For testing IGDB client

### VS Code Extensions

- ESLint
- Prettier
- Vitest
- TypeScript

---

## Success Criteria

### Technical

- [ ] All 4 packages build without errors
- [ ] All tests pass
- [ ] Test coverage ≥ 85%
- [ ] No TypeScript errors (`tsc --noEmit`)
- [ ] Linting passes
- [ ] Each package has clear API
- [ ] Each package has README with examples

### Functional

- [ ] Steam scanner finds installed games on real Steam installation
- [ ] Google Drive client authenticates and lists files
- [ ] IGDB client searches and fetches game metadata
- [ ] Native Launcher launches and monitors processes
- [ ] All error cases handled gracefully
- [ ] Rate limiting works (IGDB, Google Drive)

### Documentation

- [ ] Each package has README
- [ ] API documented with JSDoc
- [ ] Setup guides for external services
- [ ] Example usage code
- [ ] Main workspace README

---

## Risks & Mitigation

### Risk: Steam VDF Parsing

**Challenge:** VDF format is not well-documented, may have edge cases

**Mitigation:**
- Collect many sample .acf files
- Test with different Steam versions
- Implement robust error handling
- Log parsing failures for analysis

### Risk: Google Drive OAuth

**Challenge:** OAuth flow is complex, easy to get wrong

**Mitigation:**
- Follow official Google documentation
- Test with multiple accounts
- Handle token expiry properly
- Document setup process clearly

### Risk: IGDB Rate Limits

**Challenge:** 4 requests/second limit is strict

**Mitigation:**
- Implement reliable rate limiter
- Test rate limiter thoroughly
- Add retry logic with backoff
- Cache responses when possible

### Risk: Cross-Platform Launcher

**Challenge:** Process spawning differs between platforms

**Mitigation:**
- Focus on Windows first (user's platform)
- Use Node.js child_process (cross-platform)
- Test on multiple platforms if possible
- Document platform-specific behavior

---

## Next Steps After Phase 1

Once all integration libraries are working:

1. **Phase 2:** Build plugin system wrapper around these libraries
2. **Phase 3:** Build minimal server + client with auth
3. **Phase 4:** Build full UI using plugins

These libraries will become the foundation of the plugin system, so getting them right is critical!

---

## Summary

**Deliverables:**
- 4 standalone TypeScript packages with unit tests
- Proven integrations before building UI/server
- Reusable libraries for future plugin system

**Timeline:** 5-6 weeks

**Outcome:** Confidence that the hardest parts (integrations) work before committing to full architecture!
