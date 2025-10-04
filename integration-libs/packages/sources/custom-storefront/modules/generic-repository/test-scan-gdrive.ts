/**
 * Test Google Drive Game Scanning
 *
 * This script scans a Google Drive folder for games using the generic repository scanner.
 *
 * Prerequisites:
 * 1. Authenticate with Google Drive first: cd ../gdrive-client && npm run test:auth
 * 2. Have some game files in your Google Drive
 * 3. Run this script
 */

import { DriveClient, GDriveAuth } from '@mygamesanywhere/gdrive-client';
import { GDriveRepository, RepositoryScanner } from './src/index.js';

async function testGDriveScan() {
  console.log('=== Google Drive Game Scanner Test ===\n');

  // Authenticate with Google Drive
  const auth = new GDriveAuth();

  if (!(await auth.isAuthenticated())) {
    console.error('❌ Not authenticated with Google Drive!');
    console.error('Run: cd ../gdrive-client && npm run test:auth\n');
    process.exit(1);
  }

  console.log('✅ Authenticated with Google Drive\n');

  // Create Drive client
  const client = new DriveClient({
    oauth: {
      clientId: '', // Not needed, auth has it
      clientSecret: '',
      redirectUri: '',
    },
    tokenStorage: auth.getTokenStorage(),
  });

  // Override oauth client with authenticated one
  (client as any).oauth = auth.getOAuthClient();

  // Get folder to scan (default to root)
  const folderId = process.argv[2] || 'root';

  if (folderId === 'root') {
    console.log('📂 Scanning root folder of Google Drive');
    console.log('   (To scan a specific folder, pass folder ID as argument)\n');
  } else {
    console.log(`📂 Scanning folder: ${folderId}\n`);
  }

  // Create GDrive repository
  const repository = new GDriveRepository(client, folderId);

  // Create scanner
  const scanner = new RepositoryScanner(repository, {
    maxDepth: 100, // No practical limit - scan entire directory tree
    includeHidden: false,
  });

  console.log('🔍 Scanning for games (this may take a minute for large folders)...\n');

  // Scan
  const startTime = Date.now();

  // Show progress every 5 seconds
  const progressInterval = setInterval(() => {
    console.log(`⏳ Still scanning... (${Math.round((Date.now() - startTime) / 1000)}s elapsed)`);
  }, 5000);

  const result = await scanner.scan();
  clearInterval(progressInterval);

  const duration = Date.now() - startTime;

  // Display results
  console.log('=== Scan Results ===\n');
  console.log(`Duration: ${duration}ms`);
  console.log(`Files scanned: ${result.filesScanned}`);
  console.log(`Directories scanned: ${result.directoriesScanned}`);
  console.log(`Games found: ${result.games.length}\n`);

  if (result.errors.length > 0) {
    console.log(`⚠️  Errors encountered: ${result.errors.length}`);
    for (const error of result.errors.slice(0, 5)) {
      console.log(`  - ${error.path}: ${error.message}`);
    }
    console.log();
  }

  if (result.games.length === 0) {
    console.log('No games found in this Google Drive folder.');
    console.log('\nTips:');
    console.log('- Make sure you have game files in your Google Drive');
    console.log('- Try scanning a specific folder with games');
    console.log('- Supported formats: installers (.exe, .msi, .pkg), ROMs, archives, etc.\n');
    return;
  }

  // Group games by type
  const gamesByType = new Map<string, typeof result.games>();
  for (const game of result.games) {
    if (!gamesByType.has(game.type)) {
      gamesByType.set(game.type, []);
    }
    gamesByType.get(game.type)!.push(game);
  }

  // Display games by type
  for (const [type, games] of gamesByType) {
    console.log(`📦 ${type.toUpperCase().replace(/_/g, ' ')} (${games.length}):`);
    for (const game of games.slice(0, 5)) {
      console.log(`  - ${game.name} (confidence: ${Math.round(game.confidence * 100)}%)`);
    }
    if (games.length > 5) {
      console.log(`  ... and ${games.length - 5} more`);
    }
    console.log();
  }

  console.log('✨ Scan complete!\n');
  console.log('Next steps:');
  console.log('- Download and install games using the game IDs');
  console.log('- Fetch metadata from IGDB');
  console.log('- Sync library to cloud storage\n');
}

// Handle errors
process.on('unhandledRejection', (error) => {
  console.error('❌ Unhandled error:', error);
  process.exit(1);
});

// Run
testGDriveScan().catch((error) => {
  console.error('❌ Fatal error:', error);
  process.exit(1);
});
