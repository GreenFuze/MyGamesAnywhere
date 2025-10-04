# Guide for Cursor AI / AI Assistants

**Last Updated:** 2025-10-04
**Purpose:** Critical context for AI assistants continuing this project

---

## 🚨 CRITICAL CONCEPTS - Read First!

### 1. Multi-Source Games (Most Important!)
**The entire plugin system is built around this concept:**

- **Same game can exist in multiple sources** (Steam, Xbox, local files, Google Drive)
- **Plugin system automatically merges them into ONE unified game**
- **Each source maintains its own data** (installed status, playtime, etc.)

**Example:**
```typescript
// User owns "God of War" on Steam AND has a local copy AND has it on Google Drive
// Plugin system creates ONE UnifiedGame with 3 sources:

{
  id: "unified-abc123",
  title: "God of War",  // From best identifier
  sources: [
    {
      sourceId: "steam-source",
      gameId: "steam-local-582010",
      installed: true,
      playtime: 4200  // 70 hours on Steam
    },
    {
      sourceId: "epic-source",
      gameId: "epic-godofwar",
      installed: false,
      playtime: 0
    },
    {
      sourceId: "custom-storefront-source",
      gameId: "custom-godofwar.zip",
      installed: false  // It's in Google Drive
    }
  ],
  identifications: [
    { identifierId: "launchbox", confidence: 0.90 },
    { identifierId: "igdb", confidence: 0.95 }  // Future
  ],
  isInstalled: true,      // TRUE if installed in ANY source
  totalPlaytime: 4200,    // Sum from all sources
  lastPlayed: Date        // Most recent from all sources
}
```

**UI Implication:**
- Show ONE card for "God of War"
- Display badges for each source (Steam icon + Epic icon + Local icon)
- User clicks → choose which source to launch from

### 2. Plugin Package Linking
**Desktop app uses `file:` references to local packages:**

```json
// desktop-app/package.json
"dependencies": {
  "@mygamesanywhere/plugin-system": "file:../integration-libs/packages/core/plugin-system",
  "@mygamesanywhere/plugin-steam-source": "file:../integration-libs/packages/plugins/steam-source"
}
```

**This means:**
- Changes to plugins → automatically reflected in desktop app
- Must run `npm install` in desktop-app after plugin changes
- Build plugins BEFORE building desktop app

### 3. Two Different DetectedGame Types
**GOTCHA:** There are TWO different `DetectedGame` types!

1. **`generic-repository` DetectedGame** (internal scanner format)
   - Has: `type`, `location`, `confidence`, `detectedAt`
   - Used by repository scanners

2. **`plugin-system` DetectedGame** (plugin interface format)
   - Has: `sourceId`, `id`, `name`, `path`, `installed`, `platform`
   - Used by plugins

**Solution:** Plugins convert between them (see `custom-storefront-source/src/index.ts:144`)

### 4. LaunchBox Database Size
**450MB download on first run!**
- Downloads Metadata.zip from LaunchBox
- Extracts and parses 450MB of XML
- Creates SQLite database with 126,000 games
- Stored in `~/.mygamesanywhere/metadata/launchbox/`
- **Only downloads once**, then cached

### 5. Electron + Vite Configuration
**Special setup required:**

- Vite needs `vite-plugin-electron` AND `vite-plugin-electron-renderer`
- Main process in `electron/main.ts` (Node.js environment)
- Renderer process in `src/` (browser environment)
- Preload script bridges the two
- **MUST** set `nodeIntegration: true` for plugin system to work (uses Node.js APIs)

---

## 🏗️ Architecture Patterns

### Plugin Pattern
```typescript
// 1. Create plugin class implementing interface
export class MySourcePlugin implements SourcePlugin {
  readonly type = PluginType.SOURCE;
  readonly metadata = { id: "my-source", name: "My Source", ... };

  async initialize(config) { /* setup */ }
  async scan(): Promise<DetectedGame[]> { /* scan logic */ }
}

// 2. Register plugin
const plugin = new MySourcePlugin();
await plugin.initialize(config);
pluginRegistry.register(plugin);

// 3. Use through registry
const sources = pluginRegistry.getAllSources();
for (const source of sources) {
  const games = await source.scan();
  // Add to UnifiedGameManager
}
```

