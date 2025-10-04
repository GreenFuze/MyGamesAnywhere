# Next Steps - Phase 3 Continuation

**Created:** 2025-10-04
**Status:** Phase 3 Started - Desktop UI in progress
**Resume Point:** Desktop app initialized, need to complete Electron setup and build UI

---

## Current State

### ✅ What's Complete

**Phase 1: Core Integrations (100%)**
- All platform clients (Steam, Google Drive, LaunchBox, IGDB placeholder)
- Generic repository scanner (202 games detected from 763 files)
- Game identifier with LaunchBox (55% accuracy, 126k games database)
- Centralized config system

**Phase 2: Plugin System (100%)**
- Plugin architecture with multi-source support
- 3 plugins implemented and working:
  - `@mygamesanywhere/plugin-steam-source`
  - `@mygamesanywhere/plugin-custom-storefront-source`
  - `@mygamesanywhere/plugin-launchbox-identifier`
- Unified game model (same game across Steam/Xbox/local = 1 entry)
- Demo script: `integration-libs/demo-plugin-system.ts`

**Phase 3: Desktop UI (20% - In Progress)**
- ✅ Created `desktop-app/` directory
- ✅ Initialized Vite + React + TypeScript
- ✅ Installed dependencies:
  - Electron 38.2.1
  - React 19.1.1
  - React Router 7.9.3
  - Tailwind CSS 4.1.14
  - Framer Motion 12.23.22
  - Zustand 5.0.8
  - Lucide React 0.544.0
- ✅ Linked all plugin packages (file: references in package.json)
- ❌ NOT YET: Electron main process
- ❌ NOT YET: Vite Electron config
- ❌ NOT YET: Tailwind CSS initialized
- ❌ NOT YET: Any UI screens

---

## Immediate Next Steps (Priority Order)

### 1. Initialize Tailwind CSS with Gamer Theme
**File to create:** `desktop-app/tailwind.config.js`

```js
/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        // Gamer dark theme with neon accents
        'dark': {
          900: '#0a0a0f',
          800: '#14141f',
          700: '#1e1e2f',
          600: '#28283f',
        },
        'neon': {
          blue: '#00d4ff',
          purple: '#bf00ff',
          pink: '#ff00bf',
          green: '#00ff88',
        },
      },
      fontFamily: {
        gaming: ['Rajdhani', 'sans-serif'],
      },
    },
  },
  plugins: [],
}
```

**File to create:** `desktop-app/src/index.css`

```css
@tailwind base;
@tailwind components;
@tailwind utilities;

@import url('https://fonts.googleapis.com/css2?family=Rajdhani:wght@300;400;500;600;700&display=swap');

body {
  @apply bg-dark-900 text-white font-gaming;
}
```

### 2. Create Electron Main Process
**File to create:** `desktop-app/electron/main.ts`

```typescript
import { app, BrowserWindow } from 'electron';
import path from 'path';

function createWindow() {
  const mainWindow = new BrowserWindow({
    width: 1280,
    height: 800,
    minWidth: 1024,
    minHeight: 600,
    frame: false, // Frameless for custom titlebar
    backgroundColor: '#0a0a0f',
    webPreferences: {
      nodeIntegration: true,
      contextIsolation: false,
      preload: path.join(__dirname, 'preload.js'),
    },
  });

  // In development, load from Vite dev server
  if (process.env.VITE_DEV_SERVER_URL) {
    mainWindow.loadURL(process.env.VITE_DEV_SERVER_URL);
    mainWindow.webContents.openDevTools();
  } else {
    // In production, load from build
    mainWindow.loadFile(path.join(__dirname, '../dist/index.html'));
  }
}

app.whenReady().then(createWindow);

app.on('window-all-closed', () => {
  if (process.platform !== 'darwin') {
    app.quit();
  }
});

app.on('activate', () => {
  if (BrowserWindow.getAllWindows().length === 0) {
    createWindow();
  }
});
```

**File to create:** `desktop-app/electron/preload.ts`

```typescript
// Expose protected methods that allow the renderer process to use
// the ipcRenderer without exposing the entire object
import { contextBridge, ipcRenderer } from 'electron';

contextBridge.exposeInMainWorld('electron', {
  // Add IPC methods here as needed
});
```

### 3. Configure Vite for Electron
**File to update:** `desktop-app/vite.config.ts`

