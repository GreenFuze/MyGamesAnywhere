/**
 * Steam Client Operations Example
 *
 * This example demonstrates how to use SteamClient to:
 * - Install/uninstall games via Steam
 * - Launch games
 * - Get owned games from Steam Web API
 * - Access game images and metadata
 */

import { SteamClient, scanSteamLibrary } from '@mygamesanywhere/steam-scanner';

async function demonstrateClientOperations() {
  console.log('=== Steam Client Operations Demo ===\n');

  // Create client (no API key needed for protocol operations)
  const client = new SteamClient();

  // Check if Steam is running
  console.log('Checking if Steam is running...');
  const isRunning = await client.isSteamRunning();
  console.log(`Steam is ${isRunning ? 'running' : 'not running'}\n`);

  // Get installed games from scanner
  console.log('Scanning installed games...');
  const scanResult = await scanSteamLibrary();
  console.log(`Found ${scanResult.games.length} installed games\n`);

  if (scanResult.games.length > 0) {
    const game = scanResult.games[0];
    console.log(`Example game: ${game.name} (${game.appid})\n`);

    // Get game images
    console.log('Game image URLs:');
    console.log(`  Header: ${client.getGameHeaderUrl(game.appid)}`);
    console.log(`  Hero: ${client.getGameHeroUrl(game.appid)}`);
    console.log(`  Capsule: ${client.getGameCapsuleUrl(game.appid)}`);
    console.log();

    // Example operations (commented out to prevent accidental execution)
    console.log('Available operations (not executed):');
    console.log(`  Launch game:      client.launchGame('${game.appid}')`);
    console.log(`  Open store page:  client.openStorePage('${game.appid}')`);
    console.log(`  Validate files:   client.validateGameFiles('${game.appid}')`);
    console.log(`  Uninstall:        client.uninstallGame('${game.appid}')`);
    console.log();
  }

  // Example: Get app details from Steam Store API (no auth required)
  console.log('Getting app details for Team Fortress 2 (440)...');
  try {
    const appDetails = await client.getAppDetails('440');
    console.log('✅ Successfully retrieved app details');
    console.log('Details:', JSON.stringify(appDetails, null, 2).substring(0, 500) + '...\n');
  } catch (error) {
    console.error('Failed to get app details:', error);
  }

  // Steam library shortcuts
  console.log('Steam client shortcuts:');
  console.log('  client.openLibrary()    - Opens Steam library');
  console.log('  client.openDownloads()  - Opens downloads page');
  console.log('  client.openSettings()   - Opens Steam settings');
  console.log();
}

async function demonstrateWebAPI() {
  console.log('\n=== Steam Web API Demo ===\n');

  // To use Steam Web API, you need:
  // 1. Steam Web API Key from: https://steamcommunity.com/dev/apikey
  // 2. Your Steam ID (64-bit)

  const apiKey = process.env.STEAM_API_KEY;
  const steamId = process.env.STEAM_ID;

  if (!apiKey || !steamId) {
    console.log('⚠️  Steam Web API not configured');
    console.log('To use Web API features, set environment variables:');
    console.log('  STEAM_API_KEY=your-api-key');
    console.log('  STEAM_ID=your-steam-id-64');
    console.log('\nGet API key from: https://steamcommunity.com/dev/apikey');
    console.log('Find Steam ID from: https://steamid.io/\n');
    return;
  }

  const client = new SteamClient({ apiKey, steamId });

  try {
    // Get owned games
    console.log('Fetching owned games from Steam Web API...');
    const ownedGames = await client.getOwnedGames();

    console.log(`✅ You own ${ownedGames.length} games\n`);

    // Show first 10 games
    console.log('First 10 owned games:');
    ownedGames.slice(0, 10).forEach((game, i) => {
      const hours = Math.floor(game.playtime_forever / 60);
      console.log(`  ${i + 1}. ${game.name}`);
      console.log(`     App ID: ${game.appid}`);
      console.log(`     Playtime: ${hours} hours`);
      if (game.img_icon_url) {
        console.log(
          `     Icon: ${client.getGameIconUrl(game.appid.toString(), game.img_icon_url)}`
        );
      }
      console.log();
    });

    // Get recently played games
    console.log('Fetching recently played games...');
    const recentGames = await client.getRecentlyPlayedGames(5);

    if (recentGames.length > 0) {
      console.log(`\n✅ Recently played (${recentGames.length}):\n`);
      recentGames.forEach((game, i) => {
        const hours = Math.floor(game.playtime_forever / 60);
        console.log(`  ${i + 1}. ${game.name} - ${hours} hours total`);
      });
    } else {
      console.log('\nNo recently played games');
    }
  } catch (error) {
    console.error('❌ Steam Web API error:', error);
  }
}

async function demonstrateInstallUninstall() {
  console.log('\n=== Install/Uninstall Demo ===\n');

  const client = new SteamClient();

  console.log('This example shows how to trigger install/uninstall via MyGamesAnywhere');
  console.log('(Commands are commented out to prevent accidental execution)\n');

  // Example: Install a free game (Team Fortress 2)
  const tf2AppId = '440';

  console.log('Example: Installing Team Fortress 2 (free game)');
  console.log(`Code: await client.installGame('${tf2AppId}');`);
  console.log('This would:');
  console.log('  1. Open Steam client');
  console.log('  2. Show install dialog for TF2');
  console.log('  3. User confirms installation');
  console.log('  4. Steam downloads and installs\n');

  // Uncomment to actually execute:
  // await client.installGame(tf2AppId);

  console.log('Example: Uninstalling a game');
  console.log(`Code: await client.uninstallGame('${tf2AppId}');`);
  console.log('This would:');
  console.log('  1. Open Steam client');
  console.log('  2. Show uninstall dialog');
  console.log('  3. User confirms uninstallation');
  console.log('  4. Steam uninstalls the game\n');

  // Uncomment to actually execute:
  // await client.uninstallGame(tf2AppId);

  console.log('Example: Launching a game');
  console.log(`Code: await client.launchGame('${tf2AppId}');`);
  console.log('This would launch the game via Steam\n');

  // Uncomment to actually execute:
  // await client.launchGame(tf2AppId);
}

// Main execution
async function main() {
  try {
    await demonstrateClientOperations();
    await demonstrateWebAPI();
    await demonstrateInstallUninstall();

    console.log('\n✅ Demo complete!');
    console.log('\nKey Takeaways:');
    console.log('  • SteamClient handles install/uninstall via Steam protocol URLs');
    console.log('  • Steam Web API provides owned games and playtime data');
    console.log('  • All operations integrate with Steam client (not replacing it)');
    console.log('  • User actions initiated from MyGamesAnywhere, executed by Steam\n');
  } catch (error) {
    console.error('❌ Error:', error);
    process.exit(1);
  }
}

main();
