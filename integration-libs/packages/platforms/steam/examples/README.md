# Steam Scanner Examples

This directory contains practical examples demonstrating how to use `@mygamesanywhere/steam-scanner`.

## Running the Examples

All examples are written in TypeScript. To run them:

```bash
# Install dependencies first
npm install

# Build the package
npm run build

# Run an example with tsx or ts-node
npx tsx examples/basic-usage.ts

# Or compile and run with Node.js
npx tsc examples/basic-usage.ts --module commonjs --target es2020
node examples/basic-usage.js
```

## Available Examples

### 1. `basic-usage.ts`

**What it demonstrates:**
- Simplest way to scan your Steam library
- How to access scan results
- Display game information (name, size, path)

**Best for:** Getting started quickly

**Key concepts:**
```typescript
import { scanSteamLibrary } from '@mygamesanywhere/steam-scanner';

const result = await scanSteamLibrary();
console.log(`Found ${result.games.length} games`);
```

---

### 2. `error-handling.ts`

**What it demonstrates:**
- Proper error handling with try-catch
- Different error types (SteamNotFoundError, FileAccessError, etc.)
- Error handling by error code
- Graceful degradation

**Best for:** Production applications that need robust error handling

**Key concepts:**
```typescript
try {
  const result = await scanSteamLibrary();
} catch (error) {
  if (error instanceof SteamNotFoundError) {
    console.error('Steam not installed');
  }
}
```

---

### 3. `advanced-filtering.ts`

**What it demonstrates:**
- Finding largest games
- Calculating total library size
- Grouping games by library folder
- Finding recently updated games
- Filtering games by size range
- Checking if specific games are installed

**Best for:** Building library analysis tools

**Key concepts:**
```typescript
// Find largest games
const sortedBySize = [...result.games].sort((a, b) =>
  parseInt(b.sizeOnDisk) - parseInt(a.sizeOnDisk)
);

// Calculate total size
const totalSize = result.games.reduce((sum, game) =>
  sum + parseInt(game.sizeOnDisk), 0
);

// Group by library
const gamesByLibrary = new Map();
result.games.forEach(game => {
  if (!gamesByLibrary.has(game.libraryPath)) {
    gamesByLibrary.set(game.libraryPath, []);
  }
  gamesByLibrary.get(game.libraryPath).push(game);
});
```

---

### 4. `custom-path-and-vdf.ts`

**What it demonstrates:**
- Using custom Steam installation paths
- Manual Steam path detection
- Low-level VDF parser usage
- Parsing libraryfolders.vdf directly
- Parsing game manifest files
- Parsing custom VDF content

**Best for:** Advanced use cases, debugging, or custom Steam installations

**Key concepts:**
```typescript
// Custom path
const scanner = new SteamScanner();
await scanner.initialize({
  steamPath: 'D:\\Custom\\Steam'
});

// VDF parser
import { parseVDF } from '@mygamesanywhere/steam-scanner';
const parsed = parseVDF(vdfContent);
```

---

## Common Patterns

### Check if a specific game is installed

```typescript
const result = await scanSteamLibrary();
const hasTF2 = result.games.some(game => game.appid === '440');
```

### Find games larger than X GB

```typescript
const largeGames = result.games.filter(game => {
  const sizeGB = parseInt(game.sizeOnDisk) / (1024 ** 3);
  return sizeGB > 10; // Games larger than 10 GB
});
```

### Get total library size

```typescript
const totalBytes = result.games.reduce(
  (sum, game) => sum + parseInt(game.sizeOnDisk),
  0
);
const totalGB = totalBytes / (1024 ** 3);
```

### Find recently updated games

```typescript
const recentGames = [...result.games]
  .sort((a, b) => b.lastUpdated - a.lastUpdated)
  .slice(0, 10); // Top 10 most recent
```

## Next Steps

After exploring these examples:

1. Read the main [README.md](../README.md) for full API documentation
2. Check out the [tests](../tests/) for more usage examples
3. Explore the [source code](../src/) to understand implementation details
4. Build your own Steam library tools!

## Need Help?

- Open an issue on GitHub
- Check the test files for more examples
- Read the TypeScript definitions for type information

## Contributing

Have a useful example? Feel free to contribute by opening a pull request!
