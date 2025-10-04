/**
 * Test Game Identifier with Google Drive Games
 *
 * This script:
 * 1. Scans Google Drive folder for games
 * 2. Runs identifier on each detected game
 * 3. Shows match results and accuracy stats
 *
 * Prerequisites:
 * - Authenticated with Google Drive (cd ../gdrive-client && npm run test:auth)
 *
 * Usage:
 * npm run test:gdrive <folder-id>
 *
 * Example:
 * npm run test:gdrive 1Catj9Eo8hthHConFCSOOEx1IPH13ycNH
 */

import { DriveClient, GDriveAuth } from '@mygamesanywhere/gdrive-client';
import { GDriveRepository, RepositoryScanner } from '@mygamesanywhere/generic-repository';
import { LaunchBoxIdentifier } from './src/index.js';

async function testGDriveGames() {
  console.log('=== Game Identifier Test with Google Drive Games ===\n');

  // Get folder ID from args
  const folderId = process.argv[2];

  if (!folderId) {
    console.error('❌ Please provide a Google Drive folder ID');
    console.error('Usage: npm run test:gdrive <folder-id>');
    console.error('Example: npm run test:gdrive 1Catj9Eo8hthHConFCSOOEx1IPH13ycNH\n');
    process.exit(1);
  }

  // Step 1: Authenticate with Google Drive
  console.log('📂 Step 1: Authenticating with Google Drive...\n');

  const auth = new GDriveAuth();

  if (!(await auth.isAuthenticated())) {
    console.error('❌ Not authenticated with Google Drive!');
    console.error('Run: cd ../gdrive-client && npm run test:auth\n');
    process.exit(1);
  }

  console.log('✅ Authenticated with Google Drive\n');

  // Step 2: Scan Google Drive for games
  console.log('📂 Step 2: Scanning Google Drive for games...\n');
  console.log(`Folder ID: ${folderId}\n`);

  const client = new DriveClient({
    oauth: {
      clientId: '',
      clientSecret: '',
      redirectUri: '',
    },
    tokenStorage: auth.getTokenStorage(),
  });

  // Override oauth client
  (client as any).oauth = auth.getOAuthClient();

  const repository = new GDriveRepository(client, folderId);
  const scanner = new RepositoryScanner(repository, {
    maxDepth: 100,
    includeHidden: false,
  });

  console.log('🔍 Scanning... (this may take a few minutes)\n');

  const startScan = Date.now();
  const progressInterval = setInterval(() => {
    console.log(`⏳ Still scanning... (${Math.round((Date.now() - startScan) / 1000)}s elapsed)`);
  }, 10000);

  const scanResult = await scanner.scan();
  clearInterval(progressInterval);

  const scanDuration = Math.round((Date.now() - startScan) / 1000);

  console.log(`\n✅ Scan complete in ${scanDuration}s`);
  console.log(`   Files scanned: ${scanResult.filesScanned}`);
  console.log(`   Directories scanned: ${scanResult.directoriesScanned}`);
  console.log(`   Games found: ${scanResult.games.length}\n`);

  if (scanResult.games.length === 0) {
    console.log('No games found. Exiting.\n');
    process.exit(0);
  }

  // Step 3: Initialize identifier
  console.log('📚 Step 3: Initializing LaunchBox identifier...\n');

  const identifier = new LaunchBoxIdentifier({
    autoDownload: true,
    minConfidence: 0.3,
    maxResults: 5,
  });

  const isReady = await identifier.isReady();

  if (!isReady) {
    console.log('⚠️  LaunchBox metadata not found. Downloading and parsing...\n');
    console.log('⏰ This will take 5-10 minutes on first run (450MB download + parsing).\n');
    console.log('Press Ctrl+C to cancel, or wait for download to complete...\n');

    const startDownload = Date.now();
    await identifier.update();
    const downloadDuration = Math.round((Date.now() - startDownload) / 1000);

    console.log(`\n✅ Metadata imported in ${downloadDuration}s\n`);
  } else {
    console.log('✅ LaunchBox metadata already available\n');

    const stats = identifier.getStats();
    console.log('Database Statistics:');
    console.log(`  Games: ${stats.games.toLocaleString()}`);
    console.log(`  Platforms: ${stats.platforms.toLocaleString()}`);
    console.log(`  Genres: ${stats.genres.toLocaleString()}`);
    console.log();
  }

  // Step 4: Identify each game
  console.log('🎮 Step 4: Identifying games...\n');

  let identified = 0;
  let notFound = 0;
  let totalConfidence = 0;

  const resultsByType = new Map<string, { found: number; total: number }>();
  const identifiedGames: any[] = [];

  for (let i = 0; i < scanResult.games.length; i++) {
    const game = scanResult.games[i];

    // Track by type
    if (!resultsByType.has(game.type)) {
      resultsByType.set(game.type, { found: 0, total: 0 });
    }
    const typeStats = resultsByType.get(game.type)!;
    typeStats.total++;

    // Identify
    try {
      const result = await identifier.identify(game);

      if (result.metadata && result.matchConfidence >= 0.3) {
        identified++;
        typeStats.found++;
        totalConfidence += result.matchConfidence;

        identifiedGames.push({
          original: game.name,
          matched: result.metadata.title,
          platform: result.metadata.platform,
          confidence: result.matchConfidence,
          type: game.type,
        });

        // Show progress every 10 games
        if ((i + 1) % 10 === 0) {
          console.log(`  Processed ${i + 1}/${scanResult.games.length} games...`);
        }
      } else {
        notFound++;
      }
    } catch (error) {
      console.error(`  ❌ Error identifying ${game.name}:`, error);
      notFound++;
    }
  }

  console.log(`\n✅ Identification complete!\n`);

  // Step 5: Show results
  console.log('=== Results Summary ===\n');
  console.log(`Total games scanned: ${scanResult.games.length}`);
  console.log(`Identified: ${identified} (${Math.round((identified / scanResult.games.length) * 100)}%)`);
  console.log(`Not found: ${notFound} (${Math.round((notFound / scanResult.games.length) * 100)}%)`);

  if (identified > 0) {
    const avgConfidence = totalConfidence / identified;
    console.log(`Average confidence: ${Math.round(avgConfidence * 100)}%`);
  }

  console.log('\n=== Results by Game Type ===\n');

  for (const [type, stats] of resultsByType) {
    const accuracy = Math.round((stats.found / stats.total) * 100);
    console.log(`${type.replace(/_/g, ' ').toUpperCase()}:`);
    console.log(`  Total: ${stats.total}`);
    console.log(`  Identified: ${stats.found} (${accuracy}%)`);
    console.log();
  }

  // Show sample matches
  console.log('=== Sample Matches ===\n');

  // Sort by confidence (high to low)
  identifiedGames.sort((a, b) => b.confidence - a.confidence);

  // Show top 10 matches
  console.log('Top 10 Matches (Highest Confidence):');
  for (const match of identifiedGames.slice(0, 10)) {
    console.log(`\n  📦 ${match.original}`);
    console.log(`     ✅ ${match.matched}`);
    console.log(`     Platform: ${match.platform || 'Unknown'}`);
    console.log(`     Confidence: ${Math.round(match.confidence * 100)}%`);
  }

  // Show bottom 10 matches (lowest confidence)
  if (identifiedGames.length > 10) {
    console.log('\n\nBottom 10 Matches (Lowest Confidence):');
    for (const match of identifiedGames.slice(-10).reverse()) {
      console.log(`\n  📦 ${match.original}`);
      console.log(`     ✅ ${match.matched}`);
      console.log(`     Platform: ${match.platform || 'Unknown'}`);
      console.log(`     Confidence: ${Math.round(match.confidence * 100)}%`);
    }
  }

  console.log('\n\n=== Test Complete! ===\n');

  // Save results to file (optional)
  const resultsPath = `./test-results-${Date.now()}.json`;
  const fs = await import('fs');
  fs.writeFileSync(
    resultsPath,
    JSON.stringify(
      {
        scan: {
          folderId,
          filesScanned: scanResult.filesScanned,
          directoriesScanned: scanResult.directoriesScanned,
          gamesFound: scanResult.games.length,
          duration: scanDuration,
        },
        identification: {
          identified,
          notFound,
          accuracy: Math.round((identified / scanResult.games.length) * 100),
          avgConfidence: identified > 0 ? totalConfidence / identified : 0,
        },
        byType: Object.fromEntries(resultsByType),
        matches: identifiedGames,
      },
      null,
      2
    )
  );

  console.log(`📄 Detailed results saved to: ${resultsPath}\n`);

  // Close database
  identifier.close();
}

// Handle errors
process.on('unhandledRejection', (error) => {
  console.error('❌ Unhandled error:', error);
  process.exit(1);
});

// Run
testGDriveGames().catch((error) => {
  console.error('❌ Fatal error:', error);
  process.exit(1);
});