### Unified Game Manager Pattern
```typescript
const manager = new UnifiedGameManager();
manager.setMatchStrategy(MatchStrategy.FUZZY_TITLE, 0.85);

// Add detected games
for (const game of detectedGames) {
  const gameSource: GameSource = {
    sourceId: "steam-source",
    gameId: game.id,
    detectedGame: game,
    installed: game.installed
  };

  // Automatically merges if same game exists
  const unified = manager.addDetectedGame(gameSource);
}

// Get all unified games
const allGames = manager.getAllGames();
```

### State Management Pattern (Zustand)
```typescript
// store/games.ts
export const useGamesStore = create<GamesState>((set) => ({
  games: [],
  loading: false,

  setGames: (games) => set({ games }),
  setLoading: (loading) => set({ loading }),
}));

// Usage in component
const { games, loading, setGames } = useGamesStore();
```

---

## 🚫 Common Pitfalls

### 1. Type Import Errors
**WRONG:**
```typescript
import { DetectedGame } from '@mygamesanywhere/generic-repository';
import { DetectedGame } from '@mygamesanywhere/plugin-system';
// ERROR: Duplicate identifier 'DetectedGame'
```

**RIGHT:**
```typescript
import type { DetectedGame as RepoGame } from '@mygamesanywhere/generic-repository';
import type { DetectedGame } from '@mygamesanywhere/plugin-system';
```

### 2. Forgetting to Build Plugins
**Before running desktop app:**
```bash
cd integration-libs
npm run build  # MUST build plugins first!

cd ../desktop-app
npm install    # Link updated plugins
npm run dev
```

### 3. Async Plugin Initialization
**WRONG:**
```typescript
const plugin = new SteamSourcePlugin();
pluginRegistry.register(plugin);  // Not initialized yet!
const games = await plugin.scan(); // May fail
```

**RIGHT:**
```typescript
const plugin = new SteamSourcePlugin();
await plugin.initialize(config);  // Wait for init
pluginRegistry.register(plugin);
const games = await plugin.scan();
```

### 4. OAuth in Electron
**Browser OAuth flow needs special handling:**

```typescript
// Don't open OAuth in Electron window
// Use system browser instead:
import { shell } from 'electron';

async function startOAuth() {
  const authUrl = "https://accounts.google.com/...";

  // Open in system browser
  await shell.openExternal(authUrl);

  // Listen for redirect
  // Option 1: Custom protocol (steam://callback)
  // Option 2: Local server on localhost:3000
}
```

### 5. File Paths in Electron
**WRONG:**
```typescript
const dbPath = './launchbox.db';  // Relative to what?
```

**RIGHT:**
```typescript
import { app } from 'electron';
import path from 'path';

const userDataPath = app.getPath('userData');
const dbPath = path.join(userDataPath, 'launchbox.db');
// e.g., C:\Users\name\AppData\Roaming\mygamesanywhere\launchbox.db
```

---

## 🎨 UI Design Guidelines

### Color Palette (Gamer Theme)
```css
/* Dark backgrounds */
--dark-900: #0a0a0f;  /* Darkest - main bg */
--dark-800: #14141f;  /* Cards, containers */
--dark-700: #1e1e2f;  /* Hover states */
--dark-600: #28283f;  /* Borders */

/* Neon accents */
--neon-blue: #00d4ff;    /* Primary actions */
--neon-purple: #bf00ff;  /* Secondary */
--neon-pink: #ff00bf;    /* Highlights */
--neon-green: #00ff88;   /* Success states */
```

