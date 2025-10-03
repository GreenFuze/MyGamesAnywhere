/**
 * Recursive Directory Walker
 * Traverses directory trees with configurable depth and filtering
 */

import type { RepositoryAdapter, FileInfo, ScannerConfig } from '../types.js';

/**
 * Walk callback function
 */
export type WalkCallback = (file: FileInfo) => Promise<void> | void;

/**
 * Recursive directory walker
 */
export class RecursiveWalker {
  private adapter: RepositoryAdapter;
  private config: Required<ScannerConfig>;
  private filesScanned: number = 0;
  private directoriesScanned: number = 0;

  constructor(adapter: RepositoryAdapter, config?: ScannerConfig) {
    this.adapter = adapter;
    this.config = {
      maxDepth: config?.maxDepth ?? 10,
      includeHidden: config?.includeHidden ?? false,
      excludePatterns: config?.excludePatterns ?? [],
      extractArchives: config?.extractArchives ?? false,
      fetchMetadata: config?.fetchMetadata ?? true,
      parallel: config?.parallel ?? false,
      maxParallel: config?.maxParallel ?? 5,
    };
  }

  /**
   * Walk directory tree and call callback for each file
   */
  async walk(startPath: string, callback: WalkCallback): Promise<void> {
    this.filesScanned = 0;
    this.directoriesScanned = 0;

    await this.walkRecursive(startPath, callback, 0);
  }

  /**
   * Get number of files scanned
   */
  getFilesScanned(): number {
    return this.filesScanned;
  }

  /**
   * Get number of directories scanned
   */
  getDirectoriesScanned(): number {
    return this.directoriesScanned;
  }

  /**
   * Recursive walk implementation
   */
  private async walkRecursive(
    path: string,
    callback: WalkCallback,
    depth: number
  ): Promise<void> {
    // Check depth limit
    if (depth > this.config.maxDepth) {
      return;
    }

    // Check if path exists
    const exists = await this.adapter.exists(path);
    if (!exists) {
      return;
    }

    // Get file info
    const fileInfo = await this.adapter.getFileInfo(path);

    // Skip hidden files if configured
    if (!this.config.includeHidden && this.isHidden(fileInfo.name)) {
      return;
    }

    // Skip excluded patterns
    if (this.isExcluded(path)) {
      return;
    }

    // If it's a file, call callback
    if (!fileInfo.isDirectory) {
      this.filesScanned++;
      await callback(fileInfo);
      return;
    }

    // It's a directory, scan it
    this.directoriesScanned++;

    try {
      const entries = await this.adapter.listFiles(path);

      if (this.config.parallel) {
        // Parallel processing
        await this.processEntriesParallel(entries, callback, depth);
      } else {
        // Sequential processing
        for (const entry of entries) {
          await this.walkRecursive(entry, callback, depth + 1);
        }
      }
    } catch (error) {
      // Log error but continue walking
      console.error(`Error walking directory ${path}:`, error);
    }
  }

  /**
   * Process directory entries in parallel
   */
  private async processEntriesParallel(
    entries: string[],
    callback: WalkCallback,
    depth: number
  ): Promise<void> {
    const chunks = this.chunkArray(entries, this.config.maxParallel);

    for (const chunk of chunks) {
      await Promise.all(
        chunk.map((entry) => this.walkRecursive(entry, callback, depth + 1))
      );
    }
  }

  /**
   * Check if filename is hidden
   */
  private isHidden(name: string): boolean {
    return name.startsWith('.');
  }

  /**
   * Check if path matches excluded patterns
   */
  private isExcluded(path: string): boolean {
    const normalizedPath = path.toLowerCase();

    for (const pattern of this.config.excludePatterns) {
      const normalizedPattern = pattern.toLowerCase();

      // Simple glob-like matching
      if (normalizedPattern.includes('*')) {
        const regex = new RegExp(
          '^' + normalizedPattern.replace(/\*/g, '.*') + '$'
        );
        if (regex.test(normalizedPath)) {
          return true;
        }
      } else if (normalizedPath.includes(normalizedPattern)) {
        return true;
      }
    }

    return false;
  }

  /**
   * Split array into chunks
   */
  private chunkArray<T>(array: T[], size: number): T[][] {
    const chunks: T[][] = [];
    for (let i = 0; i < array.length; i += size) {
      chunks.push(array.slice(i, i + size));
    }
    return chunks;
  }
}
