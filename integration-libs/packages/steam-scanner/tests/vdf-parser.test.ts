/**
 * Tests for VDF Parser
 */

import { describe, it, expect } from 'vitest';
import { parseVDF, VDFParser } from '../src/vdf-parser.js';
import { VDFParseError } from '../src/errors.js';
import { readFile } from 'fs/promises';
import { join } from 'path';

describe('VDFParser', () => {
  describe('parseVDF', () => {
    it('should parse simple key-value pairs', () => {
      const vdf = `
"root"
{
  "key1"  "value1"
  "key2"  "value2"
}
      `;

      const result = parseVDF(vdf);

      expect(result).toEqual({
        root: {
          key1: 'value1',
          key2: 'value2',
        },
      });
    });

    it('should parse nested objects', () => {
      const vdf = `
"root"
{
  "nested"
  {
    "innerKey"  "innerValue"
  }
}
      `;

      const result = parseVDF(vdf);

      expect(result).toEqual({
        root: {
          nested: {
            innerKey: 'innerValue',
          },
        },
      });
    });

    it('should parse multiple nested levels', () => {
      const vdf = `
"root"
{
  "level1"
  {
    "level2"
    {
      "level3"
      {
        "key"  "value"
      }
    }
  }
}
      `;

      const result = parseVDF(vdf);

      expect(result).toEqual({
        root: {
          level1: {
            level2: {
              level3: {
                key: 'value',
              },
            },
          },
        },
      });
    });

    it('should handle escaped characters', () => {
      const vdf = `
"root"
{
  "key"  "value with \\"quotes\\""
  "newline"  "line1\\nline2"
  "tab"  "col1\\tcol2"
}
      `;

      const result = parseVDF(vdf);

      expect(result).toEqual({
        root: {
          key: 'value with "quotes"',
          newline: 'line1\nline2',
          tab: 'col1\tcol2',
        },
      });
    });

    it('should handle comments', () => {
      const vdf = `
"root"
{
  // This is a comment
  "key1"  "value1"
  "key2"  "value2"  // Inline comment
  // Another comment
}
      `;

      const result = parseVDF(vdf);

      expect(result).toEqual({
        root: {
          key1: 'value1',
          key2: 'value2',
        },
      });
    });

    it('should parse numeric strings', () => {
      const vdf = `
"root"
{
  "appid"  "440"
  "size"  "26843545600"
  "timestamp"  "1638360000"
}
      `;

      const result = parseVDF(vdf);

      expect(result).toEqual({
        root: {
          appid: '440',
          size: '26843545600',
          timestamp: '1638360000',
        },
      });
    });

    it('should throw error on unclosed string', () => {
      const vdf = `
"root"
{
  "key"  "unclosed value
}
      `;

      expect(() => parseVDF(vdf)).toThrow(VDFParseError);
    });

    it('should handle empty objects', () => {
      const vdf = `
"root"
{
  "empty"
  {
  }
  "key"  "value"
}
      `;

      const result = parseVDF(vdf);

      expect(result).toEqual({
        root: {
          empty: {},
          key: 'value',
        },
      });
    });
  });

  describe('Real VDF Files', () => {
    it('should parse libraryfolders.vdf', async () => {
      const content = await readFile(
        join(import.meta.dirname, 'fixtures', 'libraryfolders.vdf'),
        'utf-8'
      );

      const result = parseVDF(content);

      expect(result).toHaveProperty('libraryfolders');
      const folders = result.libraryfolders as Record<string, any>;

      expect(folders['0']).toBeDefined();
      expect(folders['0'].path).toBe('C:\\Program Files (x86)\\Steam');
      expect(folders['1']).toBeDefined();
      expect(folders['1'].path).toBe('D:\\SteamLibrary');
    });

    it('should parse appmanifest_440.acf', async () => {
      const content = await readFile(
        join(import.meta.dirname, 'fixtures', 'appmanifest_440.acf'),
        'utf-8'
      );

      const result = parseVDF(content);

      expect(result).toHaveProperty('AppState');
      const appState = result.AppState as Record<string, any>;

      expect(appState.appid).toBe('440');
      expect(appState.name).toBe('Team Fortress 2');
      expect(appState.installdir).toBe('Team Fortress 2');
      expect(appState.SizeOnDisk).toBe('26843545600');
    });
  });
});
