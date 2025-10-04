/**
 * VDF (Valve Data Format) Parser
 *
 * Parses Steam's VDF format which is used in:
 * - libraryfolders.vdf (library folder locations)
 * - appmanifest_*.acf files (game install manifests)
 */

import { VDFObject } from './types.js';
import { VDFParseError } from './errors.js';

/**
 * Parse VDF text content into a JavaScript object
 *
 * VDF Format Example:
 * "KeyName"
 * {
 *     "SubKey"  "Value"
 *     "NestedObject"
 *     {
 *         "Key"  "Value"
 *     }
 * }
 */
export class VDFParser {
  private content: string;
  private position: number;
  private line: number;
  private column: number;

  constructor(content: string) {
    this.content = content;
    this.position = 0;
    this.line = 1;
    this.column = 1;
  }

  /**
   * Parse the VDF content
   */
  public parse(): VDFObject {
    try {
      return this.parseObject();
    } catch (error) {
      if (error instanceof VDFParseError) {
        throw error;
      }
      throw new VDFParseError(
        `Unexpected error during VDF parsing: ${error instanceof Error ? error.message : String(error)}`,
        undefined,
        this.line
      );
    }
  }

  /**
   * Parse an object (key-value pairs between braces)
   */
  private parseObject(): VDFObject {
    const result: VDFObject = {};

    this.skipWhitespace();

    while (this.position < this.content.length) {
      this.skipWhitespace();

      // Check for closing brace
      if (this.peek() === '}') {
        break;
      }

      // End of content
      if (this.position >= this.content.length) {
        break;
      }

      // Parse key
      const key = this.parseString();
      if (!key) {
        break;
      }

      this.skipWhitespace();

      // Check if next is an opening brace (nested object)
      if (this.peek() === '{') {
        this.consume(); // consume '{'
        result[key] = this.parseObject();
        this.skipWhitespace();
        if (this.peek() === '}') {
          this.consume(); // consume '}'
        }
      } else {
        // Parse value
        const value = this.parseString();
        if (value !== null) {
          result[key] = value;
        }
      }

      this.skipWhitespace();
    }

    return result;
  }

  /**
   * Parse a quoted string
   */
  private parseString(): string | null {
    this.skipWhitespace();

    if (this.position >= this.content.length) {
      return null;
    }

    // VDF strings are enclosed in double quotes
    if (this.peek() !== '"') {
      return null;
    }

    this.consume(); // consume opening quote

    let value = '';
    let escaped = false;

    while (this.position < this.content.length) {
      const char = this.current();

      if (escaped) {
        // Handle escaped characters
        switch (char) {
          case 'n':
            value += '\n';
            break;
          case 't':
            value += '\t';
            break;
          case 'r':
            value += '\r';
            break;
          case '"':
            value += '"';
            break;
          case '\\':
            value += '\\';
            break;
          default:
            value += char;
        }
        escaped = false;
      } else if (char === '\\') {
        escaped = true;
      } else if (char === '"') {
        this.consume(); // consume closing quote
        return value;
      } else {
        value += char;
      }

      this.advance();
    }

    throw new VDFParseError(
      'Unexpected end of file while parsing string',
      undefined,
      this.line
    );
  }

  /**
   * Skip whitespace and comments
   */
  private skipWhitespace(): void {
    while (this.position < this.content.length) {
      const char = this.current();

      // Skip whitespace
      if (char === ' ' || char === '\t' || char === '\r' || char === '\n') {
        this.advance();
        continue;
      }

      // Skip comments (VDF supports // comments)
      if (char === '/' && this.peek(1) === '/') {
        // Skip until end of line
        while (
          this.position < this.content.length &&
          this.current() !== '\n'
        ) {
          this.advance();
        }
        continue;
      }

      break;
    }
  }

  /**
   * Get current character
   */
  private current(): string {
    return this.content[this.position];
  }

  /**
   * Peek at character at offset from current position
   */
  private peek(offset: number = 0): string {
    const pos = this.position + offset;
    return pos < this.content.length ? this.content[pos] : '';
  }

  /**
   * Consume current character (same as advance, but clearer intent)
   */
  private consume(): void {
    this.advance();
  }

  /**
   * Advance to next character
   */
  private advance(): void {
    if (this.position < this.content.length) {
      if (this.current() === '\n') {
        this.line++;
        this.column = 1;
      } else {
        this.column++;
      }
      this.position++;
    }
  }
}

/**
 * Parse VDF content string into object
 */
export function parseVDF(content: string): VDFObject {
  const parser = new VDFParser(content);
  return parser.parse();
}
