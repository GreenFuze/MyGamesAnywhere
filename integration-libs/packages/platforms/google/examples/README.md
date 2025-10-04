# Google Drive Client Examples

This directory contains practical examples demonstrating how to use `@mygamesanywhere/gdrive-client`.

## Prerequisites

Before running these examples, you need to:

1. **Create Google Cloud Project**
   - Go to [Google Cloud Console](https://console.cloud.google.com/)
   - Create a new project or select existing one

2. **Enable Google Drive API**
   - In your project, go to "APIs & Services" > "Library"
   - Search for "Google Drive API"
   - Click "Enable"

3. **Create OAuth 2.0 Credentials**
   - Go to "APIs & Services" > "Credentials"
   - Click "Create Credentials" > "OAuth client ID"
   - Choose "Web application"
   - Add authorized redirect URI: `http://localhost:3000/oauth/callback`
   - Save your Client ID and Client Secret

4. **Set Environment Variables**
   ```bash
   export GOOGLE_CLIENT_ID="your-client-id"
   export GOOGLE_CLIENT_SECRET="your-client-secret"
   ```

   Or create a `.env` file:
   ```
   GOOGLE_CLIENT_ID=your-client-id
   GOOGLE_CLIENT_SECRET=your-client-secret
   ```

## Running the Examples

### Prerequisites

```bash
# Install dependencies
npm install

# Build the package
npm run build
```

### Example 1: OAuth Flow

**File:** `basic-oauth-flow.ts`

**What it demonstrates:**
- Complete OAuth 2.0 authentication flow
- Starting a local server to receive callback
- Exchanging authorization code for tokens
- Saving tokens to file
- Testing authenticated requests

**Run it:**
```bash
npx tsx examples/basic-oauth-flow.ts
```

**Steps:**
1. Script will print an authorization URL
2. Open the URL in your browser
3. Sign in with your Google account
4. Authorize the application
5. You'll be redirected back to localhost
6. Tokens will be saved to `./.gdrive-tokens.json`
7. Script will test the connection by listing files

---

### Example 2: File Operations

**File:** `file-operations.ts`

**What it demonstrates:**
- Listing files and folders
- Uploading files
- Downloading files
- Updating file metadata
- Creating folders
- Uploading files to folders
- Searching files
- Deleting files

**Run it:**
```bash
# Make sure you've authenticated first!
npx tsx examples/file-operations.ts
```

**Requirements:**
- You must run `basic-oauth-flow.ts` first to authenticate
- Tokens must be saved in `./.gdrive-tokens.json`

---

## Example Walkthrough

### Step 1: Authenticate

```typescript
import { DriveClient, FileTokenStorage } from '@mygamesanywhere/gdrive-client';

const client = new DriveClient({
  oauth: {
    clientId: process.env.GOOGLE_CLIENT_ID!,
    clientSecret: process.env.GOOGLE_CLIENT_SECRET!,
    redirectUri: 'http://localhost:3000/oauth/callback',
  },
  tokenStorage: new FileTokenStorage('./.gdrive-tokens.json'),
});

// Get authorization URL
const authUrl = client.getOAuthClient().getAuthorizationUrl();
console.log('Visit:', authUrl);

// After user authorizes, exchange code
const code = '...'; // From callback URL
await client.getOAuthClient().getTokenFromCode(code);
```

### Step 2: Use the Client

```typescript
// List files
const files = await client.listFiles();

// Upload file
const file = await client.uploadFile(Buffer.from('Hello!'), {
  name: 'test.txt',
  mimeType: 'text/plain',
});

// Download file
const content = await client.downloadFile(file.id);

// Delete file
await client.deleteFile(file.id);
```

## Common Patterns

### Check if Authenticated

```typescript
const isAuth = await client.getOAuthClient().isAuthenticated();
if (!isAuth) {
  console.log('Please authenticate first');
  process.exit(1);
}
```

### Upload from File

```typescript
import { readFile } from 'fs/promises';

const content = await readFile('./document.pdf');
const file = await client.uploadFile(content, {
  name: 'document.pdf',
  mimeType: 'application/pdf',
});
```

### Download to File

```typescript
import { writeFile } from 'fs/promises';

const content = await client.downloadFile(fileId);
await writeFile('./downloaded.pdf', content);
```

### List Files with Query

```typescript
// Find all PDFs
const pdfs = await client.listFiles({
  query: "mimeType='application/pdf'",
  orderBy: 'modifiedTime desc',
  pageSize: 50,
});

// Find files modified today
const today = new Date().toISOString().split('T')[0];
const recentFiles = await client.listFiles({
  query: `modifiedTime > '${today}T00:00:00'`,
});

// Find files in specific folder
const folderFiles = await client.listFilesInFolder(folderId);
```

### Handle Pagination

```typescript
let allFiles = [];
let pageToken;

do {
  const result = await client.listFiles({
    pageSize: 100,
    pageToken,
  });

  allFiles.push(...result.files);
  pageToken = result.nextPageToken;
} while (pageToken);

console.log(`Total files: ${allFiles.length}`);
```

### Error Handling

```typescript
import {
  FileNotFoundError,
  RateLimitError,
  QuotaExceededError,
  NetworkError,
} from '@mygamesanywhere/gdrive-client';

try {
  await client.downloadFile(fileId);
} catch (error) {
  if (error instanceof FileNotFoundError) {
    console.error('File not found:', error.fileId);
  } else if (error instanceof RateLimitError) {
    console.error('Rate limited, retry after:', error.retryAfter);
    // Wait and retry
  } else if (error instanceof QuotaExceededError) {
    console.error('Out of storage space');
  } else if (error instanceof NetworkError) {
    console.error('Network error:', error.originalError);
  }
}
```

## Troubleshooting

### "Not authenticated" error
- Run `basic-oauth-flow.ts` first to authenticate
- Make sure `.gdrive-tokens.json` exists and is valid

### "Token expired" error
- The client should automatically refresh tokens
- If it doesn't work, delete `.gdrive-tokens.json` and re-authenticate

### "Invalid redirect URI" error
- Make sure `http://localhost:3000/oauth/callback` is added to authorized redirect URIs in Google Cloud Console
- Make sure the redirect URI in your code matches exactly

### "Access denied" error
- Make sure you've authorized the application
- Check that you're using the correct Google account
- Verify OAuth scopes in Google Cloud Console

## Security Notes

⚠️ **Important:**
- Never commit `.gdrive-tokens.json` to version control
- Never commit `.env` with your credentials
- Add these to `.gitignore`:
  ```
  .gdrive-tokens.json
  .env
  ```

## Next Steps

After running these examples:

1. Read the main [README.md](../README.md) for full API documentation
2. Check out the [tests](../tests/) for more usage examples
3. Explore the [source code](../src/) to understand implementation
4. Build your own cloud storage features!

## Need Help?

- Check the [Google Drive API Documentation](https://developers.google.com/drive/api/v3/about-sdk)
- Review [OAuth 2.0 Guide](https://developers.google.com/identity/protocols/oauth2)
- Open an issue on GitHub

## Contributing

Have a useful example? Feel free to contribute by opening a pull request!
