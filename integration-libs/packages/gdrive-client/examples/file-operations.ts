/**
 * File Operations Example
 *
 * This example demonstrates various file operations with Google Drive.
 * Run basic-oauth-flow.ts first to authenticate.
 */

import { DriveClient, FileTokenStorage } from '@mygamesanywhere/gdrive-client';
import { readFile, writeFile } from 'fs/promises';

const client = new DriveClient({
  oauth: {
    clientId: process.env.GOOGLE_CLIENT_ID || 'YOUR_CLIENT_ID',
    clientSecret: process.env.GOOGLE_CLIENT_SECRET || 'YOUR_CLIENT_SECRET',
    redirectUri: 'http://localhost:3000/oauth/callback',
  },
  tokenStorage: new FileTokenStorage('./.gdrive-tokens.json'),
});

async function listFiles() {
  console.log('=== LIST FILES ===\n');

  const result = await client.listFiles({
    pageSize: 20,
    orderBy: 'modifiedTime desc',
  });

  console.log(`Total files: ${result.files.length}\n`);

  result.files.forEach((file, i) => {
    console.log(`${i + 1}. ${file.name}`);
    console.log(`   ID: ${file.id}`);
    console.log(`   Type: ${file.mimeType}`);
    if (file.size) {
      const sizeMB = (parseInt(file.size) / (1024 * 1024)).toFixed(2);
      console.log(`   Size: ${sizeMB} MB`);
    }
    console.log();
  });

  return result.files;
}

async function uploadFile() {
  console.log('\n=== UPLOAD FILE ===\n');

  // Create sample content
  const content = Buffer.from(`Hello from Google Drive API!
Created at: ${new Date().toISOString()}
This is a test file uploaded by @mygamesanywhere/gdrive-client`);

  console.log('Uploading test file...');

  const file = await client.uploadFile(content, {
    name: `test-${Date.now()}.txt`,
    mimeType: 'text/plain',
    description: 'Test file uploaded by gdrive-client',
  });

  console.log('✅ File uploaded successfully!');
  console.log(`   Name: ${file.name}`);
  console.log(`   ID: ${file.id}`);
  console.log(`   Type: ${file.mimeType}`);

  return file;
}

async function downloadFile(fileId: string) {
  console.log('\n=== DOWNLOAD FILE ===\n');

  console.log(`Downloading file ${fileId}...`);

  const content = await client.downloadFile(fileId);

  console.log('✅ File downloaded successfully!');
  console.log(`   Size: ${content.length} bytes`);
  console.log(`   Content preview:\n`);
  console.log(content.toString('utf-8').substring(0, 200));

  // Save to local file
  const localPath = `./downloaded-${Date.now()}.txt`;
  await writeFile(localPath, content);
  console.log(`\n   Saved to: ${localPath}`);

  return content;
}

async function updateFileMetadata(fileId: string) {
  console.log('\n=== UPDATE FILE ===\n');

  console.log(`Updating file ${fileId}...`);

  const updated = await client.updateFile(fileId, {
    name: `updated-${Date.now()}.txt`,
    description: 'This file was updated via the API',
  });

  console.log('✅ File updated successfully!');
  console.log(`   New name: ${updated.name}`);
  console.log(`   Modified: ${updated.modifiedTime}`);

  return updated;
}

async function createFolder() {
  console.log('\n=== CREATE FOLDER ===\n');

  const folderName = `Test Folder ${Date.now()}`;
  console.log(`Creating folder: ${folderName}...`);

  const folder = await client.createFolder(folderName);

  console.log('✅ Folder created successfully!');
  console.log(`   Name: ${folder.name}`);
  console.log(`   ID: ${folder.id}`);
  console.log(`   Type: ${folder.mimeType}`);

  return folder;
}

async function uploadToFolder(folderId: string) {
  console.log('\n=== UPLOAD TO FOLDER ===\n');

  const content = Buffer.from('This file is inside a folder');

  console.log(`Uploading file to folder ${folderId}...`);

  const file = await client.uploadFile(content, {
    name: `file-in-folder-${Date.now()}.txt`,
    mimeType: 'text/plain',
    parents: [folderId],
  });

  console.log('✅ File uploaded to folder!');
  console.log(`   Name: ${file.name}`);
  console.log(`   ID: ${file.id}`);
  console.log(`   Parent: ${file.parents?.[0]}`);

  return file;
}

async function listFilesInFolder(folderId: string) {
  console.log('\n=== LIST FILES IN FOLDER ===\n');

  const result = await client.listFilesInFolder(folderId);

  console.log(`Files in folder: ${result.files.length}\n`);

  result.files.forEach((file, i) => {
    console.log(`${i + 1}. ${file.name} (${file.id})`);
  });

  return result;
}

async function searchFiles() {
  console.log('\n=== SEARCH FILES ===\n');

  // Search for text files
  console.log('Searching for text files...');

  const result = await client.listFiles({
    query: "mimeType='text/plain'",
    pageSize: 10,
  });

  console.log(`Found ${result.files.length} text files:\n`);

  result.files.forEach((file, i) => {
    console.log(`${i + 1}. ${file.name}`);
  });

  return result;
}

async function deleteFile(fileId: string) {
  console.log('\n=== DELETE FILE ===\n');

  console.log(`Deleting file ${fileId}...`);

  await client.deleteFile(fileId);

  console.log('✅ File deleted successfully!');
}

// Main execution
async function main() {
  try {
    console.log('Google Drive File Operations Example\n');
    console.log('=====================================\n');

    // Check authentication
    const isAuth = await client.getOAuthClient().isAuthenticated();
    if (!isAuth) {
      console.error('Not authenticated! Run basic-oauth-flow.ts first.');
      process.exit(1);
    }

    console.log('✅ Authenticated\n');

    // List existing files
    const existingFiles = await listFiles();

    // Create a folder
    const folder = await createFolder();

    // Upload file
    const uploadedFile = await uploadFile();

    // Download the file we just uploaded
    await downloadFile(uploadedFile.id);

    // Update file metadata
    await updateFileMetadata(uploadedFile.id);

    // Upload file to folder
    const fileInFolder = await uploadToFolder(folder.id);

    // List files in folder
    await listFilesInFolder(folder.id);

    // Search files
    await searchFiles();

    // Clean up - delete uploaded files
    console.log('\n=== CLEANUP ===\n');
    await deleteFile(uploadedFile.id);
    await deleteFile(fileInFolder.id);
    await deleteFile(folder.id);

    console.log('\n✅ All operations completed successfully!');
  } catch (error) {
    console.error('\n❌ Error:', error);
    process.exit(1);
  }
}

main();