### Component Patterns
```tsx
// Card with hover effect
<motion.div
  whileHover={{ scale: 1.05, boxShadow: "0 0 20px rgba(0, 212, 255, 0.5)" }}
  className="bg-dark-800 rounded-lg p-4 cursor-pointer"
>
  {/* Game content */}
</motion.div>

// Multi-source badge
<div className="flex gap-1">
  {game.sources.map(source => (
    <Badge
      key={source.sourceId}
      icon={getSourceIcon(source.sourceId)}
      color={getSourceColor(source.sourceId)}
    />
  ))}
</div>

// Progress indicator
<motion.div
  className="h-2 bg-neon-blue rounded-full"
  initial={{ width: 0 }}
  animate={{ width: `${progress}%` }}
  transition={{ duration: 0.5 }}
/>
```

### Animation Patterns (Framer Motion)
```tsx
// Page transitions
<motion.div
  initial={{ opacity: 0, x: -20 }}
  animate={{ opacity: 1, x: 0 }}
  exit={{ opacity: 0, x: 20 }}
  transition={{ duration: 0.3 }}
>
  {/* Page content */}
</motion.div>

// Staggered list
<motion.div variants={containerVariants}>
  {games.map((game, i) => (
    <motion.div
      key={game.id}
      variants={itemVariants}
      custom={i}
    >
      <GameCard game={game} />
    </motion.div>
  ))}
</motion.div>

const containerVariants = {
  hidden: { opacity: 0 },
  show: {
    opacity: 1,
    transition: { staggerChildren: 0.1 }
  }
};

const itemVariants = {
  hidden: { opacity: 0, y: 20 },
  show: { opacity: 1, y: 0 }
};
```

---

## 📝 Code Conventions

### File Naming
- Components: `PascalCase.tsx` (e.g., `GameCard.tsx`)
- Hooks: `use*.ts` (e.g., `usePlugins.ts`)
- Stores: `*.ts` (e.g., `games.ts`, `setup.ts`)
- Services: `*Service.ts` (e.g., `PluginService.ts`)
- Types: `types.ts` or `*.types.ts`

### Import Order
```typescript
// 1. React/external
import { useState } from 'react';
import { motion } from 'framer-motion';

// 2. Internal packages
import { pluginRegistry } from '@mygamesanywhere/plugin-system';

// 3. Local components
import GameCard from '../components/GameCard';

// 4. Local utilities
import { formatPlaytime } from '../utils';

// 5. Types
import type { UnifiedGame } from '@mygamesanywhere/plugin-system';

// 6. Styles
import './GameLibrary.css';
```

### Error Handling
```typescript
// Service layer - throw errors
async scanGames() {
  if (!this.initialized) {
    throw new Error('PluginService not initialized');
  }
  // ...
}

// UI layer - catch and display
try {
  await pluginService.scanGames();
} catch (error) {
  setError(error instanceof Error ? error.message : 'Unknown error');
  toast.error('Failed to scan games');
}
```

---

## 🧪 Testing Approach

### Unit Tests (Future)
```typescript
// Test plugin isolation
describe('SteamSourcePlugin', () => {
  it('should scan local library', async () => {
    const plugin = new SteamSourcePlugin();
    await plugin.initialize({ scanLocal: true });
    const games = await plugin.scan();
    expect(games.length).toBeGreaterThan(0);
  });
});
```

### Manual Testing Checklist
```markdown
Setup Wizard:
- [ ] Steam auto-detection works
- [ ] Google Drive OAuth completes
- [ ] Custom folders can be added
- [ ] Scan shows progress
- [ ] Complete screen shows game count

Game Library:
- [ ] All games displayed
- [ ] Multi-source badges show
- [ ] Search filters games
- [ ] Sort works (name, playtime)
- [ ] Click launches game
- [ ] Cover art loads (or placeholder)

Performance:
- [ ] Library loads in < 2s
- [ ] Smooth animations (60fps)
- [ ] No memory leaks
```

---

## 🔍 Debugging Tips

### Enable Electron DevTools
```typescript
// electron/main.ts
if (process.env.NODE_ENV === 'development') {
  mainWindow.webContents.openDevTools();
}
```

