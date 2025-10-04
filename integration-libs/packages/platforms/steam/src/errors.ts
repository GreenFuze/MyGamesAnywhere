/**
 * Custom error classes for Steam Scanner
 */

/**
 * Base error class for Steam Scanner
 */
export class SteamScannerError extends Error {
  constructor(message: string, public readonly code: string) {
    super(message);
    this.name = 'SteamScannerError';
    Object.setPrototypeOf(this, SteamScannerError.prototype);
  }
}

/**
 * Error thrown when Steam installation is not found
 */
export class SteamNotFoundError extends SteamScannerError {
  constructor(message: string = 'Steam installation not found') {
    super(message, 'STEAM_NOT_FOUND');
    this.name = 'SteamNotFoundError';
    Object.setPrototypeOf(this, SteamNotFoundError.prototype);
  }
}

/**
 * Error thrown when VDF parsing fails
 */
export class VDFParseError extends SteamScannerError {
  constructor(
    message: string,
    public readonly filePath?: string,
    public readonly line?: number
  ) {
    super(message, 'VDF_PARSE_ERROR');
    this.name = 'VDFParseError';
    Object.setPrototypeOf(this, VDFParseError.prototype);
  }
}

/**
 * Error thrown when file operations fail
 */
export class FileAccessError extends SteamScannerError {
  constructor(
    message: string,
    public readonly filePath: string,
    public readonly originalError?: Error
  ) {
    super(message, 'FILE_ACCESS_ERROR');
    this.name = 'FileAccessError';
    Object.setPrototypeOf(this, FileAccessError.prototype);
  }
}

/**
 * Error thrown when configuration is invalid
 */
export class InvalidConfigError extends SteamScannerError {
  constructor(message: string) {
    super(message, 'INVALID_CONFIG');
    this.name = 'InvalidConfigError';
    Object.setPrototypeOf(this, InvalidConfigError.prototype);
  }
}
