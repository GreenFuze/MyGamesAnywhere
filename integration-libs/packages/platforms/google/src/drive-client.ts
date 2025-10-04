/**
 * Google Drive Client
 * Provides file operations on Google Drive
 */

import type {
  DriveClientConfig,
  DriveFile,
  FileListResponse,
  UploadOptions,
  SearchOptions,
} from './types.js';
import { OAuth2Client } from './oauth-client.js';
import {
  DriveAPIError,
  FileNotFoundError,
  NetworkError,
  RateLimitError,
  QuotaExceededError,
} from './errors.js';

/**
 * Google Drive Client
 * Main class for interacting with Google Drive API
 */
export class DriveClient {
  private static readonly API_BASE = 'https://www.googleapis.com/drive/v3';
  private static readonly UPLOAD_BASE =
    'https://www.googleapis.com/upload/drive/v3';

  private oauth: OAuth2Client;

  constructor(config: DriveClientConfig) {
    this.oauth = new OAuth2Client(config.oauth, config.tokenStorage);
  }

  /**
   * Get OAuth client for authentication
   */
  getOAuthClient(): OAuth2Client {
    return this.oauth;
  }

  /**
   * List files in Google Drive
   */
  async listFiles(options: SearchOptions = {}): Promise<FileListResponse> {
    const params = new URLSearchParams({
      fields:
        options.fields ||
        'files(id,name,mimeType,size,createdTime,modifiedTime,parents,trashed),nextPageToken',
      pageSize: (options.pageSize || 100).toString(),
    });

    if (options.query) {
      params.set('q', options.query);
    }

    if (options.pageToken) {
      params.set('pageToken', options.pageToken);
    }

    if (options.orderBy) {
      params.set('orderBy', options.orderBy);
    }

    const url = `${DriveClient.API_BASE}/files?${params.toString()}`;

    try {
      const response = await this.authenticatedRequest(url, {
        method: 'GET',
      });

      const data = (await response.json()) as FileListResponse;
      return data;
    } catch (error) {
      this.handleError(error, 'Failed to list files');
      throw error; // TypeScript needs this
    }
  }

  /**
   * Get file metadata by ID
   */
  async getFile(fileId: string, fields?: string): Promise<DriveFile> {
    const params = new URLSearchParams({
      fields:
        fields ||
        'id,name,mimeType,size,createdTime,modifiedTime,parents,trashed',
    });

    const url = `${DriveClient.API_BASE}/files/${fileId}?${params.toString()}`;

    try {
      const response = await this.authenticatedRequest(url, {
        method: 'GET',
      });

      if (response.status === 404) {
        throw new FileNotFoundError(`File not found: ${fileId}`, fileId);
      }

      const data = (await response.json()) as DriveFile;
      return data;
    } catch (error) {
      this.handleError(error, `Failed to get file ${fileId}`);
      throw error;
    }
  }

  /**
   * Upload a file to Google Drive
   */
  async uploadFile(
    content: Buffer | string,
    options: UploadOptions
  ): Promise<DriveFile> {
    const metadata = {
      name: options.name,
      mimeType: options.mimeType,
      parents: options.parents,
      description: options.description,
    };

    const boundary = '-------boundary' + Date.now();
    const delimiter = `\r\n--${boundary}\r\n`;
    const closeDelimiter = `\r\n--${boundary}--`;

    const multipartBody =
      delimiter +
      'Content-Type: application/json; charset=UTF-8\r\n\r\n' +
      JSON.stringify(metadata) +
      delimiter +
      `Content-Type: ${options.mimeType || 'application/octet-stream'}\r\n\r\n` +
      (content instanceof Buffer ? content.toString('base64') : content) +
      closeDelimiter;

    const url = `${DriveClient.UPLOAD_BASE}/files?uploadType=multipart`;

    try {
      const response = await this.authenticatedRequest(url, {
        method: 'POST',
        headers: {
          'Content-Type': `multipart/related; boundary=${boundary}`,
        },
        body: multipartBody,
      });

      const data = (await response.json()) as DriveFile;
      return data;
    } catch (error) {
      this.handleError(error, `Failed to upload file ${options.name}`);
      throw error;
    }
  }

  /**
   * Download file content from Google Drive
   */
  async downloadFile(fileId: string): Promise<Buffer> {
    const url = `${DriveClient.API_BASE}/files/${fileId}?alt=media`;

    try {
      const response = await this.authenticatedRequest(url, {
        method: 'GET',
      });

      if (response.status === 404) {
        throw new FileNotFoundError(`File not found: ${fileId}`, fileId);
      }

      const arrayBuffer = await response.arrayBuffer();
      return Buffer.from(arrayBuffer);
    } catch (error) {
      this.handleError(error, `Failed to download file ${fileId}`);
      throw error;
    }
  }

