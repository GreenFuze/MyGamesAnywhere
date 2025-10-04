# Credentials Guide

This guide shows you **exactly** what to put in your config file.

## Quick Reference

### ❓ What you see vs. What you put

| Where | What You See | What You Put in Config |
|-------|--------------|------------------------|
| **Steam API Key page** | `Key: A1B2C3D4E5F6G7H8I9J0K1L2M3N4O5P6` | `"apiKey": "A1B2C3D4E5F6G7H8I9J0K1L2M3N4O5P6"` |
| **Steam Profile** | Username: `greenfuze` | `"username": "greenfuze"` |
| **Google Cloud Console** | `Client ID: 123-abc.apps.googleusercontent.com` | `"clientId": "123-abc.apps.googleusercontent.com"` |

## Steam Example

### Step 1: Visit https://steamcommunity.com/dev/apikey

You'll see something like:
```
Key: A1B2C3D4E5F6G7H8I9J0K1L2M3N4O5P6
```

### Step 2: Get your Steam username

Your Steam username is the name you use to log in, or your custom URL.

For example, if your profile is:
- `steamcommunity.com/id/greenfuze` → username is `greenfuze`
- Or just the username you use to log in to Steam

### Step 3: Create `~/.mygamesanywhere/config.json`

```json
{
  "steam": {
    "apiKey": "A1B2C3D4E5F6G7H8I9J0K1L2M3N4O5P6",
    "username": "greenfuze"
  }
}
```

**✅ CORRECT - Just the values, no URLs, no extra text**

**❌ WRONG:**
```json
{
  "steam": {
    "apiKey": "YOUR_STEAM_API_KEY_FROM_https://steamcommunity.com/dev/apikey",
    "username": "YOUR_STEAM_USERNAME_FROM_https://steamcommunity.com/"
  }
}
```

## Google Drive Example

### Step 1: Visit https://console.cloud.google.com/apis/credentials

Create OAuth 2.0 Client ID, you'll see:
```
Client ID: 123456789-abc123.apps.googleusercontent.com
Client Secret: GOCSPX-abc123def456ghi789
```

### Step 2: Add to config

```json
{
  "googleDrive": {
    "clientId": "123456789-abc123.apps.googleusercontent.com",
    "clientSecret": "GOCSPX-abc123def456ghi789",
    "redirectUri": "http://localhost:3000/oauth/callback"
  }
}
```

## IGDB Example

### Step 1: Visit https://dev.twitch.tv/console/apps

Register application, you'll see:
```
Client ID: abc123def456ghi789
Client Secret: xyz987uvw654rst321
```

### Step 2: Add to config

```json
{
  "igdb": {
    "clientId": "abc123def456ghi789",
    "clientSecret": "xyz987uvw654rst321"
  }
}
```

## Complete Example

Here's what a complete `~/.mygamesanywhere/config.json` looks like:

```json
{
  "steam": {
    "apiKey": "A1B2C3D4E5F6G7H8I9J0K1L2M3N4O5P6",
    "username": "greenfuze"
  },
  "googleDrive": {
    "clientId": "123456789-abc123.apps.googleusercontent.com",
    "clientSecret": "GOCSPX-abc123def456ghi789",
    "redirectUri": "http://localhost:3000/oauth/callback"
  },
  "igdb": {
    "clientId": "abc123def456ghi789",
    "clientSecret": "xyz987uvw654rst321"
  }
}
```

## Common Mistakes

### ❌ Mistake 1: Including the URL

**WRONG:**
```json
"apiKey": "Get from https://steamcommunity.com/dev/apikey"
```

**RIGHT:**
```json
"apiKey": "A1B2C3D4E5F6G7H8I9J0K1L2M3N4O5P6"
```

### ❌ Mistake 2: Using Steam ID instead of username

**WRONG:**
```json
"username": "76561198012345678"
```

**RIGHT:**
```json
"username": "greenfuze"
```

### ❌ Mistake 3: Forgetting quotes

**WRONG:**
```json
"apiKey": A1B2C3D4E5F6G7H8I9J0K1L2M3N4O5P6
```

**RIGHT:**
```json
"apiKey": "A1B2C3D4E5F6G7H8I9J0K1L2M3N4O5P6"
```

### ❌ Mistake 4: Extra spaces

**WRONG:**
```json
"apiKey": " A1B2C3D4E5F6G7H8I9J0K1L2M3N4O5P6 "
```

**RIGHT:**
```json
"apiKey": "A1B2C3D4E5F6G7H8I9J0K1L2M3N4O5P6"
```

## Validation

To check if your config is valid:

```typescript
import { getConfigManager } from '@mygamesanywhere/config';

const config = getConfigManager();
await config.load();

if (config.validate()) {
  console.log('✅ Config is valid!');
} else {
  console.log('❌ Config has errors');
}

// Check specific platform
if (config.isConfigured('steam')) {
  console.log('✅ Steam is configured');
}
```

## File Location

**Where to save:** `~/.mygamesanywhere/config.json`

**Full paths:**
- Windows: `C:\Users\<your-username>\.mygamesanywhere\config.json`
- macOS: `/Users/<your-username>/.mygamesanywhere/config.json`
- Linux: `/home/<your-username>/.mygamesanywhere/config.json`

**Quick create:**
```bash
# Create directory
mkdir ~/.mygamesanywhere

# Create config file (empty)
echo '{}' > ~/.mygamesanywhere/config.json

# Edit with your favorite editor
notepad ~/.mygamesanywhere/config.json  # Windows
nano ~/.mygamesanywhere/config.json     # macOS/Linux
code ~/.mygamesanywhere/config.json     # VS Code
```

## Security

⚠️ **IMPORTANT:**

1. **Never share your config file**
   - Contains sensitive API keys
   - Like sharing your passwords

2. **Set proper permissions (macOS/Linux)**
   ```bash
   chmod 600 ~/.mygamesanywhere/config.json
   ```

3. **Never commit to Git**
   - Already in `.gitignore`
   - Double-check before committing

4. **Regenerate if compromised**
   - Go back to the credential source
   - Generate new keys
   - Update config file
