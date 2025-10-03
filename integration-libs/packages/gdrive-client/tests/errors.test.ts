/**
 * Tests for error classes
 */

import { describe, it, expect } from 'vitest';
import {
  DriveClientError,
  OAuth2Error,
  TokenExpiredError,
  AuthenticationError,
  DriveAPIError,
  FileNotFoundError,
  NetworkError,
  InvalidConfigError,
  RateLimitError,
  QuotaExceededError,
} from '../src/errors.js';

describe('Error Classes', () => {
  describe('DriveClientError', () => {
    it('should create error with message and code', () => {
      const error = new DriveClientError('Test error', 'TEST_CODE');

      expect(error).toBeInstanceOf(Error);
      expect(error).toBeInstanceOf(DriveClientError);
      expect(error.message).toBe('Test error');
      expect(error.code).toBe('TEST_CODE');
      expect(error.name).toBe('DriveClientError');
    });

    it('should have correct prototype chain', () => {
      const error = new DriveClientError('Test', 'CODE');

      expect(error instanceof DriveClientError).toBe(true);
      expect(error instanceof Error).toBe(true);
    });

    it('should preserve stack trace', () => {
      const error = new DriveClientError('Test', 'CODE');

      expect(error.stack).toBeDefined();
      expect(error.stack).toContain('DriveClientError');
    });
  });

  describe('OAuth2Error', () => {
    it('should create error with status code', () => {
      const error = new OAuth2Error('Auth failed', 401);

      expect(error).toBeInstanceOf(OAuth2Error);
      expect(error).toBeInstanceOf(DriveClientError);
      expect(error.message).toBe('Auth failed');
      expect(error.code).toBe('OAUTH2_ERROR');
      expect(error.statusCode).toBe(401);
      expect(error.name).toBe('OAuth2Error');
    });

    it('should create error with response data', () => {
      const responseData = { error: 'invalid_grant' };
      const error = new OAuth2Error('Token invalid', 400, responseData);

      expect(error.statusCode).toBe(400);
      expect(error.response).toEqual(responseData);
    });
  });

  describe('TokenExpiredError', () => {
    it('should create error with default message', () => {
      const error = new TokenExpiredError();

      expect(error).toBeInstanceOf(TokenExpiredError);
      expect(error.message).toBe('Access token has expired');
      expect(error.code).toBe('TOKEN_EXPIRED');
      expect(error.name).toBe('TokenExpiredError');
    });

    it('should create error with custom message', () => {
      const error = new TokenExpiredError('Custom expired message');

      expect(error.message).toBe('Custom expired message');
    });
  });

  describe('AuthenticationError', () => {
    it('should create error with default message', () => {
      const error = new AuthenticationError();

      expect(error).toBeInstanceOf(AuthenticationError);
      expect(error.message).toBe('Authentication failed');
      expect(error.code).toBe('AUTHENTICATION_ERROR');
    });

    it('should create error with custom message', () => {
      const error = new AuthenticationError('Invalid credentials');

      expect(error.message).toBe('Invalid credentials');
    });
  });

  describe('DriveAPIError', () => {
    it('should create error with message', () => {
      const error = new DriveAPIError('API call failed');

      expect(error).toBeInstanceOf(DriveAPIError);
      expect(error.message).toBe('API call failed');
      expect(error.code).toBe('DRIVE_API_ERROR');
      expect(error.name).toBe('DriveAPIError');
    });

    it('should create error with status code and response', () => {
      const response = { error: { message: 'Not found' } };
      const error = new DriveAPIError('File not found', 404, response);

      expect(error.statusCode).toBe(404);
      expect(error.response).toEqual(response);
    });
  });

  describe('FileNotFoundError', () => {
    it('should create error with file ID', () => {
      const error = new FileNotFoundError('File not found', 'abc123');

      expect(error).toBeInstanceOf(FileNotFoundError);
      expect(error.message).toBe('File not found');
      expect(error.fileId).toBe('abc123');
      expect(error.code).toBe('FILE_NOT_FOUND');
      expect(error.name).toBe('FileNotFoundError');
    });

    it('should create error without file ID', () => {
      const error = new FileNotFoundError('File not found');

      expect(error.fileId).toBeUndefined();
    });
  });

  describe('NetworkError', () => {
    it('should create error with message', () => {
      const error = new NetworkError('Connection failed');

      expect(error).toBeInstanceOf(NetworkError);
      expect(error.message).toBe('Connection failed');
      expect(error.code).toBe('NETWORK_ERROR');
      expect(error.name).toBe('NetworkError');
    });

    it('should create error with original error', () => {
      const originalError = new Error('ECONNREFUSED');
      const error = new NetworkError('Connection failed', originalError);

      expect(error.originalError).toBe(originalError);
      expect(error.originalError?.message).toBe('ECONNREFUSED');
    });
  });

  describe('InvalidConfigError', () => {
    it('should create error with message', () => {
      const error = new InvalidConfigError('Invalid OAuth config');

      expect(error).toBeInstanceOf(InvalidConfigError);
      expect(error.message).toBe('Invalid OAuth config');
      expect(error.code).toBe('INVALID_CONFIG');
      expect(error.name).toBe('InvalidConfigError');
    });
  });

  describe('RateLimitError', () => {
    it('should create error with default message', () => {
      const error = new RateLimitError();

      expect(error).toBeInstanceOf(RateLimitError);
      expect(error.message).toBe('Rate limit exceeded');
      expect(error.code).toBe('RATE_LIMIT_ERROR');
      expect(error.name).toBe('RateLimitError');
    });

    it('should create error with retry-after header', () => {
      const error = new RateLimitError('Rate limit', 60);

      expect(error.retryAfter).toBe(60);
    });
  });

  describe('QuotaExceededError', () => {
    it('should create error with default message', () => {
      const error = new QuotaExceededError();

      expect(error).toBeInstanceOf(QuotaExceededError);
      expect(error.message).toBe('Storage quota exceeded');
      expect(error.code).toBe('QUOTA_EXCEEDED');
      expect(error.name).toBe('QuotaExceededError');
    });

    it('should create error with custom message', () => {
      const error = new QuotaExceededError('Out of storage');

      expect(error.message).toBe('Out of storage');
    });
  });

  describe('Error Inheritance', () => {
    it('should maintain correct instanceof relationships', () => {
      const errors = [
        new OAuth2Error('test'),
        new TokenExpiredError(),
        new AuthenticationError(),
        new DriveAPIError('test'),
        new FileNotFoundError('test'),
        new NetworkError('test'),
        new InvalidConfigError('test'),
        new RateLimitError(),
        new QuotaExceededError(),
      ];

      errors.forEach((error) => {
        expect(error).toBeInstanceOf(Error);
        expect(error).toBeInstanceOf(DriveClientError);
      });
    });

    it('should allow catching specific error types', () => {
      const throwOAuth2 = () => {
        throw new OAuth2Error('Auth failed');
      };

      const throwFileNotFound = () => {
        throw new FileNotFoundError('Not found', 'file123');
      };

      expect(() => throwOAuth2()).toThrow(OAuth2Error);
      expect(() => throwFileNotFound()).toThrow(FileNotFoundError);

      // All should be catchable as DriveClientError
      expect(() => throwOAuth2()).toThrow(DriveClientError);
      expect(() => throwFileNotFound()).toThrow(DriveClientError);
    });

    it('should allow error code filtering', () => {
      const errors = [
        new OAuth2Error('test'),
        new TokenExpiredError(),
        new DriveAPIError('test'),
        new FileNotFoundError('test'),
        new RateLimitError(),
      ];

      const codes = errors.map((e) => e.code);

      expect(codes).toContain('OAUTH2_ERROR');
      expect(codes).toContain('TOKEN_EXPIRED');
      expect(codes).toContain('DRIVE_API_ERROR');
      expect(codes).toContain('FILE_NOT_FOUND');
      expect(codes).toContain('RATE_LIMIT_ERROR');
      expect(new Set(codes).size).toBe(5); // All unique
    });
  });

  describe('Error Messages', () => {
    it('should preserve error messages through inheritance', () => {
      const customMessage = 'Very specific error message';
      const error = new OAuth2Error(customMessage);

      expect(error.message).toBe(customMessage);
      expect(error.toString()).toContain(customMessage);
    });

    it('should include error name in toString', () => {
      const errors = [
        new OAuth2Error('test'),
        new FileNotFoundError('test'),
        new NetworkError('test'),
        new RateLimitError('test'),
      ];

      errors.forEach((error) => {
        const str = error.toString();
        expect(str).toContain(error.name);
        expect(str).toContain('test');
      });
    });
  });

  describe('Error Context', () => {
    it('should provide context for OAuth errors', () => {
      const response = {
        error: 'invalid_grant',
        error_description: 'Token has been revoked',
      };
      const error = new OAuth2Error('OAuth failed', 400, response);

      expect(error.statusCode).toBe(400);
      expect(error.response).toEqual(response);
      expect(error.code).toBe('OAUTH2_ERROR');
    });

    it('should provide context for file errors', () => {
      const error = new FileNotFoundError('Document missing', 'doc_xyz123');

      expect(error.fileId).toBe('doc_xyz123');
      expect(error.message).toContain('Document missing');
    });

    it('should provide context for network errors', () => {
      const originalError = new Error('ETIMEDOUT');
      originalError.stack = 'Network timeout stack';

      const error = new NetworkError('Request timeout', originalError);

      expect(error.originalError).toBeDefined();
      expect(error.originalError?.message).toBe('ETIMEDOUT');
      expect(error.originalError?.stack).toContain('timeout');
    });

    it('should provide retry-after for rate limit errors', () => {
      const error = new RateLimitError('Too many requests', 120);

      expect(error.retryAfter).toBe(120);
      expect(error.code).toBe('RATE_LIMIT_ERROR');
    });
  });
});
