# @mygamesanywhere/gdrive-client

Google Drive client for MyGamesAnywhere - provides OAuth 2.0 authentication and file operations for cloud storage sync.

## Features

- 🔐 **OAuth 2.0**: Full Google OAuth 2.0 implementation with automatic token refresh
- 📁 **File Operations**: List, upload, download, update, delete files and folders
- 💾 **Token Storage**: File-based and in-memory token storage
- 🔄 **Auto-Refresh**: Automatic access token refresh before expiration
- 🛡️ **Type-Safe**: Full TypeScript support with Zod validation
- ⚡ **Error Handling**: Comprehensive error types with rate limit and quota handling
- ✅ **Well-Tested**: 41 tests with 100% pass rate

## Installation

```bash
npm install @mygamesanywhere/gdrive-client
```

## Prerequisites

For end users: **None!** Just run `npm run test:auth` and log in with Google.

For project maintainers: See [`GOOGLE_OAUTH_SETUP_MAINTAINER.md`](./GOOGLE_OAUTH_SETUP_MAINTAINER.md) to set up OAuth credentials.

## Quick Start (Simplified Authentication)

The easiest way to authenticate is using the `GDriveAuth` helper:

```typescript
import { authenticateGDrive, DriveClient } from '@mygamesanywhere/gdrive-client';

// Authenticate with Google Drive (opens browser)
const auth = await authenticateGDrive();

// Create client
const client = new DriveClient({
  oauth: { clientId: '', clientSecret: '', redirectUri: '' },
  tokenStorage: auth.getTokenStorage(),
});
(client as any).oauth = auth.getOAuthClient();

// Use the client
const files = await client.listFiles();
console.log(`Found ${files.files.length} files`);
```

**That's it!** The helper:
- Uses pre-configured OAuth credentials (set by project maintainer)
- Opens browser for Google login
- Saves tokens to `~/.mygamesanywhere/.gdrive-tokens.json`
- Auto-refreshes tokens when needed

## Quick Start (Manual OAuth Flow)

For advanced usage, you can manage OAuth manually:

```typescript
import { DriveClient, FileTokenStorage } from '@mygamesanywhere/gdrive-client';

// Configure OAuth and storage
const config = {
  oauth: {
    clientId: 'YOUR_CLIENT_ID',
    clientSecret: 'YOUR_CLIENT_SECRET',
    redirectUri: 'http://localhost:3000/oauth/callback',
  },
  tokenStorage: new FileTokenStorage('./.tokens.json'),
};

const client = new DriveClient(config);

// Get authorization URL for user
const authUrl = client.getOAuthClient().getAuthorizationUrl();
console.log(`Visit: ${authUrl}`);

// After user authorizes, exchange code for tokens
const code = 'AUTHORIZATION_CODE_FROM_CALLBACK';
await client.getOAuthClient().getTokenFromCode(code);

// Now you can use the client
const files = await client.listFiles();
console.log(`Found ${files.files.length} files`);
```

## Authentication Flow

### 1. Get Authorization URL

```typescript
import { DriveClient, FileTokenStorage } from '@mygamesanywhere/gdrive-client';

const client = new DriveClient({
  oauth: {
    clientId: process.env.GOOGLE_CLIENT_ID!,
    clientSecret: process.env.GOOGLE_CLIENT_SECRET!,
    redirectUri: 'http://localhost:3000/callback',
  },
  tokenStorage: new FileTokenStorage('./.tokens.json'),
});

const oauthClient = client.getOAuthClient();
const authUrl = oauthClient.getAuthorizationUrl('optional-state');

// Redirect user to authUrl
console.log('Visit:', authUrl);
```

### 2. Exchange Authorization Code

```typescript
// After user authorizes, Google redirects to your redirectUri with a code
const code = req.query.code; // From callback URL

await oauthClient.getTokenFromCode(code);
// Tokens are automatically saved to storage
```

### 3. Use Authenticated Client

```typescript
// Client automatically uses saved tokens
const files = await client.listFiles();
```

### 4. Token Refresh (Automatic)

The client automatically refreshes tokens when they expire:

