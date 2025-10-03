# Testing Steam Web API Integration

This guide will help you test the Steam Web API features.

## Prerequisites

You need:
1. A Steam account
2. Games in your Steam library (at least 1 game)
3. Internet connection

## Step 1: Get Your Steam Credentials

### A. Get Steam Web API Key

1. **Visit:** https://steamcommunity.com/dev/apikey
2. **Sign in** with your Steam account
3. **Domain Name:** Enter any domain (e.g., "localhost" or "mygamesanywhere.com")
4. **Submit** and copy your API key

Your API key looks like: `A1B2C3D4E5F6G7H8I9J0K1L2M3N4O5P6`

### B. Get Your Steam ID (64-bit)

1. **Visit:** https://steamid.io/
2. **Enter** your Steam profile URL or username
3. **Copy** your **steamID64** (17-digit number)

Your Steam ID looks like: `76561198012345678`

## Step 2: Configure Your Credentials

### Option A: Using .env File (Recommended)

1. **Navigate to the steam-scanner package:**
   ```bash
   cd integration-libs/packages/steam-scanner
   ```

2. **Copy the example file:**
   ```bash
   # Windows (PowerShell)
   Copy-Item .env.example .env

   # macOS/Linux
   cp .env.example .env
   ```

3. **Add your credentials to the config file:**

   Create or edit `~/.mygamesanywhere/config.json`:

   ```json
   {
     "steam": {
       "apiKey": "A1B2C3D4E5F6G7H8I9J0K1L2M3N4O5P6",
       "steamId": "76561198012345678"
     }
   }
   ```

   **IMPORTANT:** Replace with YOUR actual values:
   - `apiKey`: The 32-character key from Steam (letters and numbers)
   - `steamId`: Your 17-digit Steam ID

4. **Save the file**

### Option B: Using Environment Variables

**Windows (PowerShell):**
```powershell
$env:STEAM_API_KEY="your_api_key_here"
$env:STEAM_ID="your_steam_id_here"
```

**macOS/Linux:**
```bash
export STEAM_API_KEY="your_api_key_here"
export STEAM_ID="your_steam_id_here"
```

## Step 3: Run the Test

```bash
cd integration-libs/packages/steam-scanner
npm run test:steam-api
```

## What the Test Does

The test script will:

1. ✅ **Load your credentials** from `.env`
2. ✅ **Fetch all owned games** from Steam Web API
3. ✅ **Calculate statistics** (total games, playtime, etc.)
4. ✅ **Show top 10 most played games**
5. ✅ **Fetch recently played games**
6. ✅ **Scan locally installed games** (VDF files)
7. ✅ **Combine both sources** to show installation status
8. ✅ **Test Store API** (no auth required)
9. ✅ **Check if Steam client is running**
10. ✅ **Generate image URLs** for games

## Expected Output

```
=== Steam Web API Test ===

✅ Configuration loaded
API Key: A1B2C3D4...
Steam ID: 76561198012345678

📚 Fetching owned games from Steam Web API...

✅ Success! You own 234 games

📊 Library Summary:
  Total games: 234
  Total playtime: 1,234 hours (51 days)

🏆 Top 10 Most Played:
  1. Team Fortress 2
     567 hours (App ID: 440)
  2. Counter-Strike 2
     234 hours (App ID: 730)
  ...

🎮 Fetching recently played games...

✅ Recently played (5):
  1. Cyberpunk 2077 - 123 hours total
  2. Baldur's Gate 3 - 89 hours total
  ...

🔍 Scanning locally installed games...

✅ Found 45 installed games locally

🔗 Combining Web API + Local Scan...

📈 Combined Statistics:
  Total owned: 234
  Installed: 45
  Not installed: 189
  Installation rate: 19%

💾 Some games you own but haven't installed:
  1. Portal 2 (5 hours played)
  2. Half-Life 2
  ...

🖼️  Image URLs for "Team Fortress 2":
  Header: https://cdn.cloudflare.steamstatic.com/steam/apps/440/header.jpg
  Hero: https://cdn.cloudflare.steamstatic.com/steam/apps/440/library_hero.jpg
  Capsule: https://cdn.cloudflare.steamstatic.com/steam/apps/440/library_600x900.jpg

✅ All tests passed!

📝 Summary:
  ✅ Steam Web API - Working
  ✅ Local VDF Scanner - Working
  ✅ Combined Detection - Working
  ✅ Store API - Working
  ✅ Client Detection - Working

🎉 Steam integration is fully functional!
```

## Troubleshooting

### Error: "403 Forbidden" or "401 Unauthorized"

**Problem:** Invalid API key or Steam ID

**Solution:**
- Double-check your API key at https://steamcommunity.com/dev/apikey
- Verify your Steam ID at https://steamid.io/
- Make sure you copied the **steamID64** (not steamID or steamID3)

### Error: ".env file not found"

**Problem:** You haven't created the `.env` file

**Solution:**
```bash
cd integration-libs/packages/steam-scanner
cp .env.example .env
# Then edit .env with your credentials
```

### Error: "ENOTFOUND" or Network Error

**Problem:** No internet connection or Steam API is down

**Solution:**
- Check your internet connection
- Try again in a few minutes
- Check Steam API status: https://steamstat.us/

### Error: "Steam not found"

**Problem:** Steam is not installed on your system

**Solution:**
- The local VDF scan will fail, but Web API will still work
- Install Steam from: https://store.steampowered.com/

## Security Notes

⚠️ **IMPORTANT:**

1. **Never commit your `.env` file to Git**
   - It's already in `.gitignore`
   - Your API key is like a password

2. **Never share your API key publicly**
   - Don't post it in issues, chat, or screenshots

3. **Regenerate your key if compromised**
   - Visit: https://steamcommunity.com/dev/apikey
   - Click "Revoke my current key" and generate a new one

## Next Steps

After testing, you can use the same credentials in your MyGamesAnywhere app:

```typescript
import { SteamClient } from '@mygamesanywhere/steam-scanner';

const client = new SteamClient({
  apiKey: process.env.STEAM_API_KEY!,
  steamId: process.env.STEAM_ID!,
});

// Get all owned games
const games = await client.getOwnedGames();
```

## Need Help?

- **Steam Web API Docs:** https://steamcommunity.com/dev
- **IGDB Forum:** https://steamcommunity.com/discussions/
- **Project Issues:** https://github.com/GreenFuze/MyGamesAnywhere/issues