```typescript
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import electron from 'vite-plugin-electron';
import renderer from 'vite-plugin-electron-renderer';

export default defineConfig({
  plugins: [
    react(),
    electron([
      {
        entry: 'electron/main.ts',
      },
      {
        entry: 'electron/preload.ts',
        onstart(options) {
          options.reload();
        },
      },
    ]),
    renderer(),
  ],
});
```

### 4. Create App Structure with Router
**File to update:** `desktop-app/src/App.tsx`

```typescript
import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import SetupWizard from './pages/SetupWizard';
import GameLibrary from './pages/GameLibrary';
import { useSetupStore } from './store/setup';

function App() {
  const { isSetupComplete } = useSetupStore();

  return (
    <BrowserRouter>
      <Routes>
        <Route
          path="/"
          element={isSetupComplete ? <Navigate to="/library" /> : <Navigate to="/setup" />}
        />
        <Route path="/setup/*" element={<SetupWizard />} />
        <Route path="/library" element={<GameLibrary />} />
      </Routes>
    </BrowserRouter>
  );
}

export default App;
```

### 5. Create Zustand Stores

**File to create:** `desktop-app/src/store/setup.ts`

```typescript
import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface SetupState {
  isSetupComplete: boolean;
  steamEnabled: boolean;
  steamApiKey?: string;
  gdriveEnabled: boolean;
  gdriveFolderId?: string;
  gdriveTokens?: any;
  customFolders: string[];

  setSetupComplete: (complete: boolean) => void;
  setSteamConfig: (enabled: boolean, apiKey?: string) => void;
  setGDriveConfig: (enabled: boolean, folderId?: string, tokens?: any) => void;
  addCustomFolder: (folder: string) => void;
}

export const useSetupStore = create<SetupState>()(
  persist(
    (set) => ({
      isSetupComplete: false,
      steamEnabled: false,
      gdriveEnabled: false,
      customFolders: [],

      setSetupComplete: (complete) => set({ isSetupComplete: complete }),
      setSteamConfig: (enabled, apiKey) => set({ steamEnabled: enabled, steamApiKey: apiKey }),
      setGDriveConfig: (enabled, folderId, tokens) =>
        set({ gdriveEnabled: enabled, gdriveFolderId: folderId, gdriveTokens: tokens }),
      addCustomFolder: (folder) =>
        set((state) => ({ customFolders: [...state.customFolders, folder] })),
    }),
    {
      name: 'setup-storage',
    }
  )
);
```

**File to create:** `desktop-app/src/store/games.ts`

```typescript
import { create } from 'zustand';
import type { UnifiedGame } from '@mygamesanywhere/plugin-system';

interface GamesState {
  games: UnifiedGame[];
  loading: boolean;
  error: string | null;

  setGames: (games: UnifiedGame[]) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
}

export const useGamesStore = create<GamesState>((set) => ({
  games: [],
  loading: false,
  error: null,

  setGames: (games) => set({ games }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
}));
```

### 6. Build Setup Wizard Screens

**File to create:** `desktop-app/src/pages/SetupWizard/index.tsx`

```typescript
import { Routes, Route } from 'react-router-dom';
import Welcome from './Welcome';
import SteamSetup from './SteamSetup';
import GDriveSetup from './GDriveSetup';
import CustomFolders from './CustomFolders';
import Scanning from './Scanning';
import Complete from './Complete';

export default function SetupWizard() {
  return (
    <div className="min-h-screen bg-dark-900 flex items-center justify-center">
      <div className="max-w-4xl w-full">
        <Routes>
          <Route path="/" element={<Welcome />} />
          <Route path="/steam" element={<SteamSetup />} />
          <Route path="/gdrive" element={<GDriveSetup />} />
          <Route path="/folders" element={<CustomFolders />} />
          <Route path="/scanning" element={<Scanning />} />
          <Route path="/complete" element={<Complete />} />
        </Routes>
      </div>
    </div>
  );
}
```

**Design for each screen:**
- **Welcome**: Animated logo, "Let's find your games!", big "Get Started" button
- **Steam**: Auto-detect Steam, optional API key input, "Skip" or "Next"
- **GDrive**: OAuth button, folder picker, "Skip" or "Next"
- **Custom Folders**: Folder picker UI, list of added folders, "Skip" or "Next"
- **Scanning**: Progress bar, game count updating live, "Scanning Steam...", "Scanning Google Drive...", etc.
- **Complete**: "Found X games!", big game grid preview, "Launch Library" button