```typescript
// This will automatically refresh if token is expired
const files = await client.listFiles(); // No manual refresh needed!
```

## File Operations

### List Files

```typescript
// List all files
const result = await client.listFiles();
console.log(result.files);

// List with query
const result = await client.listFiles({
  query: "mimeType='application/vnd.google-apps.folder'",
  pageSize: 10,
  orderBy: 'modifiedTime desc',
});

// List files in a folder
const result = await client.listFilesInFolder('folder_id');
```

### Upload File

```typescript
import { readFile } from 'fs/promises';

const content = await readFile('./document.txt');

const file = await client.uploadFile(content, {
  name: 'document.txt',
  mimeType: 'text/plain',
  parents: ['parent_folder_id'], // Optional
});

console.log(`Uploaded: ${file.id}`);
```

### Download File

```typescript
const content = await client.downloadFile('file_id');

// Save to disk
import { writeFile } from 'fs/promises';
await writeFile('./downloaded.txt', content);
```

### Get File Metadata

```typescript
const file = await client.getFile('file_id');

console.log(file.name);
console.log(file.size);
console.log(file.modifiedTime);
```

### Update File

```typescript
await client.updateFile('file_id', {
  name: 'new-name.txt',
  description: 'Updated description',
});
```

### Delete File

```typescript
await client.deleteFile('file_id');
```

### Create Folder

```typescript
const folder = await client.createFolder('MyFolder', 'parent_folder_id');
console.log(`Created folder: ${folder.id}`);
```

### Search Files

```typescript
// Search by name
const result = await client.searchByName('document.txt');

// Advanced search with Google Drive query syntax
const result = await client.listFiles({
  query: "name contains 'report' and mimeType='application/pdf'",
  orderBy: 'createdTime desc',
});
```

## Token Storage

### File-Based Storage

```typescript
import { FileTokenStorage } from '@mygamesanywhere/gdrive-client';

const storage = new FileTokenStorage('./.gdrive-tokens.json');

const client = new DriveClient({
  oauth: { /* ... */ },
  tokenStorage: storage,
});
```

### In-Memory Storage

```typescript
import { MemoryTokenStorage } from '@mygamesanywhere/gdrive-client';

const storage = new MemoryTokenStorage();

const client = new DriveClient({
  oauth: { /* ... */ },
  tokenStorage: storage,
});
```

### Custom Storage

Implement the `TokenStorage` interface:

```typescript
import type { TokenStorage, StoredTokens } from '@mygamesanywhere/gdrive-client';

class DatabaseTokenStorage implements TokenStorage {
  async saveTokens(tokens: StoredTokens): Promise<void> {
    // Save to database
  }

  async loadTokens(): Promise<StoredTokens | null> {
    // Load from database
  }

  async clearTokens(): Promise<void> {
    // Clear from database
  }
}
```

## Error Handling

```typescript
import {
  DriveClient,
  OAuth2Error,
  TokenExpiredError,
  AuthenticationError,
  FileNotFoundError,
  RateLimitError,
  QuotaExceededError,
  NetworkError,
} from '@mygamesanywhere/gdrive-client';

try {
  const files = await client.listFiles();
} catch (error) {
  if (error instanceof TokenExpiredError) {
    console.error('Token expired, please re-authenticate');
  } else if (error instanceof FileNotFoundError) {
    console.error('File not found:', error.fileId);
  } else if (error instanceof RateLimitError) {
    console.error('Rate limit exceeded, retry after:', error.retryAfter);
  } else if (error instanceof QuotaExceededError) {
    console.error('Storage quota exceeded');
  } else if (error instanceof NetworkError) {
    console.error('Network error:', error.originalError);
  } else if (error instanceof OAuth2Error) {
    console.error('OAuth error:', error.statusCode, error.response);
  }
}
```

## API Reference

### `DriveClient`

Main client class for Google Drive operations.

**Constructor:**
```typescript
new DriveClient(config: DriveClientConfig)
```

