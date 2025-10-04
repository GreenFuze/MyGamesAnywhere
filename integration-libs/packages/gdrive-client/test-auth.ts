/**
 * Google Drive Authentication Test
 *
 * Simple authentication test using the new GDriveAuth helper.
 * No manual OAuth setup required - just run and authenticate!
 */

import { authenticateGDrive, DriveClient } from './src/index.js';

async function testAuth() {
  console.log('=== Google Drive Authentication Test ===\n');

  try {
    // Authenticate (or check if already authenticated)
    const auth = await authenticateGDrive();

    // Create Drive client
    const client = new DriveClient({
      oauth: {
        clientId: '', // Not needed, auth already has it
        clientSecret: '',
        redirectUri: '',
      },
      tokenStorage: auth.getTokenStorage(),
    });

    // Override the oauth client with our authenticated one
    (client as any).oauth = auth.getOAuthClient();

    console.log('\n📂 Testing Google Drive access...\n');

    // List files
    console.log('📋 Listing files...');
    const result = await client.listFiles({ pageSize: 10 });

    console.log(`✅ Found ${result.files.length} files\n`);

    if (result.files.length > 0) {
      console.log('Recent files:');
      for (const file of result.files) {
        const size = file.size
          ? ` (${Math.round(parseInt(file.size) / 1024)} KB)`
          : '';
        const type =
          file.mimeType === 'application/vnd.google-apps.folder'
            ? ' (folder)'
            : '';
        console.log(`  - ${file.name}${type}${size}`);
      }
      console.log();
    }

    console.log('✅ Authentication status:', (await auth.isAuthenticated()) ? 'Authenticated' : 'Not authenticated');
    console.log('\n✨ Success! Google Drive is connected.\n');
    console.log('Next steps:');
    console.log('- Scan Google Drive for games: cd ../generic-repository && npm run test:scan-gdrive\n');
  } catch (error) {
    console.error('\n❌ Error:', error instanceof Error ? error.message : error);
    process.exit(1);
  }
}

testAuth();
