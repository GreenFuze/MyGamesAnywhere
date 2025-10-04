/**
 * Abstract Repository Adapter
 * Provides a common interface for local and cloud storage
 */

import type { RepositoryAdapter, RepositoryType, FileInfo } from '../types.js';

/**
 * Base repository adapter class
 * Extend this for specific storage implementations
 */
export abstract class BaseRepositoryAdapter implements RepositoryAdapter {
  abstract type: RepositoryType;

  /**
   * List all files in a directory
   */
  abstract listFiles(path: string): Promise<string[]>;

  /**
   * Get file information
   */
  abstract getFileInfo(path: string): Promise<FileInfo>;

  /**
   * Check if path exists
   */
  abstract exists(path: string): Promise<boolean>;

  /**
   * Check if path is a directory
   */
  abstract isDirectory(path: string): Promise<boolean>;

  /**
   * Download file to local temporary location
   * For local files, this just returns the path
   * For cloud files, this downloads to temp directory
   */
  abstract downloadToTemp(path: string): Promise<string>;

  /**
   * Get file size in bytes
   */
  abstract getSize(path: string): Promise<number>;

  /**
   * Normalize path for the repository
   * Handles platform-specific path separators
   */
  protected normalizePath(path: string): string {
    return path.replace(/\\/g, '/');
  }

  /**
   * Get file extension (lowercase, without dot)
   */
  protected getExtension(path: string | undefined): string {
    if (!path) return '';
    const match = path.match(/\.([^.]+)$/);
    return match ? match[1].toLowerCase() : '';
  }

  /**
   * Get filename from path
   */
  protected getFilename(path: string): string {
    return path.split(/[/\\]/).pop() || '';
  }
}
