/**
 * Test LaunchBox Game Identifier
 *
 * This script tests the LaunchBox identifier with various game filenames.
 *
 * Usage:
 * npm run test:launchbox
 */

import { LaunchBoxIdentifier } from './src/index.js';

/**
 * Test filenames from user
 */
const TEST_FILENAMES = [
  'AD&D - Eye of the Beholder (U).srm',
  'Devil May Cry - HD Collection (USA) (En,Ja,Fr,De,Es,It).ps3',
  'Ratchet.and.Clank.Rift.Apart.v1.922.0.0.zip',
  'setup_alone_in_the_dark_3_1.0_cs_(28191).exe',
  'setup_lego_batman_1.0_(18156).exe',
  'setup_lego_batman_1.0_(18156)-1.bin',
  'setup_pikuniku_1.0.5-gog1_(64bit)_(27230).exe',
  'SONIC MEGA COLLECTION PLUS.zip',
  'God of War.7z',
  'HangMan.exe', // Custom game - likely no match
  'GO.BAT', // DOSBox game
];

async function testLaunchBoxIdentifier() {
  console.log('=== LaunchBox Game Identifier Test ===\n');

  const identifier = new LaunchBoxIdentifier({
    autoDownload: true,
    minConfidence: 0.5,
    maxResults: 5,
  });

  // Check if metadata is ready
  const isReady = await identifier.isReady();

  if (!isReady) {
    console.log('⚠️  LaunchBox metadata not found. Downloading and parsing...\n');
    console.log('This will take several minutes (450MB XML files).\n');

    const startTime = Date.now();
    await identifier.update();
    const duration = Math.round((Date.now() - startTime) / 1000);

    console.log(`\n⏱️  Import took ${duration} seconds\n`);
  } else {
    console.log('✅ LaunchBox metadata already available\n');

    // Show stats
    const stats = identifier.getStats();
    console.log('Database Statistics:');
    console.log(`  Games: ${stats.games.toLocaleString()}`);
    console.log(`  Platforms: ${stats.platforms.toLocaleString()}`);
    console.log(`  Genres: ${stats.genres.toLocaleString()}`);
    console.log(`  Files: ${stats.files.toLocaleString()}`);
    console.log();
  }

  console.log('=== Testing Game Identification ===\n');

  let identified = 0;
  let notFound = 0;

  for (const filename of TEST_FILENAMES) {
    // Create mock detected game
    const detectedGame = {
      id: filename,
      name: filename,
      type: 'unknown',
      path: `/test/${filename}`,
      size: 0,
      confidence: 0.8,
    };

    try {
      const result = await identifier.identify(detectedGame);

      if (result.metadata) {
        identified++;
        console.log(`\n📦 ${filename}`);
        console.log(`   ✅ ${result.metadata.title}`);
        console.log(`   Platform: ${result.metadata.platform || 'Unknown'}`);
        console.log(`   Confidence: ${Math.round(result.matchConfidence * 100)}%`);
        if (result.metadata.developer) {
          console.log(`   Developer: ${result.metadata.developer}`);
        }
        if (result.metadata.releaseDate) {
          console.log(`   Released: ${result.metadata.releaseDate}`);
        }
      } else {
        notFound++;
        console.log(`\n📦 ${filename}`);
        console.log(`   ❌ No match found`);
      }
    } catch (error) {
      console.error(`\n❌ Error identifying ${filename}:`, error);
    }
  }

  console.log('\n=== Results ===');
  console.log(`Tested: ${TEST_FILENAMES.length} files`);
  console.log(`Identified: ${identified} (${Math.round((identified / TEST_FILENAMES.length) * 100)}%)`);
  console.log(`Not found: ${notFound}`);

  console.log('\n✨ Test complete!');

  // Close database
  identifier.close();
}

// Handle errors
process.on('unhandledRejection', (error) => {
  console.error('❌ Unhandled error:', error);
  process.exit(1);
});

// Run
testLaunchBoxIdentifier().catch((error) => {
  console.error('❌ Fatal error:', error);
  process.exit(1);
});