### 7. Build Game Library UI

**File to create:** `desktop-app/src/pages/GameLibrary/index.tsx`

Key features:
- Grid view with cover art (placeholder if no cover)
- Multi-source badges (Steam icon + Local icon + GDrive icon)
- Hover effects (scale, glow)
- Search bar (top)
- Filters (left sidebar: All Games, Installed, Sources, Platforms)
- Sort (top right: Name, Last Played, Playtime)
- Click game → Launch or show details

### 8. Integrate Plugin System

**File to create:** `desktop-app/src/services/PluginService.ts`

```typescript
import { pluginRegistry, UnifiedGameManager, MatchStrategy } from '@mygamesanywhere/plugin-system';
import { SteamSourcePlugin } from '@mygamesanywhere/plugin-steam-source';
import { LaunchBoxIdentifierPlugin } from '@mygamesanywhere/plugin-launchbox-identifier';
import { CustomStorefrontSourcePlugin } from '@mygamesanywhere/plugin-custom-storefront-source';
import type { UnifiedGame } from '@mygamesanywhere/plugin-system';

export class PluginService {
  private manager = new UnifiedGameManager();
  private initialized = false;

  async initialize(config: any): Promise<void> {
    // Initialize Steam if enabled
    if (config.steamEnabled) {
      const steamPlugin = new SteamSourcePlugin();
      await steamPlugin.initialize({
        scanLocal: true,
        scanWeb: !!config.steamApiKey,
        apiKey: config.steamApiKey,
        username: config.steamUsername,
      });
      pluginRegistry.register(steamPlugin);
    }

    // Initialize LaunchBox
    const launchboxPlugin = new LaunchBoxIdentifierPlugin();
    await launchboxPlugin.initialize({ autoDownload: true });
    pluginRegistry.register(launchboxPlugin);

    // Initialize Google Drive if enabled
    if (config.gdriveEnabled && config.gdriveTokens) {
      const gdrivePlugin = new CustomStorefrontSourcePlugin();
      await gdrivePlugin.initialize({
        gdriveFolderId: config.gdriveFolderId,
        gdriveTokens: config.gdriveTokens,
        gdriveCredentials: config.gdriveCredentials,
      });
      pluginRegistry.register(gdrivePlugin);
    }

    this.manager.setMatchStrategy(MatchStrategy.FUZZY_TITLE, 0.85);
    this.initialized = true;
  }

  async scanAllSources(onProgress?: (message: string) => void): Promise<UnifiedGame[]> {
    const sources = pluginRegistry.getAllSources();

    for (const source of sources) {
      onProgress?.(`Scanning ${source.metadata.name}...`);
      const games = await source.scan();

      for (const game of games) {
        this.manager.addDetectedGame({
          sourceId: source.metadata.id,
          gameId: game.id,
          detectedGame: game,
          installed: game.installed || false,
          lastPlayed: game.lastPlayed,
        });
      }
    }

    // Identify games
    const identifiers = pluginRegistry.getAllIdentifiers();
    const unifiedGames = this.manager.getAllGames();

    for (const game of unifiedGames) {
      onProgress?.(`Identifying ${game.title}...`);

      for (const identifier of identifiers) {
        try {
          const result = await identifier.identify(game.sources[0].detectedGame);
          if (result.metadata) {
            this.manager.addIdentification(game.id, identifier.metadata.id, result);
          }
        } catch (error) {
          console.error(`Failed to identify ${game.title}:`, error);
        }
      }
    }

    return this.manager.getAllGames();
  }

  getAllGames(): UnifiedGame[] {
    return this.manager.getAllGames();
  }

  async launchGame(gameId: string, sourceId: string): Promise<void> {
    const source = pluginRegistry.getSource(sourceId);
    const game = this.manager.getGame(gameId);

    if (!source || !game) {
      throw new Error('Game or source not found');
    }

    const sourceData = game.sources.find(s => s.sourceId === sourceId);
    if (!sourceData) {
      throw new Error('Source not found for game');
    }

    if (source.launch) {
      await source.launch(sourceData.gameId);
    }
  }
}

export const pluginService = new PluginService();
```

---

## Testing Checklist

Before considering Phase 3 complete:

