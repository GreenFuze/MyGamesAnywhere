/**
 * Test Steam Web API Integration
 *
 * This script tests the Steam Web API features using centralized configuration.
 *
 * Setup:
 * 1. Follow SETUP.md in project root to configure ~/.mygamesanywhere/config.json
 * 2. Add your Steam API key and Steam username
 * 3. Run: npm run test:steam-api
 *
 * Or use environment variables:
 * STEAM_API_KEY=your-key STEAM_USERNAME=your-username npm run test:steam-api
 */

import { SteamClient, scanSteamLibrary } from './src/index.js';
import { getConfigManager } from '@mygamesanywhere/config';

async function testSteamWebAPI() {
  console.log('=== Steam Web API Test ===\n');

  // Load configuration from centralized config
  const configManager = getConfigManager();
  await configManager.load();

  const steamConfig = configManager.getSteam();

  if (!steamConfig?.apiKey || !steamConfig?.username) {
    console.error('❌ Steam not configured!\n');
    console.error('Please configure Steam in one of these ways:\n');
    console.error('Option 1: Create ~/.mygamesanywhere/config.json');
    console.error('  See: SETUP.md in project root\n');
    console.error('Option 2: Set environment variables');
    console.error('  STEAM_API_KEY=your-key');
    console.error('  STEAM_USERNAME=your-username\n');
    console.error('Get credentials:');
    console.error('  API Key: https://steamcommunity.com/dev/apikey');
    console.error('  Username: Your Steam username (e.g., "greenfuze")\n');
    process.exit(1);
  }

  const apiKey = steamConfig.apiKey;
  const username = steamConfig.username;

  console.log('✅ Configuration loaded from:', configManager.getConfigPath());
  console.log(`API Key: ${apiKey.substring(0, 8)}...`);
  console.log(`Steam Username: ${username}\n`);

  // Create client
  const client = new SteamClient({ apiKey, username });

  try {
    // Test 1: Get owned games
    console.log('📚 Fetching owned games from Steam Web API...');
    const ownedGames = await client.getOwnedGames();

    console.log(`\n✅ Success! You own ${ownedGames.length} games\n`);

    // Show summary
    const totalPlaytime = ownedGames.reduce((sum, g) => sum + g.playtime_forever, 0);
    const totalHours = Math.floor(totalPlaytime / 60);

    console.log('📊 Library Summary:');
    console.log(`  Total games: ${ownedGames.length}`);
    console.log(`  Total playtime: ${totalHours} hours (${Math.floor(totalHours / 24)} days)\n`);

    // Show top 10 most played
    const topPlayed = [...ownedGames]
      .sort((a, b) => b.playtime_forever - a.playtime_forever)
      .slice(0, 10);

    console.log('🏆 Top 10 Most Played:');
    topPlayed.forEach((game, i) => {
      const hours = Math.floor(game.playtime_forever / 60);
      console.log(`  ${i + 1}. ${game.name}`);
      console.log(`     ${hours} hours (App ID: ${game.appid})`);
    });
    console.log();

    // Test 2: Get recently played
    console.log('🎮 Fetching recently played games...');
    const recentGames = await client.getRecentlyPlayedGames(5);

    if (recentGames.length > 0) {
      console.log(`\n✅ Recently played (${recentGames.length}):\n`);
      recentGames.forEach((game, i) => {
        const hours = Math.floor(game.playtime_forever / 60);
        console.log(`  ${i + 1}. ${game.name} - ${hours} hours total`);
      });
      console.log();
    } else {
      console.log('\n⚠️  No recently played games\n');
    }

    // Test 3: Combine with local scan
    console.log('🔍 Scanning locally installed games...');
    const scanResult = await scanSteamLibrary();

    console.log(`\n✅ Found ${scanResult.games.length} installed games locally\n`);

    // Combine data
    console.log('🔗 Combining Web API + Local Scan...\n');

    const installedSet = new Set(scanResult.games.map(g => g.appid));
    const installed = ownedGames.filter(g => installedSet.has(g.appid.toString()));
    const notInstalled = ownedGames.filter(g => !installedSet.has(g.appid.toString()));

    console.log('📈 Combined Statistics:');
    console.log(`  Total owned: ${ownedGames.length}`);
    console.log(`  Installed: ${installed.length}`);
    console.log(`  Not installed: ${notInstalled.length}`);
    console.log(`  Installation rate: ${Math.floor((installed.length / ownedGames.length) * 100)}%\n`);

    // Show some not-installed games
    if (notInstalled.length > 0) {
      console.log('💾 Some games you own but haven\'t installed:');
      notInstalled.slice(0, 10).forEach((game, i) => {
        const hours = Math.floor(game.playtime_forever / 60);
        console.log(`  ${i + 1}. ${game.name}${hours > 0 ? ` (${hours} hours played)` : ''}`);
      });
      console.log();
    }

    // Test 4: Get app details (no auth required)
    console.log('🔍 Testing Store API (getting TF2 details)...');
    const tf2Details = await client.getAppDetails('440');
    console.log('✅ Store API works! (no auth required)\n');

    // Test 5: Check if Steam is running
    console.log('🎯 Checking if Steam client is running...');
    const isRunning = await client.isSteamRunning();
    console.log(`${isRunning ? '✅' : '⚠️ '} Steam is ${isRunning ? 'running' : 'not running'}\n`);

    // Show image URLs for first installed game
    if (installed.length > 0) {
      const firstGame = installed[0];
      console.log(`🖼️  Image URLs for "${firstGame.name}":`);
      console.log(`  Header: ${client.getGameHeaderUrl(firstGame.appid.toString())}`);
      console.log(`  Hero: ${client.getGameHeroUrl(firstGame.appid.toString())}`);
      console.log(`  Capsule: ${client.getGameCapsuleUrl(firstGame.appid.toString())}`);
      console.log();
    }

    console.log('✅ All tests passed!\n');

    // Summary
    console.log('📝 Summary:');
    console.log('  ✅ Steam Web API - Working');
    console.log('  ✅ Local VDF Scanner - Working');
    console.log('  ✅ Combined Detection - Working');
    console.log('  ✅ Store API - Working');
    console.log('  ✅ Client Detection - Working');
    console.log();
    console.log('🎉 Steam integration is fully functional!');

  } catch (error) {
    console.error('\n❌ Error:', error);

    if (error instanceof Error) {
      if (error.message.includes('403') || error.message.includes('401')) {
        console.error('\n💡 Hint: Your API key or Steam ID might be invalid.');
        console.error('   Check: https://steamcommunity.com/dev/apikey');
      } else if (error.message.includes('ENOTFOUND')) {
        console.error('\n💡 Hint: Check your internet connection.');
      }
    }

    process.exit(1);
  }
}

testSteamWebAPI();
