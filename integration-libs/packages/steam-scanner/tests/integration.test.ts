/**
 * Integration tests for Steam Scanner
 *
 * These tests run against a real Steam installation on the system.
 * They are skipped if Steam is not found.
 */

import { describe, it, expect } from 'vitest';
import { SteamScanner, scanSteamLibrary } from '../src/scanner.js';
import { SteamNotFoundError } from '../src/errors.js';

describe('Integration Tests - Real Steam Installation', () => {
  // These tests only run if Steam is actually installed
  it('should detect Steam installation', async () => {
    const scanner = new SteamScanner();

    try {
      const steamPath = await scanner.detectSteamPath();
      expect(steamPath).toBeTruthy();
      expect(typeof steamPath).toBe('string');
      console.log(`✓ Steam detected at: ${steamPath}`);
    } catch (error) {
      if (error instanceof SteamNotFoundError) {
        console.log('⚠ Steam not found on this system - skipping integration tests');
        expect(true).toBe(true); // Pass the test but skip
      } else {
        throw error;
      }
    }
  });

  it('should scan Steam library', async () => {
    try {
      const result = await scanSteamLibrary();

      // Validate result structure
      expect(result).toBeDefined();
      expect(result.games).toBeInstanceOf(Array);
      expect(result.libraryFolders).toBeInstanceOf(Array);
      expect(result.steamPath).toBeTruthy();
      expect(result.scanDuration).toBeGreaterThan(0);

      // Log results
      console.log('\n=== Steam Scan Results ===');
      console.log(`Steam Path: ${result.steamPath}`);
      console.log(`Library Folders: ${result.libraryFolders.length}`);
      console.log(`Games Found: ${result.games.length}`);
      console.log(`Scan Duration: ${result.scanDuration}ms\n`);

      // Log library folders
      if (result.libraryFolders.length > 0) {
        console.log('Library Folders:');
        result.libraryFolders.forEach((folder, i) => {
          console.log(`  ${i + 1}. ${folder.path}`);
          console.log(`     Size: ${folder.totalsize} bytes`);
        });
        console.log();
      }

      // Log first few games
      if (result.games.length > 0) {
        console.log(`First ${Math.min(10, result.games.length)} Games:`);
        result.games.slice(0, 10).forEach((game, i) => {
          console.log(`  ${i + 1}. ${game.name} (App ID: ${game.appid})`);
          console.log(`     Install Dir: ${game.installdir}`);
          console.log(`     Size: ${game.sizeOnDisk} bytes`);
        });
        console.log();
      }

      // Validate game data
      if (result.games.length > 0) {
        const firstGame = result.games[0];
        expect(firstGame.appid).toBeTruthy();
        expect(firstGame.name).toBeTruthy();
        expect(firstGame.installdir).toBeTruthy();
        expect(firstGame.libraryPath).toBeTruthy();
      }
    } catch (error) {
      if (error instanceof SteamNotFoundError) {
        console.log('⚠ Steam not found on this system - skipping integration tests');
        expect(true).toBe(true);
      } else {
        throw error;
      }
    }
  });

  it('should handle multiple library folders if they exist', async () => {
    try {
      const result = await scanSteamLibrary();

      if (result.libraryFolders.length > 1) {
        console.log(`\n✓ Multiple library folders detected: ${result.libraryFolders.length}`);

        // Check that games are distributed across folders
        const gamesPerFolder = new Map<string, number>();
        result.games.forEach((game) => {
          const count = gamesPerFolder.get(game.libraryPath) || 0;
          gamesPerFolder.set(game.libraryPath, count + 1);
        });

        console.log('\nGames per library folder:');
        gamesPerFolder.forEach((count, path) => {
          console.log(`  ${path}: ${count} games`);
        });
        console.log();

        expect(gamesPerFolder.size).toBeGreaterThan(0);
      } else {
        console.log('\n⚠ Only one library folder found');
        expect(result.libraryFolders.length).toBeGreaterThanOrEqual(1);
      }
    } catch (error) {
      if (error instanceof SteamNotFoundError) {
        console.log('⚠ Steam not found - skipping test');
        expect(true).toBe(true);
      } else {
        throw error;
      }
    }
  });

  it('should validate game data integrity', async () => {
    try {
      const result = await scanSteamLibrary();

      if (result.games.length === 0) {
        console.log('⚠ No games found in Steam library');
        expect(true).toBe(true);
        return;
      }

      // Check that all games have required fields
      const missingFields: string[] = [];

      result.games.forEach((game, i) => {
        if (!game.appid) missingFields.push(`Game ${i}: missing appid`);
        if (!game.name) missingFields.push(`Game ${i}: missing name`);
        if (!game.installdir) missingFields.push(`Game ${i}: missing installdir`);
        if (!game.libraryPath) missingFields.push(`Game ${i}: missing libraryPath`);
      });

      if (missingFields.length > 0) {
        console.error('Data integrity issues found:');
        missingFields.forEach((issue) => console.error(`  - ${issue}`));
      }

      expect(missingFields).toHaveLength(0);

      console.log(`✓ All ${result.games.length} games have complete data`);
    } catch (error) {
      if (error instanceof SteamNotFoundError) {
        console.log('⚠ Steam not found - skipping test');
        expect(true).toBe(true);
      } else {
        throw error;
      }
    }
  });

  it('should complete scan within reasonable time', async () => {
    try {
      const startTime = Date.now();
      const result = await scanSteamLibrary();
      const duration = Date.now() - startTime;

      console.log(`\nScan Performance:`);
      console.log(`  Total Duration: ${duration}ms`);
      console.log(`  Games/second: ${((result.games.length / duration) * 1000).toFixed(2)}`);
      console.log(`  Avg time per game: ${(duration / Math.max(result.games.length, 1)).toFixed(2)}ms`);

      // Scan should complete within 10 seconds for typical libraries
      expect(duration).toBeLessThan(10000);
    } catch (error) {
      if (error instanceof SteamNotFoundError) {
        console.log('⚠ Steam not found - skipping test');
        expect(true).toBe(true);
      } else {
        throw error;
      }
    }
  });

  it('should handle custom Steam path configuration', async () => {
    const scanner = new SteamScanner();

    try {
      // First, detect the real path
      const realPath = await scanner.detectSteamPath();

      // Now initialize with that path explicitly
      await scanner.initialize({ steamPath: realPath });

      // Scan should work the same
      const result = await scanner.scan();

      expect(result.games).toBeInstanceOf(Array);
      expect(result.steamPath).toBe(realPath);

      console.log(`✓ Custom path configuration works: ${realPath}`);
    } catch (error) {
      if (error instanceof SteamNotFoundError) {
        console.log('⚠ Steam not found - skipping test');
        expect(true).toBe(true);
      } else {
        throw error;
      }
    }
  });
});
