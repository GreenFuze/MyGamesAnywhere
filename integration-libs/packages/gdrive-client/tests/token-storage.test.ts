/**
 * Tests for token storage implementations
 */

import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { FileTokenStorage, MemoryTokenStorage } from '../src/token-storage.js';
import type { StoredTokens } from '../src/types.js';
import { existsSync } from 'fs';
import { unlink } from 'fs/promises';
import { join } from 'path';
import { tmpdir } from 'os';

describe('MemoryTokenStorage', () => {
  let storage: MemoryTokenStorage;

  beforeEach(() => {
    storage = new MemoryTokenStorage();
  });

  it('should store and retrieve tokens', async () => {
    const tokens: StoredTokens = {
      access_token: 'test_access_token',
      refresh_token: 'test_refresh_token',
      expires_in: 3600,
      token_type: 'Bearer',
      expiresAt: Date.now() + 3600000,
    };

    await storage.saveTokens(tokens);
    const retrieved = await storage.loadTokens();

    expect(retrieved).toEqual(tokens);
  });

  it('should return null when no tokens stored', async () => {
    const retrieved = await storage.loadTokens();
    expect(retrieved).toBeNull();
  });

  it('should clear tokens', async () => {
    const tokens: StoredTokens = {
      access_token: 'test_access_token',
      refresh_token: 'test_refresh_token',
      expires_in: 3600,
      token_type: 'Bearer',
      expiresAt: Date.now() + 3600000,
    };

    await storage.saveTokens(tokens);
    await storage.clearTokens();

    const retrieved = await storage.loadTokens();
    expect(retrieved).toBeNull();
  });

  it('should validate tokens with Zod schema', async () => {
    const validTokens: StoredTokens = {
      access_token: 'test_access_token',
      expires_in: 3600,
      token_type: 'Bearer',
      expiresAt: Date.now() + 3600000,
    };

    await expect(storage.saveTokens(validTokens)).resolves.not.toThrow();
  });
});

describe('FileTokenStorage', () => {
  let storage: FileTokenStorage;
  let testFilePath: string;

  beforeEach(() => {
    testFilePath = join(tmpdir(), `test-tokens-${Date.now()}.json`);
    storage = new FileTokenStorage(testFilePath);
  });

  afterEach(async () => {
    if (existsSync(testFilePath)) {
      await unlink(testFilePath);
    }
  });

  it('should store and retrieve tokens from file', async () => {
    const tokens: StoredTokens = {
      access_token: 'test_access_token',
      refresh_token: 'test_refresh_token',
      expires_in: 3600,
      token_type: 'Bearer',
      scope: 'https://www.googleapis.com/auth/drive.file',
      expiresAt: Date.now() + 3600000,
    };

    await storage.saveTokens(tokens);
    expect(existsSync(testFilePath)).toBe(true);

    const retrieved = await storage.loadTokens();
    expect(retrieved).toEqual(tokens);
  });

  it('should return null when file does not exist', async () => {
    const retrieved = await storage.loadTokens();
    expect(retrieved).toBeNull();
  });

  it('should return null when file contains invalid JSON', async () => {
    const { writeFile } = await import('fs/promises');
    await writeFile(testFilePath, 'invalid json', 'utf-8');

    const retrieved = await storage.loadTokens();
    expect(retrieved).toBeNull();
  });

  it('should return null when file contains invalid token schema', async () => {
    const { writeFile } = await import('fs/promises');
    await writeFile(
      testFilePath,
      JSON.stringify({ invalid: 'data' }),
      'utf-8'
    );

    const retrieved = await storage.loadTokens();
    expect(retrieved).toBeNull();
  });

  it('should clear tokens by deleting file', async () => {
    const tokens: StoredTokens = {
      access_token: 'test_access_token',
      refresh_token: 'test_refresh_token',
      expires_in: 3600,
      token_type: 'Bearer',
      expiresAt: Date.now() + 3600000,
    };

    await storage.saveTokens(tokens);
    expect(existsSync(testFilePath)).toBe(true);

    await storage.clearTokens();
    expect(existsSync(testFilePath)).toBe(false);
  });

  it('should not throw when clearing non-existent file', async () => {
    await expect(storage.clearTokens()).resolves.not.toThrow();
  });

  it('should write formatted JSON', async () => {
    const tokens: StoredTokens = {
      access_token: 'test_access_token',
      expires_in: 3600,
      token_type: 'Bearer',
      expiresAt: Date.now() + 3600000,
    };

    await storage.saveTokens(tokens);

    const { readFile } = await import('fs/promises');
    const content = await readFile(testFilePath, 'utf-8');

    // Check that it's formatted (has newlines and indentation)
    expect(content).toContain('\n');
    expect(content).toContain('  ');
  });

  it('should overwrite existing tokens', async () => {
    const tokens1: StoredTokens = {
      access_token: 'token1',
      expires_in: 3600,
      token_type: 'Bearer',
      expiresAt: Date.now() + 3600000,
    };

    const tokens2: StoredTokens = {
      access_token: 'token2',
      expires_in: 7200,
      token_type: 'Bearer',
      expiresAt: Date.now() + 7200000,
    };

    await storage.saveTokens(tokens1);
    await storage.saveTokens(tokens2);

    const retrieved = await storage.loadTokens();
    expect(retrieved?.access_token).toBe('token2');
  });
});