### Plugin Debugging
```typescript
// Add to PluginService
async scanAllSources(onProgress?: (msg: string) => void) {
  console.log('[PluginService] Starting scan...');

  for (const source of sources) {
    console.log(`[PluginService] Scanning ${source.metadata.name}`);
    const games = await source.scan();
    console.log(`[PluginService] Found ${games.length} games`);
    // ...
  }
}
```

### State Debugging (Zustand DevTools)
```typescript
import { devtools } from 'zustand/middleware';

export const useGamesStore = create<GamesState>()(
  devtools(
    (set) => ({ /* state */ }),
    { name: 'GamesStore' }  // Shows in Redux DevTools
  )
);
```

---

## 📚 Key Files Reference

### Must Read Before Coding
1. `NEXT_STEPS.md` - Step-by-step with code
2. `integration-libs/PLUGIN-SYSTEM.md` - Plugin architecture
3. `integration-libs/demo-plugin-system.ts` - Working example

### Core Plugin Files
- `packages/core/plugin-system/src/types.ts` - All interfaces
- `packages/core/plugin-system/src/unified-game.ts` - Multi-source logic
- `packages/plugins/*/src/index.ts` - Plugin implementations

### Config Files
- `desktop-app/package.json` - Dependencies
- `desktop-app/vite.config.ts` - Build config (needs Electron plugins)
- `desktop-app/tailwind.config.js` - Not created yet!

---

## ⚡ Quick Commands

```bash
# Build everything
cd integration-libs && npm run build
cd ../desktop-app && npm install && npm run dev

# Test plugins
cd integration-libs && npm run demo:plugins

# Check what exists
ls desktop-app/src/
ls desktop-app/electron/  # Should not exist yet

# Create missing dirs
mkdir -p desktop-app/electron
mkdir -p desktop-app/src/{pages,store,services,components}
```

---

## 🎯 Priority Tasks for Cursor

### HIGH PRIORITY (Do First)
1. **Create `tailwind.config.js`** with gamer theme (copy from NEXT_STEPS.md)
2. **Create `electron/main.ts`** - Main process (copy from NEXT_STEPS.md)
3. **Update `vite.config.ts`** - Add Electron plugins (copy from NEXT_STEPS.md)
4. **Test:** Run `npm run dev` - Electron window should open

### MEDIUM PRIORITY (Then)
5. **Create Zustand stores** - `store/setup.ts` and `store/games.ts`
6. **Create `services/PluginService.ts`** - Bridge plugins to UI
7. **Build Setup Wizard** - 6 screens (Welcome → Complete)

### LOW PRIORITY (Finally)
8. **Build Game Library** - Grid view with multi-source badges
9. **Polish animations** - Framer Motion effects
10. **Test end-to-end** - Full user flow

---

## 💡 Success Tips

1. **Start with Electron config** - Get app running first, then build UI
2. **Copy from NEXT_STEPS.md** - Don't recreate, all code is there
3. **Test incrementally** - Each screen individually
4. **Use demo script** - `demo-plugin-system.ts` shows plugin usage
5. **Check CURRENT_STATUS.md** - For latest project state

---

## ⚠️ Known Limitations

1. **LaunchBox accuracy: 55%** - Some games won't identify
   - Not in database (new games)
   - Too generic names ("GO.BAT")
   - Future: Add IGDB as fallback

2. **Google Drive scanning is slow** - 5 min for 763 files
   - API rate limits
   - Future: Batch requests, cache metadata

3. **No save game sync yet** - Phase 4 feature
4. **No achievements tracking** - Phase 4 feature
5. **No playtime tracking** - Phase 4 feature

---

## 🚀 Ready to Code!

**Start here:** `NEXT_STEPS.md` Step 1 - Initialize Tailwind CSS

**Remember:**
- Multi-source games = core concept
- Build plugins before desktop app
- Use working demo as reference
- All code is in NEXT_STEPS.md

**You got this! 🎮**
