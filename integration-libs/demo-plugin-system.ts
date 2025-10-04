/**
 * Plugin System Demo
 * Demonstrates the complete plugin architecture with multi-source games
 */

import {
  pluginRegistry,
  UnifiedGameManager,
  MatchStrategy,
  type GameSource,
} from './packages/core/plugin-system/src/index.js';
import { SteamSourcePlugin } from './packages/plugins/steam-source/src/index.js';
import { LaunchBoxIdentifierPlugin } from './packages/plugins/launchbox-identifier/src/index.js';
import { getConfig } from './packages/core/config/src/index.js';

/**
 * Demo: Complete plugin system workflow
 */
async function main() {
  console.log('=== MyGamesAnywhere Plugin System Demo ===\n');

  // ========================================
  // Step 1: Initialize Plugins
  // ========================================
  console.log('Step 1: Initializing plugins...\n');

  // Steam source plugin
  const steamPlugin = new SteamSourcePlugin();
  await steamPlugin.initialize({
    scanLocal: true,
    scanWeb: false, // Set to true if you have Steam API key
    installedOnly: false,
  });
  pluginRegistry.register(steamPlugin);

  // LaunchBox identifier plugin
  const launchboxPlugin = new LaunchBoxIdentifierPlugin();
  await launchboxPlugin.initialize({
    autoDownload: true,
    minConfidence: 0.5,
  });
  pluginRegistry.register(launchboxPlugin);

  console.log(`✅ Registered ${pluginRegistry.size()} plugins\n`);

  // ========================================
  // Step 2: Scan for Games from All Sources
  // ========================================
  console.log('Step 2: Scanning for games from all sources...\n');

  const allSources = pluginRegistry.getAllSources();
  console.log(`Found ${allSources.length} source plugins`);

  const unifiedManager = new UnifiedGameManager();
  unifiedManager.setMatchStrategy(MatchStrategy.FUZZY_TITLE, 0.85);

  for (const sourcePlugin of allSources) {
    console.log(`\n📁 Scanning: ${sourcePlugin.metadata.name}`);

    try {
      const detectedGames = await sourcePlugin.scan();
      console.log(`  Found ${detectedGames.length} games`);

      // Add each detected game to unified manager
      for (const detectedGame of detectedGames.slice(0, 10)) { // Limit to first 10 for demo
        const gameSource: GameSource = {
          sourceId: sourcePlugin.metadata.id,
          gameId: detectedGame.id,
          detectedGame,
          installed: detectedGame.installed || false,
          lastPlayed: detectedGame.lastPlayed,
          metadata: detectedGame.metadata,
        };

        const unifiedGame = unifiedManager.addDetectedGame(gameSource);
        console.log(`    + Added: ${detectedGame.name}`);

        if (unifiedGame.sources.length > 1) {
          console.log(`      ⚡ Multi-source game! Also found in: ${unifiedGame.sources.map(s => s.sourceId).join(', ')}`);
        }
      }
    } catch (error) {
      console.error(`  ❌ Error scanning ${sourcePlugin.metadata.name}:`, error);
    }
  }

  // ========================================
  // Step 3: Identify Games
  // ========================================
  console.log('\n\nStep 3: Identifying games with metadata...\n');

  const allIdentifiers = pluginRegistry.getAllIdentifiers();
  console.log(`Found ${allIdentifiers.length} identifier plugins\n`);

  const unifiedGames = unifiedManager.getAllGames();
  console.log(`Processing ${unifiedGames.length} unified games...\n`);

  for (const unifiedGame of unifiedGames.slice(0, 5)) { // Limit to first 5 for demo
    console.log(`\n🎮 Identifying: ${unifiedGame.title}`);
    console.log(`   Sources: ${unifiedGame.sources.map(s => s.sourceId).join(', ')}`);

    for (const identifier of allIdentifiers) {
      try {
        // Use the first source's detected game for identification
        const primarySource = unifiedGame.sources[0];
        const identified = await identifier.identify(primarySource.detectedGame);

        if (identified.metadata) {
          unifiedManager.addIdentification(
            unifiedGame.id,
            identifier.metadata.id,
            identified
          );

          console.log(`   ✅ ${identifier.metadata.name}: "${identified.metadata.title}" (${Math.round(identified.matchConfidence * 100)}%)`);
          if (identified.metadata.developer) {
            console.log(`      Developer: ${identified.metadata.developer}`);
          }
          if (identified.metadata.releaseDate) {
            console.log(`      Released: ${identified.metadata.releaseDate}`);
          }
        } else {
          console.log(`   ❌ ${identifier.metadata.name}: No match found`);
        }
      } catch (error: any) {
        console.log(`   ⚠️  ${identifier.metadata.name}: ${error.message}`);
      }
    }
  }

  // ========================================
  // Step 4: Display Unified Game Library
  // ========================================
  console.log('\n\n=== Unified Game Library ===\n');

  const finalGames = unifiedManager.getAllGames();
  console.log(`Total unified games: ${finalGames.length}\n`);

  // Group by number of sources
  const multiSourceGames = finalGames.filter(g => g.sources.length > 1);
  const singleSourceGames = finalGames.filter(g => g.sources.length === 1);

  console.log(`📊 Multi-source games: ${multiSourceGames.length}`);
  console.log(`📊 Single-source games: ${singleSourceGames.length}\n`);

  if (multiSourceGames.length > 0) {
    console.log('=== Multi-Source Games (Same game in multiple libraries) ===\n');
    for (const game of multiSourceGames.slice(0, 3)) {
      console.log(`🎯 ${game.title}`);
      console.log(`   Platform: ${game.platform || 'Unknown'}`);
      console.log(`   Installed: ${game.isInstalled ? 'Yes' : 'No'}`);
      console.log(`   Sources:`);
      for (const source of game.sources) {
        console.log(`     - ${source.sourceId}: ${source.detectedGame.name}`);
      }
      if (game.identifications.length > 0) {
        console.log(`   Identifications:`);
        for (const id of game.identifications) {
          console.log(`     - ${id.identifierId}: ${id.metadata.title} (${Math.round(id.confidence * 100)}%)`);
        }
      }
      console.log();
    }
  }

  // ========================================
  // Step 5: Plugin Actions Demo
  // ========================================
  console.log('\n=== Plugin Actions Demo ===\n');

  const steamSource = pluginRegistry.getSource('steam-source');
  if (steamSource && finalGames.length > 0) {
    const steamGames = finalGames.filter(g =>
      g.sources.some(s => s.sourceId === 'steam-source')
    );

    if (steamGames.length > 0) {
      const exampleGame = steamGames[0];
      const steamSourceData = exampleGame.sources.find(s => s.sourceId === 'steam-source');

      if (steamSourceData) {
        console.log(`Example: Actions available for "${exampleGame.title}"`);
        console.log(`  - Launch: steamPlugin.launch("${steamSourceData.gameId}")`);
        console.log(`  - Install: steamPlugin.install("${steamSourceData.gameId}")`);
        console.log(`  - Uninstall: steamPlugin.uninstall("${steamSourceData.gameId}")`);
        console.log();
      }
    }
  }

  // ========================================
  // Step 6: Registry Statistics
  // ========================================
  console.log('\n=== Plugin Registry Statistics ===\n');
  console.log(`Total plugins: ${pluginRegistry.size()}`);
  console.log(`Source plugins: ${pluginRegistry.getAllSources().length}`);
  console.log(`Identifier plugins: ${pluginRegistry.getAllIdentifiers().length}`);
  console.log(`Storage plugins: ${pluginRegistry.getAllStorages().length}`);

  console.log('\nRegistered plugins:');
  for (const plugin of pluginRegistry.getAll()) {
    console.log(`  - ${plugin.metadata.name} (${plugin.metadata.id}) v${plugin.metadata.version}`);
    console.log(`    ${plugin.metadata.description}`);
  }

  // ========================================
  // Cleanup
  // ========================================
  console.log('\n\n=== Cleanup ===\n');
  await steamPlugin.cleanup();
  await launchboxPlugin.cleanup();
  console.log('✅ All plugins cleaned up');

  console.log('\n=== Demo Complete ===\n');
}

// Run demo
main().catch(console.error);
