/**
 * @mygamesanywhere/gdrive-client
 *
 * Google Drive client for MyGamesAnywhere
 * Provides OAuth 2.0 authentication and file operations
 */

// Main client
export { DriveClient } from './drive-client.js';
export { OAuth2Client } from './oauth-client.js';

// Authentication helper (simplified OAuth flow)
export { GDriveAuth, authenticateGDrive } from './gdrive-auth.js';

// Token storage implementations
export { FileTokenStorage, MemoryTokenStorage } from './token-storage.js';

// Types
export type {
  OAuth2Config,
  OAuth2Tokens,
  StoredTokens,
  DriveFile,
  FileListResponse,
  UploadOptions,
  DownloadOptions,
  SearchOptions,
  DriveClientConfig,
  TokenStorage,
} from './types.js';

export {
  OAuth2ConfigSchema,
  OAuth2TokensSchema,
  StoredTokensSchema,
  DriveFileSchema,
  FileListResponseSchema,
  isStoredTokens,
  isDriveFile,
  isFileListResponse,
} from './types.js';

// Errors
export {
  DriveClientError,
  OAuth2Error,
  TokenExpiredError,
  AuthenticationError,
  DriveAPIError,
  FileNotFoundError,
  NetworkError,
  InvalidConfigError,
  RateLimitError,
  QuotaExceededError,
} from './errors.js';
