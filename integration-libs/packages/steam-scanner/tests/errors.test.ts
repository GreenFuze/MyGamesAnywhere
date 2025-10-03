/**
 * Tests for error classes
 */

import { describe, it, expect } from 'vitest';
import {
  SteamScannerError,
  SteamNotFoundError,
  VDFParseError,
  FileAccessError,
  InvalidConfigError,
} from '../src/errors.js';

describe('Error Classes', () => {
  describe('SteamScannerError', () => {
    it('should create error with message and code', () => {
      const error = new SteamScannerError('Test error', 'TEST_CODE');

      expect(error).toBeInstanceOf(Error);
      expect(error).toBeInstanceOf(SteamScannerError);
      expect(error.message).toBe('Test error');
      expect(error.code).toBe('TEST_CODE');
      expect(error.name).toBe('SteamScannerError');
    });

    it('should have correct prototype chain', () => {
      const error = new SteamScannerError('Test', 'CODE');

      expect(error instanceof SteamScannerError).toBe(true);
      expect(error instanceof Error).toBe(true);
    });

    it('should preserve stack trace', () => {
      const error = new SteamScannerError('Test', 'CODE');

      expect(error.stack).toBeDefined();
      expect(error.stack).toContain('SteamScannerError');
    });
  });

  describe('SteamNotFoundError', () => {
    it('should create error with default message', () => {
      const error = new SteamNotFoundError();

      expect(error).toBeInstanceOf(SteamNotFoundError);
      expect(error).toBeInstanceOf(SteamScannerError);
      expect(error.message).toBe('Steam installation not found');
      expect(error.code).toBe('STEAM_NOT_FOUND');
      expect(error.name).toBe('SteamNotFoundError');
    });

    it('should create error with custom message', () => {
      const error = new SteamNotFoundError('Custom not found message');

      expect(error.message).toBe('Custom not found message');
      expect(error.code).toBe('STEAM_NOT_FOUND');
    });

    it('should be catchable as SteamScannerError', () => {
      try {
        throw new SteamNotFoundError();
      } catch (error) {
        expect(error).toBeInstanceOf(SteamScannerError);
        expect(error).toBeInstanceOf(SteamNotFoundError);
      }
    });
  });

  describe('VDFParseError', () => {
    it('should create error with message only', () => {
      const error = new VDFParseError('Parse failed');

      expect(error).toBeInstanceOf(VDFParseError);
      expect(error.message).toBe('Parse failed');
      expect(error.code).toBe('VDF_PARSE_ERROR');
      expect(error.name).toBe('VDFParseError');
      expect(error.filePath).toBeUndefined();
      expect(error.line).toBeUndefined();
    });

    it('should create error with file path', () => {
      const error = new VDFParseError(
        'Parse failed',
        '/path/to/file.vdf'
      );

      expect(error.message).toBe('Parse failed');
      expect(error.filePath).toBe('/path/to/file.vdf');
      expect(error.line).toBeUndefined();
    });

    it('should create error with file path and line number', () => {
      const error = new VDFParseError(
        'Parse failed',
        '/path/to/file.vdf',
        42
      );

      expect(error.message).toBe('Parse failed');
      expect(error.filePath).toBe('/path/to/file.vdf');
      expect(error.line).toBe(42);
    });

    it('should preserve all error properties', () => {
      const error = new VDFParseError('Error', 'file.vdf', 10);

      // Should be catchable and properties accessible
      try {
        throw error;
      } catch (e) {
        if (e instanceof VDFParseError) {
          expect(e.filePath).toBe('file.vdf');
          expect(e.line).toBe(10);
          expect(e.code).toBe('VDF_PARSE_ERROR');
        }
      }
    });
  });

  describe('FileAccessError', () => {
    it('should create error with file path', () => {
      const error = new FileAccessError(
        'Cannot access file',
        '/path/to/file'
      );

      expect(error).toBeInstanceOf(FileAccessError);
      expect(error.message).toBe('Cannot access file');
      expect(error.filePath).toBe('/path/to/file');
      expect(error.code).toBe('FILE_ACCESS_ERROR');
      expect(error.name).toBe('FileAccessError');
      expect(error.originalError).toBeUndefined();
    });

    it('should create error with original error', () => {
      const originalError = new Error('ENOENT');
      const error = new FileAccessError(
        'Cannot access file',
        '/path/to/file',
        originalError
      );

      expect(error.message).toBe('Cannot access file');
      expect(error.filePath).toBe('/path/to/file');
      expect(error.originalError).toBe(originalError);
    });

    it('should preserve original error information', () => {
      const originalError = new Error('Permission denied');
      originalError.stack = 'Original stack trace';

      const error = new FileAccessError(
        'Access failed',
        '/secure/file',
        originalError
      );

      expect(error.originalError?.message).toBe('Permission denied');
      expect(error.originalError?.stack).toBe('Original stack trace');
    });

    it('should be catchable with file path', () => {
      try {
        throw new FileAccessError('Error', '/test/path');
      } catch (e) {
        if (e instanceof FileAccessError) {
          expect(e.filePath).toBe('/test/path');
        }
      }
    });
  });

  describe('InvalidConfigError', () => {
    it('should create error with message', () => {
      const error = new InvalidConfigError('Invalid steam path');

      expect(error).toBeInstanceOf(InvalidConfigError);
      expect(error).toBeInstanceOf(SteamScannerError);
      expect(error.message).toBe('Invalid steam path');
      expect(error.code).toBe('INVALID_CONFIG');
      expect(error.name).toBe('InvalidConfigError');
    });

    it('should be catchable as SteamScannerError', () => {
      try {
        throw new InvalidConfigError('Bad config');
      } catch (error) {
        expect(error).toBeInstanceOf(SteamScannerError);
        expect(error).toBeInstanceOf(InvalidConfigError);
        if (error instanceof SteamScannerError) {
          expect(error.code).toBe('INVALID_CONFIG');
        }
      }
    });
  });

  describe('Error Inheritance', () => {
    it('should maintain correct instanceof relationships', () => {
      const errors = [
        new SteamNotFoundError(),
        new VDFParseError('test'),
        new FileAccessError('test', '/path'),
        new InvalidConfigError('test'),
      ];

      errors.forEach((error) => {
        expect(error).toBeInstanceOf(Error);
        expect(error).toBeInstanceOf(SteamScannerError);
      });
    });

    it('should allow catching specific error types', () => {
      const throwSteamNotFound = () => {
        throw new SteamNotFoundError();
      };

      const throwVDFError = () => {
        throw new VDFParseError('Parse error');
      };

      const throwFileError = () => {
        throw new FileAccessError('Access error', '/path');
      };

      // Catch specific types
      expect(() => throwSteamNotFound()).toThrow(SteamNotFoundError);
      expect(() => throwVDFError()).toThrow(VDFParseError);
      expect(() => throwFileError()).toThrow(FileAccessError);

      // All should be catchable as SteamScannerError
      expect(() => throwSteamNotFound()).toThrow(SteamScannerError);
      expect(() => throwVDFError()).toThrow(SteamScannerError);
      expect(() => throwFileError()).toThrow(SteamScannerError);

      // All should be catchable as Error
      expect(() => throwSteamNotFound()).toThrow(Error);
      expect(() => throwVDFError()).toThrow(Error);
      expect(() => throwFileError()).toThrow(Error);
    });

    it('should allow error code filtering', () => {
      const errors = [
        new SteamNotFoundError(),
        new VDFParseError('test'),
        new FileAccessError('test', '/path'),
        new InvalidConfigError('test'),
      ];

      const codes = errors.map((e) => e.code);

      expect(codes).toContain('STEAM_NOT_FOUND');
      expect(codes).toContain('VDF_PARSE_ERROR');
      expect(codes).toContain('FILE_ACCESS_ERROR');
      expect(codes).toContain('INVALID_CONFIG');
      expect(new Set(codes).size).toBe(4); // All unique
    });
  });

  describe('Error Messages', () => {
    it('should preserve error messages through inheritance', () => {
      const customMessage = 'Very specific error message';
      const error = new SteamNotFoundError(customMessage);

      expect(error.message).toBe(customMessage);
      expect(error.toString()).toContain(customMessage);
    });

    it('should include error name in toString', () => {
      const errors = [
        new SteamNotFoundError('test'),
        new VDFParseError('test'),
        new FileAccessError('test', '/path'),
        new InvalidConfigError('test'),
      ];

      errors.forEach((error) => {
        const str = error.toString();
        expect(str).toContain(error.name);
        expect(str).toContain('test');
      });
    });
  });

  describe('Error Context', () => {
    it('should provide useful context for debugging VDF errors', () => {
      const error = new VDFParseError(
        'Unexpected token',
        'libraryfolders.vdf',
        15
      );

      expect(error.filePath).toBe('libraryfolders.vdf');
      expect(error.line).toBe(15);
      expect(error.message).toContain('Unexpected token');

      // Should provide enough info to locate the issue
      const debugInfo = {
        file: error.filePath,
        line: error.line,
        message: error.message,
      };

      expect(debugInfo.file).toBeTruthy();
      expect(debugInfo.line).toBeGreaterThan(0);
      expect(debugInfo.message).toBeTruthy();
    });

    it('should provide useful context for debugging file errors', () => {
      const originalError = new Error('EACCES: permission denied');
      const error = new FileAccessError(
        'Cannot read Steam files',
        '/var/steam/steamapps',
        originalError
      );

      expect(error.filePath).toBe('/var/steam/steamapps');
      expect(error.originalError?.message).toContain('permission denied');

      // Should help identify permission issues
      const debugInfo = {
        path: error.filePath,
        reason: error.originalError?.message,
        action: 'Check file permissions',
      };

      expect(debugInfo.path).toBeTruthy();
      expect(debugInfo.reason).toContain('permission');
    });
  });
});
