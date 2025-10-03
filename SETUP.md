# MyGamesAnywhere Setup Guide

This guide will help you configure all integrations for MyGamesAnywhere.

## Centralized Configuration

MyGamesAnywhere uses a **single configuration file** for all integrations (Steam, Google Drive, IGDB, etc.).

### Location

**Config file:** `~/.mygamesanywhere/config.json`

- Windows: `C:\Users\<your-username>\.mygamesanywhere\config.json`
- macOS: `/Users/<your-username>/.mygamesanywhere/config.json`
- Linux: `/home/<your-username>/.mygamesanywhere/config.json`

## Quick Setup

### Step 1: Create Config File

```bash
# Create directory
mkdir ~/.mygamesanywhere

# Windows (PowerShell)
New-Item -ItemType Directory -Path "$env:USERPROFILE\.mygamesanywhere" -Force

# Copy example config
cp integration-libs/packages/config/config.example.json ~/.mygamesanywhere/config.json
```

### Step 2: Get Your Credentials

#### Steam (Required for Steam features)

1. **API Key:**
   - Visit: https://steamcommunity.com/dev/apikey
   - Sign in and generate a key
   - Domain can be anything (e.g., "localhost")

2. **Steam ID:**
   - Visit: https://steamid.io/
   - Enter your profile URL
   - Copy the **steamID64** (17-digit number)

#### Google Drive (Required for cloud sync)

1. Visit: https://console.cloud.google.com/
2. Create a new project
3. Enable "Google Drive API"
4. Create OAuth 2.0 credentials:
   - Application type: Web application
   - Authorized redirect URI: `http://localhost:3000/oauth/callback`
5. Copy Client ID and Client Secret

#### IGDB (Required for game metadata)

1. Visit: https://api-docs.igdb.com/#account-creation
2. Register a Twitch application
3. Copy Twitch Client ID and Client Secret

### Step 3: Edit Config File

Open `~/.mygamesanywhere/config.json` and fill in your credentials:

```json
{
  "steam": {
    "apiKey": "YOUR_STEAM_API_KEY",
    "steamId": "YOUR_STEAM_ID_64"
  },
  "googleDrive": {
    "clientId": "YOUR_GOOGLE_CLIENT_ID",
    "clientSecret": "YOUR_GOOGLE_CLIENT_SECRET",
    "redirectUri": "http://localhost:3000/oauth/callback"
  },
  "igdb": {
    "clientId": "YOUR_TWITCH_CLIENT_ID",
    "clientSecret": "YOUR_TWITCH_CLIENT_SECRET"
  }
}
```

## Testing Your Setup

### Test Steam Integration

```bash
cd integration-libs/packages/steam-scanner
npm run test:steam-api
```

This will:
- Load your credentials from `~/.mygamesanywhere/config.json`
- Fetch all your owned games from Steam
- Show statistics and recently played games
- Combine with locally installed games

### Test Google Drive

```bash
cd integration-libs/packages/gdrive-client
npx tsx examples/basic-oauth-flow.ts
```

This will:
- Start OAuth flow
- Open browser for authorization
- Save tokens
- Test file operations

## Environment Variables (Alternative)

You can also use environment variables instead of the config file:

```bash
# Windows (PowerShell)
$env:STEAM_API_KEY="your-key"
$env:STEAM_ID="your-id"
$env:GOOGLE_CLIENT_ID="your-client-id"
$env:GOOGLE_CLIENT_SECRET="your-secret"

# macOS/Linux
export STEAM_API_KEY="your-key"
export STEAM_ID="your-id"
export GOOGLE_CLIENT_ID="your-client-id"
export GOOGLE_CLIENT_SECRET="your-secret"
```

Environment variables **override** the config file.

## Security

⚠️ **IMPORTANT:**

1. **Never commit `~/.mygamesanywhere/config.json` to Git**
2. **Set proper permissions:**
   ```bash
   # macOS/Linux
   chmod 600 ~/.mygamesanywhere/config.json
   ```
3. **Regenerate keys if compromised**

## Troubleshooting

### "Config file not found"

Create it manually:
```bash
mkdir -p ~/.mygamesanywhere
echo '{}' > ~/.mygamesanywhere/config.json
```

### "Invalid API key"

- Double-check you copied the entire key
- Make sure there are no extra spaces
- Regenerate the key if needed

### "Steam ID not found"

- Make sure you're using the **steamID64** (17-digit number)
- Not steamID or steamID3

## Next Steps

Once configured, you can use all integrations:

```typescript
import { getConfigManager } from '@mygamesanywhere/config';
import { SteamClient } from '@mygamesanywhere/steam-scanner';
import { DriveClient } from '@mygamesanywhere/gdrive-client';

// Load config
const config = getConfigManager();
await config.load();

// Use Steam
const steamConfig = config.getSteam();
const steamClient = new SteamClient(steamConfig);
const games = await steamClient.getOwnedGames();

// Use Google Drive
const driveConfig = config.getGoogleDrive();
const driveClient = new DriveClient({ oauth: driveConfig });
```

## Documentation

- **Steam Scanner:** `integration-libs/packages/steam-scanner/README.md`
- **Google Drive:** `integration-libs/packages/gdrive-client/README.md`
- **Config Manager:** `integration-libs/packages/config/README.md`
