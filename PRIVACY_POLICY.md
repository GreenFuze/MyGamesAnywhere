# Privacy Policy for MyGamesAnywhere

**Effective Date:** October 4, 2025
**Last Updated:** October 4, 2025

## Overview

MyGamesAnywhere is a **serverless**, open-source game library manager. This Privacy Policy explains how we handle your data.

## No Server, No Data Collection

**MyGamesAnywhere does NOT have any servers.** All application functionality runs entirely on your device.

### What This Means:
- ✅ **No data is sent to our servers** (we don't have any!)
- ✅ **No user tracking or analytics** (unless you explicitly enable it)
- ✅ **No third-party data sharing** (there's nothing to share)
- ✅ **You own all your data** (it stays on your devices and your cloud storage)

## Data Storage

All data is stored in **two locations you control**:

### 1. Your Local Device
- Application configuration: `~/.mygamesanywhere/config.json`
- OAuth tokens: `~/.mygamesanywhere/.gdrive-tokens.json`
- Local cache: SQLite database on your device

### 2. Your Cloud Storage (Optional)
If you choose to use cloud sync:
- Game library data is stored in **YOUR** Google Drive/OneDrive
- Location: `.mygamesanywhere/` folder in your cloud storage
- Only you have access to this data

## Data We Access

### Google Drive (If You Authorize)
When you connect Google Drive:
- **What we access:** Files in your Google Drive (read-only)
- **Why:** To scan for games and sync your library
- **OAuth scope:** `drive.readonly` (read-only access)
- **How to revoke:** Go to [Google Account Permissions](https://myaccount.google.com/permissions) and remove MyGamesAnywhere

### Steam (If You Provide API Key)
When you connect Steam:
- **What we access:** Your Steam library via Steam Web API
- **How:** You provide your Steam API key (stored locally on your device)
- **We store:** API key encrypted on your device only

## What We DON'T Collect

- ❌ Personal information (name, email, address)
- ❌ Payment information
- ❌ Browsing history
- ❌ Device information (beyond what's needed for app to function)
- ❌ Analytics or telemetry (unless you opt-in)
- ❌ Crash reports (unless you choose to submit them)

## OAuth Authentication

When you authenticate with Google Drive:

1. **Authorization:** You authorize MyGamesAnywhere via Google's OAuth flow
2. **Token Storage:** OAuth tokens are stored locally on your device at `~/.mygamesanywhere/.gdrive-tokens.json`
3. **Token Usage:** Tokens are used only to access your Google Drive files
4. **Token Security:** Tokens never leave your device and are not shared with anyone
5. **Revocation:** You can revoke access anytime via your Google Account settings

## Third-Party Services

MyGamesAnywhere may connect to these third-party services **at your request**:

### Google Drive API
- **Purpose:** Cloud storage sync
- **Data accessed:** Files in your Google Drive
- **Privacy Policy:** [Google Privacy Policy](https://policies.google.com/privacy)

### Steam Web API
- **Purpose:** Game library sync
- **Data accessed:** Your Steam library and profile
- **Privacy Policy:** [Steam Privacy Policy](https://store.steampowered.com/privacy_agreement/)

### IGDB (Twitch) API
- **Purpose:** Game metadata (covers, descriptions)
- **Data accessed:** Public game information
- **Privacy Policy:** [Twitch Privacy Policy](https://www.twitch.tv/p/legal/privacy-notice/)

## Your Rights

Since all data is stored locally or in your cloud storage:

- ✅ **Right to Access:** You have direct access to all your data
- ✅ **Right to Delete:** Delete `~/.mygamesanywhere/` folder and cloud storage folder
- ✅ **Right to Export:** All data is in standard formats (JSON, SQLite)
- ✅ **Right to Revoke:** Revoke OAuth access via third-party service settings

## Children's Privacy

MyGamesAnywhere does not knowingly collect data from children under 13. Since we don't collect any data, this is not applicable.

## Changes to Privacy Policy

We may update this Privacy Policy. Changes will be posted at:
- GitHub: https://github.com/GreenFuze/MyGamesAnywhere/blob/main/PRIVACY_POLICY.md

## Open Source

MyGamesAnywhere is open-source software. You can review exactly how your data is handled by inspecting the source code:
- GitHub: https://github.com/GreenFuze/MyGamesAnywhere

## Contact

Questions about privacy?
- GitHub Issues: https://github.com/GreenFuze/MyGamesAnywhere/issues
- Email: [Your Contact Email]

## Summary

**TL;DR:**
- We have no servers
- No data collection
- Everything stays on your device and your cloud storage
- You own and control all your data
- Open source - verify yourself!

---

**MyGamesAnywhere is provided as-is with no warranties. The developers are not responsible for any data loss or damage.**
