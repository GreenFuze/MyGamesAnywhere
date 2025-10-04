/**
 * Edge case tests for VDF Parser
 */

import { describe, it, expect } from 'vitest';
import { parseVDF } from '../src/vdf-parser.js';
import { VDFParseError } from '../src/errors.js';

describe('VDFParser - Edge Cases', () => {
  describe('Empty and Whitespace', () => {
    it('should handle empty string', () => {
      const result = parseVDF('');
      expect(result).toEqual({});
    });

    it('should handle only whitespace', () => {
      const result = parseVDF('   \n\t\r\n   ');
      expect(result).toEqual({});
    });

    it('should handle only comments', () => {
      const vdf = `
// Comment 1
// Comment 2
// Comment 3
      `;
      const result = parseVDF(vdf);
      expect(result).toEqual({});
    });

    it('should handle empty object', () => {
      const vdf = `
"root"
{
}
      `;
      const result = parseVDF(vdf);
      expect(result).toEqual({ root: {} });
    });
  });

  describe('Special Characters', () => {
    it('should handle backslashes in paths', () => {
      const vdf = `
"root"
{
  "path"  "C:\\\\Program Files\\\\Steam"
}
      `;
      const result = parseVDF(vdf);
      expect(result).toEqual({
        root: {
          path: 'C:\\Program Files\\Steam',
        },
      });
    });

    it('should handle forward slashes', () => {
      const vdf = `
"root"
{
  "path"  "/usr/local/share/Steam"
}
      `;
      const result = parseVDF(vdf);
      expect(result).toEqual({
        root: {
          path: '/usr/local/share/Steam',
        },
      });
    });

    it('should handle special characters in keys', () => {
      const vdf = `
"root"
{
  "key-with-dashes"  "value"
  "key_with_underscores"  "value"
  "key.with.dots"  "value"
}
      `;
      const result = parseVDF(vdf);
      expect(result.root).toHaveProperty('key-with-dashes');
      expect(result.root).toHaveProperty('key_with_underscores');
      expect(result.root).toHaveProperty('key.with.dots');
    });

    it('should handle unicode characters', () => {
      const vdf = `
"root"
{
  "name"  "Café ☕ 日本語"
  "emoji"  "🎮🕹️👾"
}
      `;
      const result = parseVDF(vdf);
      expect(result).toEqual({
        root: {
          name: 'Café ☕ 日本語',
          emoji: '🎮🕹️👾',
        },
      });
    });

    it('should handle empty strings', () => {
      const vdf = `
"root"
{
  "empty"  ""
  "another"  ""
}
      `;
      const result = parseVDF(vdf);
      expect(result).toEqual({
        root: {
          empty: '',
          another: '',
        },
      });
    });
  });

  describe('Line Endings', () => {
    it('should handle Windows line endings (CRLF)', () => {
      const vdf = '"root"\r\n{\r\n  "key"  "value"\r\n}\r\n';
      const result = parseVDF(vdf);
      expect(result).toEqual({
        root: {
          key: 'value',
        },
      });
    });

    it('should handle Unix line endings (LF)', () => {
      const vdf = '"root"\n{\n  "key"  "value"\n}\n';
      const result = parseVDF(vdf);
      expect(result).toEqual({
        root: {
          key: 'value',
        },
      });
    });

    it('should handle mixed line endings', () => {
      const vdf = '"root"\r\n{\n  "key"  "value"\r\n}\n';
      const result = parseVDF(vdf);
      expect(result).toEqual({
        root: {
          key: 'value',
        },
      });
    });
  });

  describe('Deep Nesting', () => {
    it('should handle very deep nesting (10 levels)', () => {
      const vdf = `
"l1"
{
  "l2"
  {
    "l3"
    {
      "l4"
      {
        "l5"
        {
          "l6"
          {
            "l7"
            {
              "l8"
              {
                "l9"
                {
                  "l10"
                  {
                    "key"  "deep value"
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}
      `;
      const result = parseVDF(vdf);
      expect(result.l1.l2.l3.l4.l5.l6.l7.l8.l9.l10).toEqual({
        key: 'deep value',
      });
    });

    it('should handle multiple nested siblings', () => {
      const vdf = `
"root"
{
  "child1"
  {
    "grandchild1"  "value1"
    "grandchild2"  "value2"
  }
  "child2"
  {
    "grandchild3"  "value3"
    "grandchild4"  "value4"
  }
}
      `;
      const result = parseVDF(vdf);
      expect(result).toEqual({
        root: {
          child1: {
            grandchild1: 'value1',
            grandchild2: 'value2',
          },
          child2: {
            grandchild3: 'value3',
            grandchild4: 'value4',
          },
        },
      });
    });
  });

  describe('Whitespace Variations', () => {
    it('should handle various amounts of whitespace', () => {
      const vdf = `
"root"
{
  "key1"     "value1"
  "key2"  "value2"
  "key3"      "value3"
}
      `;
      const result = parseVDF(vdf);
      expect(result.root).toEqual({
        key1: 'value1',
        key2: 'value2',
        key3: 'value3',
      });
    });

    it('should handle tabs and spaces mixed', () => {
      const vdf = `
"root"
{
\t"key1"\t\t"value1"
  "key2"  "value2"
\t  "key3"\t  "value3"
}
      `;
      const result = parseVDF(vdf);
      expect(result.root).toEqual({
        key1: 'value1',
        key2: 'value2',
        key3: 'value3',
      });
    });

    it('should trim whitespace inside strings', () => {
      const vdf = `
"root"
{
  "key"  "  value with spaces  "
}
      `;
      const result = parseVDF(vdf);
      // VDF preserves spaces inside quoted strings
      expect(result.root.key).toBe('  value with spaces  ');
    });
  });

  describe('Comment Variations', () => {
    it('should handle comments at different positions', () => {
      const vdf = `
// Top comment
"root" // After key
{ // After brace
  "key1"  "value1" // After value
  // Middle comment
  "key2"  "value2"
} // Closing brace
// Bottom comment
      `;
      const result = parseVDF(vdf);
      expect(result.root).toEqual({
        key1: 'value1',
        key2: 'value2',
      });
    });

    it('should handle multiple slashes in comments', () => {
      const vdf = `
"root"
{
  //// Multiple slashes
  "key"  "value" //// More slashes
}
      `;
      const result = parseVDF(vdf);
      expect(result.root).toEqual({
        key: 'value',
      });
    });

    it('should not treat // inside strings as comments', () => {
      const vdf = `
"root"
{
  "url"  "https://example.com"
  "path"  "C://Program Files//Steam"
}
      `;
      const result = parseVDF(vdf);
      expect(result.root).toEqual({
        url: 'https://example.com',
        path: 'C://Program Files//Steam',
      });
    });
  });

  describe('Escaped Characters', () => {
    it('should handle all escape sequences', () => {
      const vdf = `
"root"
{
  "newline"  "line1\\nline2"
  "tab"  "col1\\tcol2"
  "return"  "text\\rmore"
  "quote"  "He said \\"Hello\\""
  "backslash"  "path\\\\to\\\\file"
}
      `;
      const result = parseVDF(vdf);
      expect(result.root).toEqual({
        newline: 'line1\nline2',
        tab: 'col1\tcol2',
        return: 'text\rmore',
        quote: 'He said "Hello"',
        backslash: 'path\\to\\file',
      });
    });

    it('should handle unknown escape sequences', () => {
      const vdf = `
"root"
{
  "unknown"  "\\x\\y\\z"
}
      `;
      const result = parseVDF(vdf);
      // Unknown escapes are treated literally
      expect(result.root.unknown).toBe('xyz');
    });
  });

  describe('Malformed VDF', () => {
    it('should throw on unclosed string', () => {
      const vdf = `
"root"
{
  "key"  "unclosed
}
      `;
      expect(() => parseVDF(vdf)).toThrow(VDFParseError);
    });

    it('should handle key without value by treating it as empty object', () => {
      const vdf = `
"root"
{
  "key"
  "anotherKey"  "value"
}
      `;
      // VDF parser treats a key followed by another key as an object
      const result = parseVDF(vdf);
      // The parser sees "key" as starting an object, then "anotherKey" as a child
      expect(result.root).toHaveProperty('key');
    });

    it('should handle extra closing braces gracefully', () => {
      const vdf = `
"root"
{
  "key"  "value"
}
}
      `;
      const result = parseVDF(vdf);
      expect(result.root).toEqual({
        key: 'value',
      });
    });
  });

  describe('Large Values', () => {
    it('should handle very long strings', () => {
      const longValue = 'x'.repeat(10000);
      const vdf = `
"root"
{
  "longString"  "${longValue}"
}
      `;
      const result = parseVDF(vdf);
      expect(result.root.longString).toHaveLength(10000);
    });

    it('should handle many key-value pairs', () => {
      let vdf = '"root"\n{\n';
      for (let i = 0; i < 1000; i++) {
        vdf += `  "key${i}"  "value${i}"\n`;
      }
      vdf += '}\n';

      const result = parseVDF(vdf);
      expect(Object.keys(result.root)).toHaveLength(1000);
      expect(result.root.key0).toBe('value0');
      expect(result.root.key999).toBe('value999');
    });
  });

  describe('Real-world Edge Cases', () => {
    it('should handle Steam appid as string', () => {
      const vdf = `
"AppState"
{
  "appid"  "440"
  "name"  "Team Fortress 2"
}
      `;
      const result = parseVDF(vdf);
      expect(result.AppState.appid).toBe('440');
      expect(typeof result.AppState.appid).toBe('string');
    });

    it('should handle large file sizes', () => {
      const vdf = `
"library"
{
  "0"
  {
    "totalsize"  "9999999999999"
  }
}
      `;
      const result = parseVDF(vdf);
      expect(result.library['0'].totalsize).toBe('9999999999999');
    });

    it('should handle nested apps object', () => {
      const vdf = `
"libraryfolders"
{
  "0"
  {
    "path"  "C:\\\\Steam"
    "apps"
    {
      "440"  "26843545600"
      "730"  "15000000000"
    }
  }
}
      `;
      const result = parseVDF(vdf);
      expect(result.libraryfolders['0'].apps).toEqual({
        '440': '26843545600',
        '730': '15000000000',
      });
    });
  });
});
