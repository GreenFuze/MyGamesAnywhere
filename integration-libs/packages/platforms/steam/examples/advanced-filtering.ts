/**
 * Advanced Filtering Example
 *
 * This example shows how to filter and analyze your Steam library.
 */

import { scanSteamLibrary } from '@mygamesanywhere/steam-scanner';

async function analyzeLibrary() {
  console.log('Scanning and analyzing Steam library...\n');

  const result = await scanSteamLibrary();

  // 1. Find largest games
  console.log('=== TOP 10 LARGEST GAMES ===');
  const sortedBySize = [...result.games].sort((a, b) => {
    return parseInt(b.sizeOnDisk) - parseInt(a.sizeOnDisk);
  });

  sortedBySize.slice(0, 10).forEach((game, index) => {
    const sizeGB = (parseInt(game.sizeOnDisk) / (1024 ** 3)).toFixed(2);
    console.log(`${index + 1}. ${game.name} - ${sizeGB} GB`);
  });

  // 2. Calculate total library size
  console.log('\n=== LIBRARY STATISTICS ===');
  const totalSize = result.games.reduce((sum, game) => {
    return sum + parseInt(game.sizeOnDisk);
  }, 0);
  const totalSizeGB = (totalSize / (1024 ** 3)).toFixed(2);
  console.log(`Total games: ${result.games.length}`);
  console.log(`Total size: ${totalSizeGB} GB`);

  // 3. Find games by library folder
  console.log('\n=== GAMES PER LIBRARY FOLDER ===');
  const gamesByLibrary = new Map<string, typeof result.games>();

  result.games.forEach(game => {
    const library = game.libraryPath;
    if (!gamesByLibrary.has(library)) {
      gamesByLibrary.set(library, []);
    }
    gamesByLibrary.get(library)!.push(game);
  });

  gamesByLibrary.forEach((games, library) => {
    const librarySize = games.reduce((sum, g) => sum + parseInt(g.sizeOnDisk), 0);
    const librarySizeGB = (librarySize / (1024 ** 3)).toFixed(2);
    console.log(`\n${library}`);
    console.log(`  Games: ${games.length}`);
    console.log(`  Size: ${librarySizeGB} GB`);
  });

  // 4. Find recently updated games
  console.log('\n=== RECENTLY UPDATED GAMES ===');
  const sortedByUpdate = [...result.games].sort((a, b) => {
    return b.lastUpdated - a.lastUpdated;
  });

  sortedByUpdate.slice(0, 5).forEach((game, index) => {
    const date = new Date(game.lastUpdated * 1000);
    console.log(`${index + 1}. ${game.name}`);
    console.log(`   Last updated: ${date.toLocaleDateString()}`);
  });

  // 5. Find games by size range
  console.log('\n=== GAMES BY SIZE RANGE ===');
  const ranges = [
    { min: 0, max: 1, label: 'Under 1 GB' },
    { min: 1, max: 10, label: '1-10 GB' },
    { min: 10, max: 50, label: '10-50 GB' },
    { min: 50, max: Infinity, label: 'Over 50 GB' },
  ];

  ranges.forEach(range => {
    const gamesInRange = result.games.filter(game => {
      const sizeGB = parseInt(game.sizeOnDisk) / (1024 ** 3);
      return sizeGB >= range.min && sizeGB < range.max;
    });
    console.log(`${range.label}: ${gamesInRange.length} games`);
  });

  // 6. Check if specific games are installed
  console.log('\n=== CHECK SPECIFIC GAMES ===');
  const popularGames = [
    { appid: '440', name: 'Team Fortress 2' },
    { appid: '730', name: 'Counter-Strike 2' },
    { appid: '570', name: 'Dota 2' },
    { appid: '1172470', name: 'Apex Legends' },
  ];

  popularGames.forEach(({ appid, name }) => {
    const installed = result.games.some(g => g.appid === appid);
    console.log(`${name}: ${installed ? '✅ Installed' : '❌ Not installed'}`);
  });
}

// Run the example
analyzeLibrary().catch(error => {
  console.error('Error:', error.message);
  process.exit(1);
});
