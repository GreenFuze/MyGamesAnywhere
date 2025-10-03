/**
 * Basic Usage Example
 *
 * This example shows the simplest way to scan your Steam library.
 */

import { scanSteamLibrary } from '@mygamesanywhere/steam-scanner';

async function basicScan() {
  try {
    console.log('Scanning Steam library...\n');

    const result = await scanSteamLibrary();

    console.log('✅ Scan complete!');
    console.log(`Steam Path: ${result.steamPath}`);
    console.log(`Library Folders: ${result.libraryFolders.length}`);
    console.log(`Games Found: ${result.games.length}`);
    console.log(`Scan Duration: ${result.scanDuration}ms\n`);

    // Display first 10 games
    const gamesToShow = result.games.slice(0, 10);
    console.log(`First ${gamesToShow.length} games:`);
    gamesToShow.forEach((game, index) => {
      const sizeGB = (parseInt(game.sizeOnDisk) / (1024 ** 3)).toFixed(2);
      console.log(`  ${index + 1}. ${game.name}`);
      console.log(`     App ID: ${game.appid}`);
      console.log(`     Size: ${sizeGB} GB`);
      console.log(`     Path: ${game.libraryPath}/${game.installdir}\n`);
    });

  } catch (error) {
    console.error('❌ Error scanning Steam:', error.message);
  }
}

// Run the example
basicScan();
