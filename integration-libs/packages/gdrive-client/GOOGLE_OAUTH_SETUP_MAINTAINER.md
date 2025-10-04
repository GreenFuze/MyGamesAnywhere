# Google Drive OAuth Setup Guide (For Project Maintainers)

This guide is for **project maintainers** who need to set up Google OAuth credentials for the MyGamesAnywhere project.

**End users do NOT need to follow this guide.** They can simply run `npm run test:auth` once you've set up the credentials.

## Prerequisites

- A Google account (preferably a project/organization account)
- Access to Google Cloud Console
- Ability to set environment variables or project secrets

## Step 1: Create Google Cloud Project

1. **Go to Google Cloud Console**
   - Visit: https://console.cloud.google.com/

2. **Create a new project** (or select existing)
   - Click the project dropdown at the top
   - Click "New Project"
   - Name: `MyGamesAnywhere` (or any name)
   - Click "Create"

3. **Wait for project creation**
   - Takes a few seconds
   - You'll see a notification when ready

## Step 2: Enable Google Drive API

1. **Navigate to APIs & Services**
   - From the left menu: "APIs & Services" → "Library"
   - Or visit: https://console.cloud.google.com/apis/library

2. **Search for Google Drive API**
   - Type "Google Drive API" in the search box
   - Click on "Google Drive API" from results

3. **Enable the API**
   - Click "Enable" button
   - Wait for it to activate (takes a few seconds)

## Step 3: Configure OAuth Consent Screen

1. **Go to OAuth consent screen**
   - From left menu: "APIs & Services" → "OAuth consent screen"
   - Or visit: https://console.cloud.google.com/apis/credentials/consent

2. **Choose User Type**
   - Select "External" (for testing with any Google account)
   - Click "Create"

3. **Fill App Information**
   - **App name:** `MyGamesAnywhere`
   - **User support email:** Your email
   - **Developer contact email:** Your email
   - Leave other fields as default
   - Click "Save and Continue"

4. **Scopes (Step 2)**
   - Click "Add or Remove Scopes"
   - Search for "Google Drive API"
   - Select these scopes:
     - `.../auth/drive.file` - See, edit, create, and delete only the specific Google Drive files you use with this app
     - `.../auth/drive.appdata` - See, create, and delete its own configuration data in your Google Drive
   - Click "Update"
   - Click "Save and Continue"

5. **Test Users (Step 3)**
   - Click "+ Add Users"
   - Enter your Google email address
   - Click "Add"
   - Click "Save and Continue"

6. **Summary (Step 4)**
   - Review and click "Back to Dashboard"

## Step 4: Create OAuth Credentials

1. **Go to Credentials**
   - From left menu: "APIs & Services" → "Credentials"
   - Or visit: https://console.cloud.google.com/apis/credentials

2. **Create OAuth Client ID**
   - Click "+ Create Credentials" at top
   - Select "OAuth client ID"

3. **Configure OAuth Client**
   - **Application type:** Desktop app
   - **Name:** `MyGamesAnywhere Desktop Client`
   - Click "Create"

4. **Copy Your Credentials**
   - A dialog will appear with your credentials
   - **Copy Client ID** - looks like: `123456789-abc123.apps.googleusercontent.com`
   - **Copy Client Secret** - looks like: `GOCSPX-abc123def456ghi789`
   - Click "OK"

## Step 5: Configure Credentials

### For Development (Local Testing)

Set environment variables:

**Windows (PowerShell):**
```powershell
$env:GOOGLE_CLIENT_ID="123456789-abc123.apps.googleusercontent.com"
$env:GOOGLE_CLIENT_SECRET="GOCSPX-abc123def456ghi789"
```

**macOS/Linux:**
```bash
export GOOGLE_CLIENT_ID="123456789-abc123.apps.googleusercontent.com"
export GOOGLE_CLIENT_SECRET="GOCSPX-abc123def456ghi789"
```

### For Production (CI/CD)

Add these as **secrets** in your CI/CD environment:
- `GOOGLE_CLIENT_ID`
- `GOOGLE_CLIENT_SECRET`

The application will automatically pick them up from environment variables.

### Security Best Practices

⚠️ **NEVER commit these credentials to Git!**

The credentials are embedded in the application using environment variables at build/runtime:
- Development: Set locally via environment variables
- Production: Set via CI/CD secrets or deployment environment
- The `gdrive-auth.ts` file reads from `process.env.GOOGLE_CLIENT_ID` and `process.env.GOOGLE_CLIENT_SECRET`

## Step 6: Test Authentication

Once credentials are set, test the authentication flow:

```bash
cd integration-libs/packages/gdrive-client
npm run test:auth
```

This will:
1. Open a browser window
2. Ask you to authorize the app
3. Save tokens to `~/.mygamesanywhere/.gdrive-tokens.json`
4. List files in your Google Drive

**Expected output:**
```
=== Google Drive Authentication Test ===

🔐 Starting Google Drive authentication...

🌐 OAuth callback server started on http://localhost:3000
📋 Opening browser for authentication...
⏳ Waiting for authorization...

✅ Authentication complete!

📂 Testing Google Drive access...
✅ Found X files
...
```

## Troubleshooting

### Error: "Access blocked: This app's request is invalid"

**Solution:** Make sure you added yourself as a test user in the OAuth consent screen.

### Error: "redirect_uri_mismatch"

**Solution:**
1. Go to Credentials in Google Cloud Console
2. Click on your OAuth Client ID
3. Under "Authorized redirect URIs", add:
   - `http://localhost:3000/oauth/callback`
   - `http://localhost:3000/callback`
4. Click "Save"

### Error: "invalid_grant" or "Token has been expired or revoked"

**Solution:** Delete `.tokens.json` and re-authenticate:

```bash
rm .tokens.json
npm run test:auth
```

## Security Notes

⚠️ **Important:**

1. **Never commit credentials to Git**
   - `.tokens.json` is already in `.gitignore`
   - Never share your Client Secret

2. **Rotate credentials if compromised**
   - Go to Google Cloud Console → Credentials
   - Delete old OAuth Client ID
   - Create new one

3. **For production:**
   - Use verified app (submit for verification)
   - Use HTTPS for redirect URIs
   - Store credentials in OS keychain

## What's Next?

Once authenticated, you can:

1. **Test Google Drive integration**
   ```bash
   npm run test:gdrive
   ```

2. **Scan Google Drive for games**
   ```bash
   npm run test:scan-gdrive
   ```

3. **Use in your app**
   ```typescript
   import { DriveClient } from '@mygamesanywhere/gdrive-client';
   // Client will use saved tokens from .tokens.json
   ```

## Links

- [Google Cloud Console](https://console.cloud.google.com/)
- [Google Drive API Docs](https://developers.google.com/drive/api/v3/about-sdk)
- [OAuth 2.0 Docs](https://developers.google.com/identity/protocols/oauth2)
