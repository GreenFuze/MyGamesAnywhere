# @mygamesanywhere/config

Centralized configuration manager for all MyGamesAnywhere integrations.

## Why Centralized Config?

Instead of managing separate `.env` files or configuration for each integration (Steam, Google Drive, IGDB, Epic, GOG, etc.), this package provides a **single source of truth** for all your credentials and settings.

## Features

- 🎯 **Single Config File**: One place for all integration credentials
- 🔐 **Secure Storage**: Stored in user's home directory (`~/.mygamesanywhere/config.json`)
- 🌍 **Environment Override**: Support for environment variables
- 🛡️ **Type-Safe**: Full TypeScript support with Zod validation
- 🔧 **Easy Updates**: Simple API to update individual platform settings
- ✅ **Validated**: Automatic validation of all configuration

## Installation

```bash
npm install @mygamesanywhere/config
```

## Quick Start

### Option 1: Using Config File (Recommended)

**1. Create config file:**

```bash
# Copy example to your home directory
cp config.example.json ~/.mygamesanywhere/config.json

# Edit with your credentials
notepad ~/.mygamesanywhere/config.json  # Windows
nano ~/.mygamesanywhere/config.json     # macOS/Linux
```

**2. Fill in your credentials:**

```json
{
  "steam": {
    "apiKey": "A1B2C3D4E5F6...",
    "steamId": "76561198012345678"
  },
  "googleDrive": {
    "clientId": "your-client-id.apps.googleusercontent.com",
    "clientSecret": "your-client-secret",
    "redirectUri": "http://localhost:3000/oauth/callback"
  },
  "igdb": {
    "clientId": "your-twitch-client-id",
    "clientSecret": "your-twitch-client-secret"
  }
}
```

**3. Use in your app:**

```typescript
import { getConfigManager } from '@mygamesanywhere/config';

// Load configuration
const config = getConfigManager();
await config.load();

// Access Steam settings
const steamConfig = config.getSteam();
console.log(steamConfig.apiKey);
console.log(steamConfig.steamId);

// Access Google Drive settings
const driveConfig = config.getGoogleDrive();
console.log(driveConfig.clientId);
```

### Option 2: Using Environment Variables

Set environment variables:

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

Environment variables **override** config file values.

## Usage with Integrations

### Steam Integration

```typescript
import { getConfigManager } from '@mygamesanywhere/config';
import { SteamClient } from '@mygamesanywhere/steam-scanner';

// Load config
const config = getConfigManager();
await config.load();

const steamConfig = config.getSteam();

if (!steamConfig?.apiKey || !steamConfig?.steamId) {
  throw new Error('Steam not configured. Please add credentials to ~/.mygamesanywhere/config.json');
}

// Create Steam client with config
const steamClient = new SteamClient({
  apiKey: steamConfig.apiKey,
  steamId: steamConfig.steamId,
});

const games = await steamClient.getOwnedGames();
```

### Google Drive Integration

```typescript
import { getConfigManager } from '@mygamesanywhere/config';
import { DriveClient } from '@mygamesanywhere/gdrive-client';

const config = getConfigManager();
await config.load();

const driveConfig = config.getGoogleDrive();

const driveClient = new DriveClient({
  oauth: {
    clientId: driveConfig.clientId!,
    clientSecret: driveConfig.clientSecret!,
    redirectUri: driveConfig.redirectUri!,
  },
});
```

### IGDB Integration

```typescript
import { getConfigManager } from '@mygamesanywhere/config';
import { IGDBClient } from '@mygamesanywhere/igdb-client';

const config = getConfigManager();
await config.load();

const igdbConfig = config.getIGDB();

const igdbClient = new IGDBClient({
  clientId: igdbConfig.clientId!,
  clientSecret: igdbConfig.clientSecret!,
});
```

## API Reference

### `ConfigManager`

Main configuration manager class.

#### Methods

**`load(): Promise<MyGamesAnywhereConfig>`**
- Loads configuration from file and environment variables
- Returns the complete configuration object

**`save(config: MyGamesAnywhereConfig): Promise<void>`**
- Saves configuration to file
- Validates before saving

**`get(): MyGamesAnywhereConfig`**
- Returns current configuration

**`getSteam(): SteamConfig | undefined`**
- Returns Steam configuration

