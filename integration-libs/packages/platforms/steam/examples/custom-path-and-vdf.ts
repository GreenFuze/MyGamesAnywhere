/**
 * Custom Path and VDF Parser Example
 *
 * This example shows how to use custom Steam paths and the low-level VDF parser.
 */

import { SteamScanner, parseVDF } from '@mygamesanywhere/steam-scanner';
import { readFile } from 'fs/promises';
import { join } from 'path';

async function customPathExample() {
  console.log('=== CUSTOM STEAM PATH EXAMPLE ===\n');

  const scanner = new SteamScanner();

  // Method 1: Auto-detect Steam path
  console.log('Method 1: Auto-detection');
  try {
    const steamPath = await scanner.detectSteamPath();
    console.log(`✅ Steam detected at: ${steamPath}\n`);

    // Initialize with the detected path
    await scanner.initialize({ steamPath });

    const result = await scanner.scan();
    console.log(`Found ${result.games.length} games\n`);

  } catch (error) {
    console.error('❌ Auto-detection failed:', error.message);
  }

  // Method 2: Specify custom Steam path
  console.log('Method 2: Custom path');
  try {
    const customScanner = new SteamScanner();

    // Windows example
    const customPath = 'C:\\Program Files (x86)\\Steam';
    // macOS example:
    // const customPath = '/Users/username/Library/Application Support/Steam';
    // Linux example:
    // const customPath = '/home/username/.steam/steam';

    await customScanner.initialize({ steamPath: customPath });
    const result = await customScanner.scan();

    console.log(`✅ Scanned custom path: ${customPath}`);
    console.log(`Found ${result.games.length} games\n`);

  } catch (error) {
    console.error('❌ Custom path failed:', error.message);
  }
}

async function vdfParserExample() {
  console.log('=== VDF PARSER EXAMPLE ===\n');

  // Example 1: Parse libraryfolders.vdf
  console.log('Example 1: Parse libraryfolders.vdf');
  try {
    const scanner = new SteamScanner();
    const steamPath = await scanner.detectSteamPath();
    const libraryVdfPath = join(steamPath, 'steamapps', 'libraryfolders.vdf');

    const content = await readFile(libraryVdfPath, 'utf-8');
    const parsed = parseVDF(content);

    console.log('✅ Parsed libraryfolders.vdf:');
    console.log(JSON.stringify(parsed, null, 2));
    console.log();

  } catch (error) {
    console.error('❌ Failed to parse libraryfolders.vdf:', error.message);
  }

  // Example 2: Parse a game manifest file
  console.log('Example 2: Parse game manifest');
  try {
    const scanner = new SteamScanner();
    const steamPath = await scanner.detectSteamPath();

    // Find first appmanifest file
    const { readdir } = await import('fs/promises');
    const steamappsPath = join(steamPath, 'steamapps');
    const files = await readdir(steamappsPath);
    const manifestFile = files.find(f => f.startsWith('appmanifest_') && f.endsWith('.acf'));

    if (manifestFile) {
      const manifestPath = join(steamappsPath, manifestFile);
      const content = await readFile(manifestPath, 'utf-8');
      const parsed = parseVDF(content);

      console.log(`✅ Parsed ${manifestFile}:`);
      console.log(JSON.stringify(parsed, null, 2));
    } else {
      console.log('⚠ No game manifest files found');
    }

  } catch (error) {
    console.error('❌ Failed to parse manifest:', error.message);
  }

  // Example 3: Parse custom VDF content
  console.log('\nExample 3: Parse custom VDF string');
  const customVDF = `
"MyConfig"
{
  "setting1"  "value1"
  "setting2"  "value2"
  "nested"
  {
    "key"  "value"
    "number"  "12345"
  }
}
  `;

  try {
    const parsed = parseVDF(customVDF);
    console.log('✅ Parsed custom VDF:');
    console.log(JSON.stringify(parsed, null, 2));

  } catch (error) {
    console.error('❌ Failed to parse custom VDF:', error.message);
  }
}

// Run the examples
async function runExamples() {
  try {
    await customPathExample();
    console.log('\n' + '='.repeat(50) + '\n');
    await vdfParserExample();
  } catch (error) {
    console.error('Error running examples:', error);
  }
}

runExamples();