- [ ] `npm run dev` starts Electron app successfully
- [ ] Setup wizard flow works end-to-end
- [ ] Steam games detected and displayed
- [ ] Google Drive games detected and displayed
- [ ] Multi-source games show multiple badges
- [ ] Can launch games from library
- [ ] LaunchBox identification shows metadata
- [ ] Cover art displays (or placeholder)
- [ ] Search and filters work
- [ ] UI looks cool with gamer theme

---

## Commands to Run

```bash
# From project root
cd desktop-app

# Install dependencies (if needed)
npm install

# Build integration packages first
cd ../integration-libs
npm run build

# Back to desktop app
cd ../desktop-app

# Run in development
npm run dev

# Build for production
npm run build
```

---

## File Structure (What Exists vs What Needs Creating)

```
desktop-app/
├── electron/                    # ❌ CREATE THIS
│   ├── main.ts                 # ❌ Electron main process
│   └── preload.ts              # ❌ Preload script
├── src/
│   ├── pages/                  # ❌ CREATE THIS
│   │   ├── SetupWizard/       # ❌ Wizard screens
│   │   │   ├── index.tsx
│   │   │   ├── Welcome.tsx
│   │   │   ├── SteamSetup.tsx
│   │   │   ├── GDriveSetup.tsx
│   │   │   ├── CustomFolders.tsx
│   │   │   ├── Scanning.tsx
│   │   │   └── Complete.tsx
│   │   └── GameLibrary/        # ❌ Main library UI
│   │       └── index.tsx
│   ├── store/                  # ❌ CREATE THIS
│   │   ├── setup.ts           # ❌ Setup state
│   │   └── games.ts           # ❌ Games state
│   ├── services/               # ❌ CREATE THIS
│   │   └── PluginService.ts   # ❌ Plugin integration
│   ├── components/             # ❌ CREATE THIS (shared components)
│   ├── App.tsx                 # ✅ EXISTS - UPDATE
│   ├── main.tsx               # ✅ EXISTS
│   └── index.css              # ❌ CREATE THIS (Tailwind imports)
├── package.json               # ✅ EXISTS - UPDATED
├── vite.config.ts            # ✅ EXISTS - UPDATE
├── tailwind.config.js        # ❌ CREATE THIS
├── postcss.config.js         # ❌ CREATE THIS
└── tsconfig.json             # ✅ EXISTS
```

---

## Known Issues to Handle

1. **Node modules in Electron**: Some packages (like SQLite) may need native rebuilds for Electron
   - Solution: Use `electron-builder` with proper config

2. **File paths**: Electron has different path resolution than browser
   - Use `app.getPath()` for user data directory
   - Store LaunchBox DB in `app.getPath('userData')`

3. **Google Drive OAuth in Electron**: Need to handle OAuth redirect
   - Use `shell.openExternal()` for browser login
   - Capture redirect with custom protocol or localhost server

4. **Performance**: Initial scan can take time
   - Show progress UI
   - Consider caching results
   - Background scanning on app start

---

## Success Criteria for Phase 3

Phase 3 is complete when:
- ✅ User can download and run the app
- ✅ First-run setup wizard works for all integrations
- ✅ Game library displays all detected games
- ✅ Multi-source games show correct badges
- ✅ User can launch games from the UI
- ✅ UI looks polished with gamer theme
- ✅ Performance is acceptable (< 2s library load)

---

## Resources

**Documentation:**
- `docs/CURRENT_STATUS.md` - Overall project status
- `docs/ROADMAP.md` - Full project roadmap
- `integration-libs/PLUGIN-SYSTEM.md` - Plugin architecture
- `docs/ARCHITECTURE.md` - System design

**Working Code:**
- `integration-libs/demo-plugin-system.ts` - Plugin usage example
- `integration-libs/packages/plugins/*/src/` - Plugin implementations

**Dependencies:**
- Electron docs: https://www.electronjs.org/docs
- Tailwind CSS: https://tailwindcss.com/docs
- Framer Motion: https://www.framer.com/motion/
- Zustand: https://github.com/pmndrs/zustand

---

## Continuation Instructions

When resuming:

1. **Read this file first** to understand current state
2. **Check `docs/CURRENT_STATUS.md`** for latest updates
3. **Review `desktop-app/package.json`** to see what's installed
4. **Start with step 1** from "Immediate Next Steps" above
5. **Test each component** before moving to the next
6. **Update this file** as you complete steps

**First command to run:**
```bash
cd desktop-app
npm run dev  # This will fail until Electron config is complete
```

Good luck! The foundation is solid, just need to build the UI layers. 🎮
