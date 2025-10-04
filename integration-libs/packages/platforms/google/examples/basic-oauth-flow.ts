/**
 * Basic OAuth 2.0 Flow Example
 *
 * This example demonstrates the complete OAuth flow for Google Drive authentication.
 */

import { DriveClient, FileTokenStorage } from '@mygamesanywhere/gdrive-client';
import * as http from 'http';
import { URL } from 'url';

// Configuration - Replace with your own credentials
const config = {
  oauth: {
    clientId: process.env.GOOGLE_CLIENT_ID || 'YOUR_CLIENT_ID',
    clientSecret: process.env.GOOGLE_CLIENT_SECRET || 'YOUR_CLIENT_SECRET',
    redirectUri: 'http://localhost:3000/oauth/callback',
  },
  tokenStorage: new FileTokenStorage('./.gdrive-tokens.json'),
};

const client = new DriveClient(config);
const oauthClient = client.getOAuthClient();

async function startAuthFlow() {
  // Step 1: Generate authorization URL
  const authUrl = oauthClient.getAuthorizationUrl('my-state-value');

  console.log('\n=== Google Drive OAuth 2.0 Flow ===\n');
  console.log('Step 1: Visit this URL to authorize the application:\n');
  console.log(authUrl);
  console.log('\nWaiting for authorization...\n');

  // Step 2: Start local server to receive callback
  const server = http.createServer(async (req, res) => {
    const url = new URL(req.url!, `http://localhost:3000`);

    if (url.pathname === '/oauth/callback') {
      const code = url.searchParams.get('code');
      const state = url.searchParams.get('state');
      const error = url.searchParams.get('error');

      if (error) {
        res.writeHead(400, { 'Content-Type': 'text/html' });
        res.end(`<h1>Authorization Error</h1><p>${error}</p>`);
        console.error('Authorization error:', error);
        server.close();
        return;
      }

      if (!code) {
        res.writeHead(400, { 'Content-Type': 'text/html' });
        res.end('<h1>No authorization code received</h1>');
        console.error('No code in callback');
        server.close();
        return;
      }

      try {
        // Step 3: Exchange code for tokens
        console.log('Step 2: Received authorization code');
        console.log('Step 3: Exchanging code for access token...');

        const tokens = await oauthClient.getTokenFromCode(code);

        console.log('\n✅ Authentication successful!');
        console.log('Tokens saved to:', './.gdrive-tokens.json');
        console.log('\nToken details:');
        console.log(`  Access token: ${tokens.access_token.substring(0, 20)}...`);
        console.log(`  Expires at: ${new Date(tokens.expiresAt).toLocaleString()}`);
        console.log(`  Has refresh token: ${!!tokens.refresh_token}`);

        // Send success response to browser
        res.writeHead(200, { 'Content-Type': 'text/html' });
        res.end(`
          <html>
            <head><title>Authorization Successful</title></head>
            <body>
              <h1>✅ Authorization Successful!</h1>
              <p>You can close this window and return to the terminal.</p>
              <script>setTimeout(() => window.close(), 3000);</script>
            </body>
          </html>
        `);

        // Close server and test the client
        server.close(() => {
          testClient().catch(console.error);
        });
      } catch (error) {
        console.error('Error exchanging code:', error);

        res.writeHead(500, { 'Content-Type': 'text/html' });
        res.end('<h1>Error during authentication</h1>');

        server.close();
      }
    } else {
      res.writeHead(404);
      res.end('Not found');
    }
  });

  server.listen(3000, () => {
    console.log('Server listening on http://localhost:3000');
  });
}

async function testClient() {
  console.log('\n=== Testing Google Drive Client ===\n');

  try {
    // Check if authenticated
    const isAuth = await oauthClient.isAuthenticated();
    console.log(`Authenticated: ${isAuth}`);

    if (!isAuth) {
      console.log('Not authenticated. Please run the auth flow first.');
      return;
    }

    // List files
    console.log('\nFetching files from Google Drive...');
    const result = await client.listFiles({
      pageSize: 10,
      orderBy: 'modifiedTime desc',
    });

    console.log(`\nFound ${result.files.length} files:`);
    result.files.slice(0, 5).forEach((file, i) => {
      console.log(`  ${i + 1}. ${file.name}`);
      console.log(`     ID: ${file.id}`);
      console.log(`     Type: ${file.mimeType}`);
      if (file.modifiedTime) {
        console.log(`     Modified: ${new Date(file.modifiedTime).toLocaleString()}`);
      }
      console.log();
    });
  } catch (error) {
    console.error('Error testing client:', error);
  } finally {
    process.exit(0);
  }
}

// Start the auth flow
startAuthFlow().catch((error) => {
  console.error('Fatal error:', error);
  process.exit(1);
});
