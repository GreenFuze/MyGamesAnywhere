# MyGamesAnywhere

**A serverless, cross-platform game launcher and manager with cloud sync.**

Manage your entire game library from multiple sources (Steam, local folders, cloud storage) in one unified interface. All data syncs via your own cloud storage - no third-party servers required!

## 🌟 Features

- **Multi-Source Game Library**
  - Steam integration (installed games, library, launch)
  - Local directory scanning (installers, portable games, ROMs)
  - Google Drive scanning
  - More sources coming soon (Epic, GOG, etc.)

- **Smart Game Detection**
  - Installers (.exe, .msi, .pkg, .deb, .rpm)
  - Portable games (game directories)
  - ROMs (NES, SNES, PlayStation, etc.)
  - Archives (single & multi-part: .zip, .rar, .7z, .part1, etc.)
  - Cross-platform support (Windows, Linux, macOS)

- **Cloud Sync** ✨
  - Sync your library across all devices
  - Uses YOUR cloud storage (Google Drive, OneDrive)
  - No third-party servers - you own your data!

- **Metadata & Media**
  - Automatic metadata fetching (IGDB integration planned)
  - Cover art, screenshots, descriptions
  - Playtime tracking

## 🚀 Quick Start

### Prerequisites

- Node.js 18+ installed
- npm or yarn

### Installation

```bash
# Clone repository
git clone https://github.com/GreenFuze/MyGamesAnywhere.git
cd MyGamesAnywhere

# Install dependencies
cd integration-libs
npm install
```

### Google Drive Authentication

No setup required! Just authenticate:

```bash
cd integration-libs/packages/gdrive-client
npm run test:auth
```

This will:
1. Open your browser to Google login
2. You authorize MyGamesAnywhere
3. Token saved to `~/.mygamesanywhere/.gdrive-tokens.json`

✅ Done! Now you can scan Google Drive for games.

### Scan Google Drive for Games

```bash
cd integration-libs/packages/generic-repository
npm run test:scan-gdrive

# Or scan specific folder
npm run test:scan-gdrive YOUR_FOLDER_ID
```

### Steam Integration

```bash
cd integration-libs/packages/steam-scanner
npm run test:steam-api YOUR_STEAM_USERNAME
```

See [SETUP.md](./SETUP.md) for detailed setup instructions.

## 📚 Documentation

- **[SETUP.md](./SETUP.md)** - Complete setup guide for all integrations
- **[GOOGLE_DRIVE_SETUP.md](./GOOGLE_DRIVE_SETUP.md)** - Google Drive authentication & scanning
- **[ARCHITECTURE.md](./docs/ARCHITECTURE.md)** - System architecture and design
- **[ROADMAP.md](./docs/ROADMAP.md)** - Development roadmap and phases
- **[CLAUDE.md](./CLAUDE.md)** - Project overview and development notes

## 🏗️ Project Structure

```
MyGamesAnywhere/
├── integration-libs/          # Phase 1: Standalone integration packages
│   ├── packages/
│   │   ├── steam-scanner/     # ✅ Steam integration (88 tests)
│   │   ├── gdrive-client/     # ✅ Google Drive OAuth & file ops (41 tests)
│   │   ├── generic-repository/# ✅ Local/cloud game scanner
│   │   ├── igdb-client/       # 🚧 Game metadata (planned)
│   │   └── config/            # ✅ Centralized configuration
│   └── ...
├── docs/                      # Architecture & design docs
└── README.md                  # This file
```

## 🧪 Testing

Each package has comprehensive tests:

```bash
# Test Steam scanner
cd integration-libs/packages/steam-scanner
npm test

# Test Google Drive client
cd integration-libs/packages/gdrive-client
npm test

# Test generic repository scanner
cd integration-libs/packages/generic-repository
npm test
```

## 🔐 Privacy & Security

- **No third-party servers** - Your data stays on your devices and your cloud storage
- **You own your data** - Library stored in your Google Drive/OneDrive
- **OAuth tokens local** - Stored at `~/.mygamesanywhere/.gdrive-tokens.json`
- **API keys encrypted** - Credentials never shared

## 🛠️ Current Status

**Phase 1: Core Integrations** (In Progress)

✅ **Completed:**
- Steam scanner with VDF parsing
- Steam Web API integration (username-based)
- Google Drive OAuth authentication
- Google Drive file operations
- Generic repository scanner (local & cloud)
- Multi-format game detection
- Cross-platform support

🚧 **In Progress:**
- IGDB metadata integration
- Native launcher for process management

See [ROADMAP.md](./docs/ROADMAP.md) for full development plan.

## 🤝 Contributing

Contributions welcome! See [CONTRIBUTING.md](./CONTRIBUTING.md) for guidelines.

## 📄 License

GPL-3.0 - See [LICENSE.md](./LICENSE.md)

## 🔗 Links

- **GitHub:** https://github.com/GreenFuze/MyGamesAnywhere
- **Issues:** https://github.com/GreenFuze/MyGamesAnywhere/issues
- **Discussions:** https://github.com/GreenFuze/MyGamesAnywhere/discussions

---

**Made with ❤️ for the gaming community**
