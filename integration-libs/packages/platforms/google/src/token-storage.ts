/**
 * Token storage implementations
 */

import { readFile, writeFile, unlink } from 'fs/promises';
import { existsSync } from 'fs';
import type { TokenStorage, StoredTokens } from './types.js';
import { StoredTokensSchema } from './types.js';

/**
 * File-based token storage
 * Stores tokens in a JSON file on disk
 */
export class FileTokenStorage implements TokenStorage {
  constructor(private readonly filePath: string) {}

  async saveTokens(tokens: StoredTokens): Promise<void> {
    try {
      const validated = StoredTokensSchema.parse(tokens);
      const json = JSON.stringify(validated, null, 2);
      await writeFile(this.filePath, json, 'utf-8');
    } catch (error) {
      throw new Error(
        `Failed to save tokens: ${
          error instanceof Error ? error.message : 'Unknown error'
        }`
      );
    }
  }

  async loadTokens(): Promise<StoredTokens | null> {
    try {
      if (!existsSync(this.filePath)) {
        return null;
      }

      const json = await readFile(this.filePath, 'utf-8');
      const data = JSON.parse(json);

      // Validate with Zod
      const validated = StoredTokensSchema.parse(data);
      return validated;
    } catch (error) {
      // If file doesn't exist or is invalid, return null
      return null;
    }
  }

  async clearTokens(): Promise<void> {
    try {
      if (existsSync(this.filePath)) {
        await unlink(this.filePath);
      }
    } catch (error) {
      throw new Error(
        `Failed to clear tokens: ${
          error instanceof Error ? error.message : 'Unknown error'
        }`
      );
    }
  }
}

/**
 * In-memory token storage
 * Useful for testing or temporary sessions
 */
export class MemoryTokenStorage implements TokenStorage {
  private tokens: StoredTokens | null = null;

  async saveTokens(tokens: StoredTokens): Promise<void> {
    this.tokens = StoredTokensSchema.parse(tokens);
  }

  async loadTokens(): Promise<StoredTokens | null> {
    return this.tokens;
  }

  async clearTokens(): Promise<void> {
    this.tokens = null;
  }
}
