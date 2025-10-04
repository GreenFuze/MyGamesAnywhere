/**
 * Debug Google Drive folder contents
 */

import { DriveClient, GDriveAuth } from '@mygamesanywhere/gdrive-client';

async function debugGDrive() {
  console.log('=== Debug Google Drive Folder ===\n');

  // Authenticate
  const auth = new GDriveAuth();

  if (!(await auth.isAuthenticated())) {
    console.error('❌ Not authenticated with Google Drive!');
    process.exit(1);
  }

  console.log('✅ Authenticated with Google Drive\n');

  // Create Drive client
  const client = new DriveClient({
    oauth: { clientId: '', clientSecret: '', redirectUri: '' },
    tokenStorage: auth.getTokenStorage(),
  });
  (client as any).oauth = auth.getOAuthClient();

  const folderId = process.argv[2] || 'root';
  console.log(`📂 Listing contents of folder: ${folderId}\n`);

  try {
    const result = await client.listFilesInFolder(folderId);

    console.log(`Found ${result.files.length} items:\n`);

    for (const file of result.files) {
      const isFolder = file.mimeType === 'application/vnd.google-apps.folder';
      const type = isFolder ? '📁 FOLDER' : '📄 FILE';
      const size = file.size ? `(${Math.round(parseInt(file.size) / 1024)} KB)` : '';

      console.log(`${type}: ${file.name} ${size}`);
      console.log(`  ID: ${file.id}`);
      console.log(`  MIME: ${file.mimeType}`);
      console.log();
    }
  } catch (error) {
    console.error('❌ Error:', error);
  }
}

debugGDrive();
