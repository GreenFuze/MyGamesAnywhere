/**
 * Custom error classes for Google Drive client
 */

/**
 * Base error class for all Google Drive client errors
 */
export class DriveClientError extends Error {
  constructor(
    message: string,
    public readonly code: string
  ) {
    super(message);
    this.name = 'DriveClientError';
    Object.setPrototypeOf(this, DriveClientError.prototype);
  }
}

/**
 * OAuth 2.0 authentication errors
 */
export class OAuth2Error extends DriveClientError {
  constructor(
    message: string,
    public readonly statusCode?: number,
    public readonly response?: unknown
  ) {
    super(message, 'OAUTH2_ERROR');
    this.name = 'OAuth2Error';
    Object.setPrototypeOf(this, OAuth2Error.prototype);
  }
}

/**
 * Token expired or invalid
 */
export class TokenExpiredError extends DriveClientError {
  constructor(message: string = 'Access token has expired') {
    super(message, 'TOKEN_EXPIRED');
    this.name = 'TokenExpiredError';
    Object.setPrototypeOf(this, TokenExpiredError.prototype);
  }
}

/**
 * Invalid token or authentication failure
 */
export class AuthenticationError extends DriveClientError {
  constructor(message: string = 'Authentication failed') {
    super(message, 'AUTHENTICATION_ERROR');
    this.name = 'AuthenticationError';
    Object.setPrototypeOf(this, AuthenticationError.prototype);
  }
}

/**
 * Google Drive API errors
 */
export class DriveAPIError extends DriveClientError {
  constructor(
    message: string,
    public readonly statusCode?: number,
    public readonly response?: unknown
  ) {
    super(message, 'DRIVE_API_ERROR');
    this.name = 'DriveAPIError';
    Object.setPrototypeOf(this, DriveAPIError.prototype);
  }
}

/**
 * File not found in Google Drive
 */
export class FileNotFoundError extends DriveClientError {
  constructor(
    message: string,
    public readonly fileId?: string
  ) {
    super(message, 'FILE_NOT_FOUND');
    this.name = 'FileNotFoundError';
    Object.setPrototypeOf(this, FileNotFoundError.prototype);
  }
}

/**
 * Network or connection errors
 */
export class NetworkError extends DriveClientError {
  constructor(
    message: string,
    public readonly originalError?: Error
  ) {
    super(message, 'NETWORK_ERROR');
    this.name = 'NetworkError';
    Object.setPrototypeOf(this, NetworkError.prototype);
  }
}

/**
 * Invalid configuration
 */
export class InvalidConfigError extends DriveClientError {
  constructor(message: string) {
    super(message, 'INVALID_CONFIG');
    this.name = 'InvalidConfigError';
    Object.setPrototypeOf(this, InvalidConfigError.prototype);
  }
}

/**
 * Rate limit exceeded
 */
export class RateLimitError extends DriveClientError {
  constructor(
    message: string = 'Rate limit exceeded',
    public readonly retryAfter?: number
  ) {
    super(message, 'RATE_LIMIT_ERROR');
    this.name = 'RateLimitError';
    Object.setPrototypeOf(this, RateLimitError.prototype);
  }
}

/**
 * Quota exceeded
 */
export class QuotaExceededError extends DriveClientError {
  constructor(message: string = 'Storage quota exceeded') {
    super(message, 'QUOTA_EXCEEDED');
    this.name = 'QuotaExceededError';
    Object.setPrototypeOf(this, QuotaExceededError.prototype);
  }
}