  /**
   * Update file metadata
   */
  async updateFile(
    fileId: string,
    metadata: Partial<UploadOptions>
  ): Promise<DriveFile> {
    const url = `${DriveClient.API_BASE}/files/${fileId}`;

    try {
      const response = await this.authenticatedRequest(url, {
        method: 'PATCH',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(metadata),
      });

      if (response.status === 404) {
        throw new FileNotFoundError(`File not found: ${fileId}`, fileId);
      }

      const data = (await response.json()) as DriveFile;
      return data;
    } catch (error) {
      this.handleError(error, `Failed to update file ${fileId}`);
      throw error;
    }
  }

  /**
   * Delete a file from Google Drive
   */
  async deleteFile(fileId: string): Promise<void> {
    const url = `${DriveClient.API_BASE}/files/${fileId}`;

    try {
      const response = await this.authenticatedRequest(url, {
        method: 'DELETE',
      });

      if (response.status === 404) {
        throw new FileNotFoundError(`File not found: ${fileId}`, fileId);
      }

      if (response.status !== 204 && response.status !== 200) {
        throw new DriveAPIError(
          `Failed to delete file: ${response.statusText}`,
          response.status
        );
      }
    } catch (error) {
      this.handleError(error, `Failed to delete file ${fileId}`);
      throw error;
    }
  }

  /**
   * Create a folder in Google Drive
   */
  async createFolder(name: string, parentId?: string): Promise<DriveFile> {
    const metadata = {
      name,
      mimeType: 'application/vnd.google-apps.folder',
      ...(parentId ? { parents: [parentId] } : {}),
    };

    const url = `${DriveClient.API_BASE}/files`;

    try {
      const response = await this.authenticatedRequest(url, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(metadata),
      });

      const data = (await response.json()) as DriveFile;
      return data;
    } catch (error) {
      this.handleError(error, `Failed to create folder ${name}`);
      throw error;
    }
  }

  /**
   * Search for files by name
   */
  async searchByName(
    name: string,
    options: Omit<SearchOptions, 'query'> = {}
  ): Promise<FileListResponse> {
    const query = `name='${name.replace(/'/g, "\\'")}'`;
    return this.listFiles({ ...options, query });
  }

  /**
   * Search for files in a specific folder
   */
  async listFilesInFolder(
    folderId: string,
    options: Omit<SearchOptions, 'query'> = {}
  ): Promise<FileListResponse> {
    const query = `'${folderId}' in parents and trashed=false`;
    return this.listFiles({ ...options, query });
  }

  /**
   * Make an authenticated request to Google Drive API
   */
  private async authenticatedRequest(
    url: string,
    options: RequestInit = {}
  ): Promise<Response> {
    try {
      const accessToken = await this.oauth.getAccessToken();

      const response = await fetch(url, {
        ...options,
        headers: {
          ...options.headers,
          Authorization: `Bearer ${accessToken}`,
        },
      });

      // Handle rate limiting
      if (response.status === 429) {
        const retryAfter = response.headers.get('Retry-After');
        throw new RateLimitError(
          'Rate limit exceeded',
          retryAfter ? parseInt(retryAfter) : undefined
        );
      }

      // Handle quota exceeded
      if (response.status === 403) {
        const errorResponse = (await response
          .json()
          .catch(() => ({}))) as { error?: { errors?: Array<{ reason?: string }> } };
        if (
          errorResponse.error?.errors?.some(
            (e) => e.reason === 'quotaExceeded'
          )
        ) {
          throw new QuotaExceededError();
        }
      }

      return response;
    } catch (error) {
      if (
        error instanceof RateLimitError ||
        error instanceof QuotaExceededError
      ) {
        throw error;
      }
      throw new NetworkError(
        `Network request failed: ${
          error instanceof Error ? error.message : 'Unknown error'
        }`,
        error instanceof Error ? error : undefined
      );
    }
  }

  /**
   * Handle errors and throw appropriate error types
   */
  private handleError(error: unknown, context: string): never {
    if (
      error instanceof DriveAPIError ||
      error instanceof FileNotFoundError ||
      error instanceof NetworkError ||
      error instanceof RateLimitError ||
      error instanceof QuotaExceededError
    ) {
      throw error;
    }

    if (error instanceof Error) {
      throw new DriveAPIError(
        `${context}: ${error.message}`,
        undefined,
        error
      );
    }

    throw new DriveAPIError(
      `${context}: ${typeof error === 'object' && error !== null ? JSON.stringify(error) : String(error)}`
    );
  }
}
