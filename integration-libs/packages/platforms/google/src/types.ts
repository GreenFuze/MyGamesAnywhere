/**
 * Type definitions for Google Drive client
 */

import { z } from 'zod';

/**
 * OAuth 2.0 Configuration
 */
export interface OAuth2Config {
  clientId: string;
  clientSecret: string;
  redirectUri: string;
  scopes?: string[];
}

/**
 * OAuth 2.0 Token Response
 */
export interface OAuth2Tokens {
  access_token: string;
  refresh_token?: string;
  expires_in: number;
  token_type: string;
  scope?: string;
}

/**
 * Stored token data with expiration
 */
export interface StoredTokens extends OAuth2Tokens {
  expiresAt: number; // Unix timestamp
}

/**
 * Google Drive File Metadata
 */
export interface DriveFile {
  id: string;
  name: string;
  mimeType: string;
  size?: string;
  createdTime?: string;
  modifiedTime?: string;
  parents?: string[];
  trashed?: boolean;
}

/**
 * File list response from Google Drive
 */
export interface FileListResponse {
  files: DriveFile[];
  nextPageToken?: string;
  incompleteSearch?: boolean;
}

/**
 * File upload options
 */
export interface UploadOptions {
  name: string;
  mimeType?: string;
  parents?: string[];
  description?: string;
}

/**
 * File download options
 */
export interface DownloadOptions {
  fileId: string;
  alt?: 'media' | 'json';
}

/**
 * File search query options
 */
export interface SearchOptions {
  query?: string;
  pageSize?: number;
  pageToken?: string;
  orderBy?: string;
  fields?: string;
}

/**
 * Configuration for Google Drive client
 */
export interface DriveClientConfig {
  oauth: OAuth2Config;
  tokenStorage?: TokenStorage;
}

/**
 * Token storage interface - allows custom storage implementations
 */
export interface TokenStorage {
  saveTokens(tokens: StoredTokens): Promise<void>;
  loadTokens(): Promise<StoredTokens | null>;
  clearTokens(): Promise<void>;
}

/**
 * Zod schemas for runtime validation
 */

export const OAuth2ConfigSchema = z.object({
  clientId: z.string().min(1),
  clientSecret: z.string().min(1),
  redirectUri: z.string().url(),
  scopes: z.array(z.string()).optional(),
});

export const OAuth2TokensSchema = z.object({
  access_token: z.string(),
  refresh_token: z.string().optional(),
  expires_in: z.number(),
  token_type: z.string(),
  scope: z.string().optional(),
});

export const StoredTokensSchema = OAuth2TokensSchema.extend({
  expiresAt: z.number(),
});

export const DriveFileSchema = z.object({
  id: z.string(),
  name: z.string(),
  mimeType: z.string(),
  size: z.string().optional(),
  createdTime: z.string().optional(),
  modifiedTime: z.string().optional(),
  parents: z.array(z.string()).optional(),
  trashed: z.boolean().optional(),
});

export const FileListResponseSchema = z.object({
  files: z.array(DriveFileSchema),
  nextPageToken: z.string().optional(),
  incompleteSearch: z.boolean().optional(),
});

/**
 * Type guards
 */

export function isStoredTokens(value: unknown): value is StoredTokens {
  return StoredTokensSchema.safeParse(value).success;
}

export function isDriveFile(value: unknown): value is DriveFile {
  return DriveFileSchema.safeParse(value).success;
}

export function isFileListResponse(value: unknown): value is FileListResponse {
  return FileListResponseSchema.safeParse(value).success;
}
