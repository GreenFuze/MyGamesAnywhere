/**
 * Error Handling Example
 *
 * This example demonstrates how to properly handle different error types.
 */

import {
  scanSteamLibrary,
  SteamNotFoundError,
  FileAccessError,
  VDFParseError,
  SteamScannerError,
} from '@mygamesanywhere/steam-scanner';

async function scanWithErrorHandling() {
  try {
    console.log('Attempting to scan Steam library...\n');

    const result = await scanSteamLibrary();

    console.log('✅ Scan successful!');
    console.log(`Found ${result.games.length} games`);

  } catch (error) {
    // Handle specific error types
    if (error instanceof SteamNotFoundError) {
      console.error('❌ Steam Not Found');
      console.error('Steam is not installed on this system.');
      console.error('Please install Steam from: https://store.steampowered.com/');
      process.exit(1);

    } else if (error instanceof FileAccessError) {
      console.error('❌ File Access Error');
      console.error(`Cannot access file: ${error.filePath}`);
      console.error('Please check file permissions or if Steam is running.');
      if (error.originalError) {
        console.error(`Reason: ${error.originalError.message}`);
      }
      process.exit(1);

    } else if (error instanceof VDFParseError) {
      console.error('❌ VDF Parse Error');
      console.error(`Failed to parse VDF file: ${error.filePath || 'unknown'}`);
      if (error.line) {
        console.error(`Error at line ${error.line}`);
      }
      console.error('The VDF file may be corrupted.');
      process.exit(1);

    } else if (error instanceof SteamScannerError) {
      // Catch-all for other Steam scanner errors
      console.error('❌ Steam Scanner Error');
      console.error(`Error code: ${error.code}`);
      console.error(`Message: ${error.message}`);
      process.exit(1);

    } else {
      // Unknown error
      console.error('❌ Unexpected Error');
      console.error(error);
      process.exit(1);
    }
  }
}

// Alternative: Handle errors by error code
async function scanWithErrorCodes() {
  try {
    const result = await scanSteamLibrary();
    console.log(`✅ Found ${result.games.length} games`);

  } catch (error) {
    if (error instanceof SteamScannerError) {
      switch (error.code) {
        case 'STEAM_NOT_FOUND':
          console.error('Steam not installed');
          break;
        case 'FILE_ACCESS_ERROR':
          console.error('Cannot access Steam files');
          break;
        case 'VDF_PARSE_ERROR':
          console.error('VDF file corrupted');
          break;
        case 'INVALID_CONFIG':
          console.error('Invalid configuration');
          break;
        default:
          console.error(`Unknown error: ${error.code}`);
      }
    } else {
      console.error('Unexpected error:', error);
    }
  }
}

// Run the example
console.log('Example 1: Error type handling\n');
scanWithErrorHandling().then(() => {
  console.log('\n\nExample 2: Error code handling\n');
  return scanWithErrorCodes();
});