**Methods:**
- `getOAuthClient(): OAuth2Client` - Get OAuth client for authentication
- `listFiles(options?: SearchOptions): Promise<FileListResponse>` - List files
- `getFile(fileId: string): Promise<DriveFile>` - Get file metadata
- `uploadFile(content: Buffer | string, options: UploadOptions): Promise<DriveFile>` - Upload file
- `downloadFile(fileId: string): Promise<Buffer>` - Download file
- `updateFile(fileId: string, metadata: Partial<UploadOptions>): Promise<DriveFile>` - Update file
- `deleteFile(fileId: string): Promise<void>` - Delete file
- `createFolder(name: string, parentId?: string): Promise<DriveFile>` - Create folder
- `searchByName(name: string): Promise<FileListResponse>` - Search files by name
- `listFilesInFolder(folderId: string): Promise<FileListResponse>` - List files in folder

### `OAuth2Client`

OAuth 2.0 authentication client.

**Methods:**
- `getAuthorizationUrl(state?: string): string` - Get authorization URL
- `getTokenFromCode(code: string): Promise<StoredTokens>` - Exchange code for tokens
- `refreshAccessToken(refreshToken: string): Promise<StoredTokens>` - Refresh access token
- `getAccessToken(): Promise<string>` - Get valid access token (auto-refresh)
- `revokeToken(token: string): Promise<void>` - Revoke token
- `isAuthenticated(): Promise<boolean>` - Check if authenticated
- `logout(): Promise<void>` - Logout and clear tokens

## Google Drive Query Syntax

The `query` parameter supports Google Drive's search syntax:

```typescript
// Find folders
query: "mimeType='application/vnd.google-apps.folder'"

// Find files with name
query: "name='document.txt'"

// Find files containing text
query: "name contains 'report'"

// Multiple conditions
query: "name contains 'report' and mimeType='application/pdf'"

// Not trashed
query: "trashed=false"

// Modified after date
query: "modifiedTime > '2024-01-01T00:00:00'"

// In specific folder
query: "'FOLDER_ID' in parents"
```

## Testing

```bash
# Run tests
npm test

# Run tests in watch mode
npm run test:watch

# Run tests with coverage
npm run test:coverage
```

Test coverage:
- **41 tests** across 2 test suites
- Token storage tests (file and memory)
- Error handling tests
- All tests passing

## Security Notes

⚠️ **Important Security Considerations:**

1. **Never commit tokens or credentials to version control**
   - Add `.tokens.json` and `.env` to `.gitignore`

2. **Store secrets securely**
   - Use environment variables for client ID and secret
   - Use secure token storage in production

3. **Use HTTPS in production**
   - OAuth redirect URIs must use HTTPS in production

4. **Limit OAuth scopes**
   - Only request necessary scopes
   - Use `drive.file` scope instead of full `drive` access when possible

## OAuth Scopes

Default scopes:
- `https://www.googleapis.com/auth/drive.file` - Access files created by app
- `https://www.googleapis.com/auth/drive.appdata` - Access app data folder

Additional available scopes:
- `https://www.googleapis.com/auth/drive` - Full Drive access
- `https://www.googleapis.com/auth/drive.readonly` - Read-only access
- `https://www.googleapis.com/auth/drive.metadata.readonly` - Metadata only

Configure custom scopes:
```typescript
const config = {
  oauth: {
    clientId: '...',
    clientSecret: '...',
    redirectUri: '...',
    scopes: [
      'https://www.googleapis.com/auth/drive.file',
      'https://www.googleapis.com/auth/drive.appdata',
    ],
  },
};
```

## Part of MyGamesAnywhere

This package is part of the MyGamesAnywhere project - a cross-platform game launcher and manager with cloud sync.

- **GitHub:** https://github.com/GreenFuze/MyGamesAnywhere

## License

GPL-3.0

## Contributing

Contributions welcome! This is part of the larger MyGamesAnywhere project.

## Resources

- [Google Drive API Documentation](https://developers.google.com/drive/api/v3/about-sdk)
- [OAuth 2.0 Documentation](https://developers.google.com/identity/protocols/oauth2)
- [Google Cloud Console](https://console.cloud.google.com/)
