/**
 * Google Drive Repository
 * Implements repository adapter for Google Drive storage
 */

import { DriveClient } from '@mygamesanywhere/gdrive-client';
import type { DriveFile } from '@mygamesanywhere/gdrive-client';
import { BaseRepositoryAdapter } from './repository-adapter.js';
import { RepositoryType, type FileInfo } from '../types.js';
import { tmpdir } from 'os';
import { join } from 'path';
import { writeFile } from 'fs/promises';

/**
 * Google Drive repository adapter
 */
export class GDriveRepository extends BaseRepositoryAdapter {
  type: RepositoryType = RepositoryType.GDRIVE;
  private client: DriveClient;
  private rootFolderId?: string;

  /**
   * Create a new Google Drive repository
   * @param client Authenticated DriveClient
   * @param rootFolderId Optional root folder ID (defaults to "root")
   */
  constructor(client: DriveClient, rootFolderId?: string) {
    super();
    this.client = client;
    this.rootFolderId = rootFolderId;
  }

  /**
   * List all files in a folder
   * Path can be either folder ID or relative path
   */
  async listFiles(path: string): Promise<string[]> {
    try {
      // If path is empty, use root
      const folderId = path || this.rootFolderId || 'root';

      const result = await this.client.listFilesInFolder(folderId);

      // Return file paths (using file IDs as paths)
      return result.files.map((file) => file.id);
    } catch (error) {
      console.error(`Error listing files in ${path}:`, error);
      return [];
    }
  }

  /**
   * Get file information
   */
  async getFileInfo(fileId: string): Promise<FileInfo> {
    // If fileId is empty, use root folder
    const actualFileId = fileId || this.rootFolderId || 'root';
    const file = await this.client.getFile(actualFileId);

    return {
      path: actualFileId,
      name: file.name || 'unnamed',
      size: parseInt(file.size || '0'),
      isDirectory: file.mimeType === 'application/vnd.google-apps.folder',
      modifiedAt: new Date(file.modifiedTime || Date.now()),
      extension: this.getExtension(file.name),
    };
  }

  /**
   * Check if path (file ID) exists
   */
  async exists(fileId: string): Promise<boolean> {
    try {
      const actualFileId = fileId || this.rootFolderId || 'root';
      await this.client.getFile(actualFileId);
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Check if path is a directory (folder)
   */
  async isDirectory(fileId: string): Promise<boolean> {
    try {
      const actualFileId = fileId || this.rootFolderId || 'root';
      const file = await this.client.getFile(actualFileId);
      return file.mimeType === 'application/vnd.google-apps.folder';
    } catch {
      return false;
    }
  }

  /**
   * Download file to temporary location
   */
  async downloadToTemp(fileId: string): Promise<string> {
    const file = await this.client.getFile(fileId);
    const content = await this.client.downloadFile(fileId);

    // Create temp file path
    const tempPath = join(tmpdir(), `gdrive-${fileId}-${file.name}`);

    // Write to temp file
    await writeFile(tempPath, content);

    return tempPath;
  }

  /**
   * Get file size
   */
  async getSize(fileId: string): Promise<number> {
    const file = await this.client.getFile(fileId);
    return parseInt(file.size || '0');
  }

  /**
   * Search for files by name
   */
  async searchByName(name: string): Promise<DriveFile[]> {
    const result = await this.client.searchByName(name);
    return result.files;
  }

  /**
   * Get folder ID by path (for navigation)
   * This is a helper for path-based navigation
   */
  async getFolderIdByPath(path: string): Promise<string | null> {
    // If path is already a file ID, return it
    if (path.match(/^[a-zA-Z0-9_-]{20,}$/)) {
      return path;
    }

    // Otherwise, search for folder by name
    const parts = path.split('/').filter(Boolean);
    let currentFolderId = this.rootFolderId || 'root';

    for (const folderName of parts) {
      const result = await this.client.listFiles({
        query: `'${currentFolderId}' in parents and name='${folderName}' and mimeType='application/vnd.google-apps.folder'`,
        pageSize: 1,
      });

      if (result.files.length === 0) {
        return null;
      }

      currentFolderId = result.files[0].id;
    }

    return currentFolderId;
  }
}