**`getGoogleDrive(): GoogleDriveConfig | undefined`**
- Returns Google Drive configuration

**`getIGDB(): IGDBConfig | undefined`**
- Returns IGDB configuration

**`updateSteam(config: Partial<SteamConfig>): Promise<void>`**
- Updates Steam configuration
- Automatically saves to file

**`updateGoogleDrive(config: Partial<GoogleDriveConfig>): Promise<void>`**
- Updates Google Drive configuration

**`updateIGDB(config: Partial<IGDBConfig>): Promise<void>`**
- Updates IGDB configuration

**`clear(): Promise<void>`**
- Clears all configuration

**`isConfigured(platform: string): boolean`**
- Checks if a platform is configured

**`validate(): boolean`**
- Validates current configuration

**`getConfigPath(): string`**
- Returns path to config file

### `getConfigManager(configPath?: string): ConfigManager`

Get or create the global ConfigManager instance.

## Configuration Structure

```typescript
interface MyGamesAnywhereConfig {
  steam?: {
    apiKey?: string;      // From https://steamcommunity.com/dev/apikey
    steamId?: string;     // From https://steamid.io/
    customPath?: string;  // Custom Steam install path
  };

  googleDrive?: {
    clientId?: string;
    clientSecret?: string;
    redirectUri?: string;
  };

  igdb?: {
    clientId?: string;    // Twitch Client ID
    clientSecret?: string; // Twitch Client Secret
  };

  epic?: {
    email?: string;
    customPath?: string;
  };

  gog?: {
    token?: string;
    customPath?: string;
  };

  xbox?: {
    email?: string;
  };
}
```

## Where Credentials Come From

### Steam
- **API Key**: https://steamcommunity.com/dev/apikey
- **Steam ID**: https://steamid.io/ (use steamID64)

### Google Drive
- **Credentials**: https://console.cloud.google.com/
- Enable Google Drive API
- Create OAuth 2.0 credentials

### IGDB
- **Credentials**: https://api-docs.igdb.com/#account-creation
- Register Twitch application
- Use Twitch Client ID and Secret

### Epic, GOG, Xbox
- Coming soon in future updates

## Environment Variable Names

The following environment variables are supported:

```bash
# Steam
STEAM_API_KEY
STEAM_ID

# Google Drive
GOOGLE_CLIENT_ID
GOOGLE_CLIENT_SECRET
GOOGLE_REDIRECT_URI

# IGDB
IGDB_CLIENT_ID
IGDB_CLIENT_SECRET
```

## Config File Location

**Default location:**
- Windows: `C:\Users\<username>\.mygamesanywhere\config.json`
- macOS: `/Users/<username>/.mygamesanywhere/config.json`
- Linux: `/home/<username>/.mygamesanywhere/config.json`

**Custom location:**
```typescript
import { ConfigManager } from '@mygamesanywhere/config';

const config = new ConfigManager('/custom/path/to/config.json');
await config.load();
```

## Security Best Practices

⚠️ **IMPORTANT:**

1. **Never commit config.json to Git**
   - Add `~/.mygamesanywhere/` to global `.gitignore`

2. **Set proper file permissions**
   ```bash
   # macOS/Linux
   chmod 600 ~/.mygamesanywhere/config.json
   ```

3. **Use environment variables in CI/CD**
   - Don't store credentials in code or repositories

4. **Rotate keys if compromised**
   - Regenerate API keys if they're exposed

## Examples

### Check if Configured

```typescript
const config = getConfigManager();
await config.load();

if (!config.isConfigured('steam')) {
  console.log('Steam not configured');
  // Show setup wizard
}
```

### Update Configuration Programmatically

```typescript
const config = getConfigManager();
await config.load();

// Update Steam settings
await config.updateSteam({
  apiKey: 'new-api-key',
  steamId: 'new-steam-id',
});

console.log('Steam configuration updated!');
```

### Validate Configuration

```typescript
const config = getConfigManager();
await config.load();

if (config.validate()) {
  console.log('✅ Configuration is valid');
} else {
  console.error('❌ Configuration has errors');
}
```

## Part of MyGamesAnywhere

This package is part of the MyGamesAnywhere project - a cross-platform game launcher and manager.

- **GitHub:** https://github.com/GreenFuze/MyGamesAnywhere

## License

GPL-3.0
