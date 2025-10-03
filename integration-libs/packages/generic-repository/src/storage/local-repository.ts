/**
 * Local Filesystem Repository
 * Implements repository adapter for local directories
 */

import { readdir, stat, access } from 'fs/promises';
import { join, resolve, basename } from 'path';
import { constants } from 'fs';
import { BaseRepositoryAdapter } from './repository-adapter.js';
import { RepositoryType, type FileInfo } from '../types.js';

/**
 * Local filesystem repository adapter
 */
export class LocalRepository extends BaseRepositoryAdapter {
  type: RepositoryType = RepositoryType.LOCAL;
  private rootPath: string;

  /**
   * Create a new local repository
   * @param rootPath Root directory path
   */
  constructor(rootPath: string) {
    super();
    this.rootPath = resolve(rootPath);
  }

  /**
   * List all files in a directory
   */
  async listFiles(path: string): Promise<string[]> {
    const fullPath = this.resolvePath(path);

    try {
      const entries = await readdir(fullPath, { withFileTypes: true });
      return entries.map((entry) => join(path, entry.name));
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === 'ENOENT') {
        return [];
      }
      throw error;
    }
  }

  /**
   * Get file information
   */
  async getFileInfo(path: string): Promise<FileInfo> {
    const fullPath = this.resolvePath(path);
    const stats = await stat(fullPath);

    return {
      path,
      name: basename(fullPath),
      size: stats.size,
      isDirectory: stats.isDirectory(),
      modifiedAt: stats.mtime,
      extension: this.getExtension(path),
    };
  }

  /**
   * Check if path exists
   */
  async exists(path: string): Promise<boolean> {
    const fullPath = this.resolvePath(path);

    try {
      await access(fullPath, constants.F_OK);
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Check if path is a directory
   */
  async isDirectory(path: string): Promise<boolean> {
    const fullPath = this.resolvePath(path);

    try {
      const stats = await stat(fullPath);
      return stats.isDirectory();
    } catch {
      return false;
    }
  }

  /**
   * Download file to temp
   * For local files, just return the full path since no download is needed
   */
  async downloadToTemp(path: string): Promise<string> {
    return this.resolvePath(path);
  }

  /**
   * Get file size
   */
  async getSize(path: string): Promise<number> {
    const fullPath = this.resolvePath(path);
    const stats = await stat(fullPath);
    return stats.size;
  }

  /**
   * Get the root path
   */
  getRootPath(): string {
    return this.rootPath;
  }

  /**
   * Resolve relative path to absolute path
   */
  private resolvePath(relativePath: string): string {
    // If it's already absolute and within root, return it
    const absolutePath = resolve(relativePath);
    if (absolutePath.startsWith(this.rootPath)) {
      return absolutePath;
    }

    // Otherwise, join with root
    return join(this.rootPath, relativePath);
  }
}
