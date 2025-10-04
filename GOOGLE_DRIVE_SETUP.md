# Google Drive Setup & Testing Guide

This guide walks you through authenticating with Google Drive and testing the generic repository scanner.

## ✅ OAuth Credentials Configured

OAuth credentials for MyGamesAnywhere are now configured and embedded in the application. No additional setup is required!

**How it works:**
- OAuth credentials identify the MyGamesAnywhere app to Google
- When you authenticate, YOU get a personal token tied to YOUR Google account
- Credentials are the same for all users (standard for desktop apps like GitHub CLI, VS Code, etc.)
- Each user's token is private and stored on their own computer

**For project maintainers:** See [`GOOGLE_OAUTH_SETUP_MAINTAINER.md`](./integration-libs/packages/gdrive-client/GOOGLE_OAUTH_SETUP_MAINTAINER.md) for OAuth setup documentation.

## Quick Start for Users

Once the OAuth credentials are configured, authentication is simple:

```bash
# 1. Install dependencies (if not already done)
cd integration-libs
npm install

# 2. Authenticate with Google Drive
cd packages/gdrive-client
npm run test:auth

# 3. Scan Google Drive for games
cd ../generic-repository
npm run test:scan-gdrive
```

That's it! No manual OAuth setup required.

## Detailed Instructions

### Step 1: Authenticate with Google Drive

```bash
cd integration-libs/packages/gdrive-client
npm run test:auth
```

**What happens:**
1. Opens your browser to Google's login page
2. You log in and authorize MyGamesAnywhere
3. Tokens are saved to `~/.mygamesanywhere/.gdrive-tokens.json`
4. Lists some files from your Google Drive

**Expected output:**
```
=== Google Drive Authentication Test ===

✅ Already authenticated with Google Drive

📂 Testing Google Drive access...

📋 Listing files...
✅ Found 42 files

Recent files:
  - My Document.pdf (125 KB)
  - Vacation Photos (folder)
  - Game ROMs (folder)
  ...

✅ Authentication status: Authenticated

✨ Success! Google Drive is connected.
```

**Note:** If already authenticated, it will skip the browser login and use cached tokens.

### Step 2: Scan Google Drive for Games

```bash
cd integration-libs/packages/generic-repository
npm run test:scan-gdrive
```

**Scan specific folder:**
```bash
# Get folder ID from Google Drive URL
# Example: https://drive.google.com/drive/folders/1ABC123xyz...
npm run test:scan-gdrive 1ABC123xyz...
```

**What it detects:**
- ✅ Installers (.exe, .msi, .pkg, .deb, .rpm)
- ✅ Portable games (game directories)
- ✅ ROMs (NES, SNES, PlayStation, etc.)
- ✅ Archives (.zip, .rar, .7z, multi-part)
- ✅ Emulator-required games

**Expected output:**
```
=== Google Drive Game Scanner Test ===

✅ Authenticated with Google Drive

📂 Scanning root folder of Google Drive

🔍 Scanning for games...

=== Scan Results ===

Duration: 3452ms
Files scanned: 127
Directories scanned: 18
Games found: 23

📦 ROM (15):
  - Super Mario World (confidence: 95%)
  - Legend Of Zelda (confidence: 95%)
  - Pokemon Red (confidence: 95%)
  - Sonic The Hedgehog (confidence: 95%)
  - Mega Man X (confidence: 95%)
  ... and 10 more

📦 ARCHIVED (5):
  - Game Collection.zip (confidence: 80%)
  - DOS Games.rar (confidence: 80%)
  - Retro Pack.part1.rar (confidence: 85%)
  ...

📦 INSTALLER_EXECUTABLE (3):
  - Game Setup.exe (confidence: 90%)
  - Install Game.exe (confidence: 90%)
  ...

✨ Scan complete!
```

## Troubleshooting

### Issue: "Not authenticated with Google Drive"

**Solution:**
```bash
cd integration-libs/packages/gdrive-client
npm run test:auth
```

### Issue: "YOUR_CLIENT_ID_HERE.apps.googleusercontent.com"

This means the OAuth credentials haven't been set up yet. This is for the **project maintainer** to do. See `GOOGLE_OAUTH_SETUP_MAINTAINER.md`.

### Issue: "redirect_uri_mismatch"

The OAuth credentials need to have `http://localhost:3000/oauth/callback` added as an authorized redirect URI in Google Cloud Console. Contact the project maintainer.

### Issue: "Access blocked: This app's request is invalid"

The OAuth consent screen needs to be configured in Google Cloud Console. Contact the project maintainer.

### Issue: "Token expired"

Tokens auto-refresh automatically. If you encounter this error:

```bash
# Delete tokens and re-authenticate
rm ~/.mygamesanywhere/.gdrive-tokens.json
cd integration-libs/packages/gdrive-client
npm run test:auth
```

## File Locations

**Tokens:**
- `~/.mygamesanywhere/.gdrive-tokens.json` - OAuth tokens (auto-refreshed)

**Never commit:**
- `.gdrive-tokens.json` (already in .gitignore)
- OAuth credentials should be environment variables

## Using in Your Code

```typescript
import { authenticateGDrive, DriveClient } from '@mygamesanywhere/gdrive-client';
import { GDriveRepository, RepositoryScanner } from '@mygamesanywhere/generic-repository';

// Authenticate
const auth = await authenticateGDrive();

// Create client
const client = new DriveClient({
  oauth: { clientId: '', clientSecret: '', redirectUri: '' },
  tokenStorage: auth.getTokenStorage(),
});
(client as any).oauth = auth.getOAuthClient();

// Scan for games
const repository = new GDriveRepository(client, 'root');
const scanner = new RepositoryScanner(repository);
const result = await scanner.scan();

console.log(`Found ${result.games.length} games!`);
```

## Next Steps

Once authenticated and scanning works:

1. **Test with different folders**
   ```bash
   npm run test:scan-gdrive YOUR_FOLDER_ID
   ```

2. **Scan local directories**
   ```bash
   cd integration-libs/packages/generic-repository
   npm run test:scan /path/to/games
   ```

3. **Implement metadata fetching** (IGDB integration)

4. **Build installation management** (Phase 2)

5. **Implement save sync** (Phase 4)

## Architecture

```
Google Drive Repository Scanner
├── @mygamesanywhere/gdrive-client
│   ├── GDriveAuth (simplified OAuth helper)
│   ├── OAuth 2.0 authentication
│   ├── File operations (list, download, upload)
│   └── Token management (auto-refresh)
│
├── @mygamesanywhere/generic-repository
│   ├── GDriveRepository adapter
│   ├── File classification
│   ├── Game detection
│   └── Multi-format support
│
└── User's home directory
    └── ~/.mygamesanywhere/.gdrive-tokens.json
```

## Security Notes

⚠️ **Important:**

1. **OAuth tokens are sensitive**
   - Stored in `~/.mygamesanywhere/.gdrive-tokens.json`
   - Never commit tokens to Git
   - Tokens are auto-refreshed
   - Tokens expire if not used for 6 months

2. **Client credentials (for maintainers)**
   - Use environment variables
   - Never commit to Git
   - Rotate if compromised

3. **Scopes granted:**
   - `drive.file` - Access only files created by this app
   - `drive.appdata` - Access app data folder
   - NOT full Drive access (more secure)

## Links

- [Google Cloud Console](https://console.cloud.google.com/)
- [Google Drive API Docs](https://developers.google.com/drive/api/v3/about-sdk)
- [OAuth 2.0 Guide](https://developers.google.com/identity/protocols/oauth2)
- [Project Documentation](./docs/)
