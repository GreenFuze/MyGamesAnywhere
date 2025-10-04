/**
 * Get Your Steam ID from Steam's Official API
 *
 * This uses Steam's own API to convert your username to Steam ID
 */

import { getConfigManager } from '@mygamesanywhere/config';

async function getSteamId() {
  console.log('=== Get Your Steam ID ===\n');

  // Load config to get API key
  const config = getConfigManager();
  await config.load();

  const steamConfig = config.getSteam();

  if (!steamConfig?.apiKey) {
    console.error('❌ No API key found in config');
    console.error('Please add your Steam API key to ~/.mygamesanywhere/config.json first\n');
    process.exit(1);
  }

  // Ask for username
  console.log('Enter your Steam username or custom URL:');
  console.log('(Example: greenfuze or the part after steamcommunity.com/id/)');
  console.log();

  // Get username from command line or use default
  const username = process.argv[2] || 'greenfuze';

  console.log(`Looking up Steam ID for: ${username}\n`);

  try {
    // Use Steam's official API to resolve username to Steam ID
    const url = `https://api.steampowered.com/ISteamUser/ResolveVanityURL/v1/`;
    const params = new URLSearchParams({
      key: steamConfig.apiKey,
      vanityurl: username,
    });

    const response = await fetch(`${url}?${params}`);
    const data = await response.json() as {
      response: {
        success: number;
        steamid?: string;
        message?: string;
      };
    };

    if (data.response.success === 1 && data.response.steamid) {
      const steamId = data.response.steamid;

      console.log('✅ Found your Steam ID!\n');
      console.log(`Steam ID (steamID64): ${steamId}\n`);
      console.log('Copy this to your config file:\n');
      console.log(`{`);
      console.log(`  "steam": {`);
      console.log(`    "apiKey": "${steamConfig.apiKey}",`);
      console.log(`    "steamId": "${steamId}"`);
      console.log(`  }`);
      console.log(`}\n`);

      // Try to update config automatically
      try {
        await config.updateSteam({ steamId });
        console.log('✅ Config updated automatically!');
        console.log(`Saved to: ${config.getConfigPath()}\n`);
      } catch (error) {
        console.log('⚠️  Could not update config automatically');
        console.log('Please copy the JSON above to your config file manually\n');
      }

    } else {
      console.error('❌ Could not find Steam ID for username:', username);
      console.error('\nTry one of these methods:\n');
      console.error('1. Open Steam app → Click profile → Account details');
      console.error('2. Visit your Steam profile at steamcommunity.com');
      console.error('   Look at the URL - if it shows /profiles/NUMBER, that\'s your Steam ID\n');
    }

  } catch (error) {
    console.error('❌ Error:', error instanceof Error ? error.message : error);
    console.error('\nAlternative methods to find your Steam ID:\n');
    console.error('1. Open Steam app → Click profile → Account details');
    console.error('2. Visit your Steam profile at steamcommunity.com');
    console.error('   Look at the URL - if it shows /profiles/NUMBER, that\'s your Steam ID\n');
  }
}

// Run
getSteamId();
