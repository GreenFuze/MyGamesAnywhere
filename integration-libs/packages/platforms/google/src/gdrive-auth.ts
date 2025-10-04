/**
 * Google Drive Authentication Helper
 * Provides simplified OAuth authentication for end users
 */

import { OAuth2Client } from './oauth-client.js';
import { FileTokenStorage } from './token-storage.js';
import { homedir } from 'os';
import { join } from 'path';
import { createServer } from 'http';
import { parse } from 'url';
import open from 'open';

/**
 * Default OAuth credentials for MyGamesAnywhere
 *
 * NOTE: These are public client credentials for a native/CLI application.
 * According to OAuth 2.0 best practices, native apps cannot keep client secrets confidential.
 * These credentials are safe to embed in the application.
 *
 * The client credentials identify the MyGamesAnywhere app to Google.
 * Each user gets their own token by logging in with their Google account.
 * This is the same approach used by GitHub CLI, Google Cloud SDK, VS Code, etc.
 */
const DEFAULT_OAUTH_CONFIG = {
  clientId: process.env.GOOGLE_CLIENT_ID || '628863254475-aqhloe6280l3h0cm1tofmf5ib28v6e84.apps.googleusercontent.com',
  clientSecret: process.env.GOOGLE_CLIENT_SECRET || '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',
  redirectUri: 'http://localhost:3000/oauth/callback',
  scopes: [
    'https://www.googleapis.com/auth/drive.readonly',  // Read access to all files
  ],
};

/**
 * Default token storage location in user's home directory
 */
const DEFAULT_TOKEN_PATH = join(
  homedir(),
  '.mygamesanywhere',
  '.gdrive-tokens.json'
);

/**
 * GDrive Authentication Helper
 * Simplifies OAuth authentication with sensible defaults
 */
export class GDriveAuth {
  private oauth: OAuth2Client;
  private tokenStorage: FileTokenStorage;

  /**
   * Create authentication helper with optional custom config
   */
  constructor(
    config?: Partial<typeof DEFAULT_OAUTH_CONFIG>,
    tokenPath?: string
  ) {
    const finalConfig = {
      ...DEFAULT_OAUTH_CONFIG,
      ...config,
    };

    this.tokenStorage = new FileTokenStorage(tokenPath || DEFAULT_TOKEN_PATH);
    this.oauth = new OAuth2Client(finalConfig, this.tokenStorage);
  }

  /**
   * Authenticate with Google Drive using browser-based OAuth flow
   *
   * This will:
   * 1. Start a local HTTP server for OAuth callback
   * 2. Open browser to Google login page
   * 3. Wait for user to authorize
   * 4. Save tokens to disk
   *
   * @returns Promise that resolves when authentication is complete
   */
  async authenticate(): Promise<void> {
    return new Promise((resolve, reject) => {
      // Create local server for OAuth callback
      const server = createServer(async (req, res) => {
        if (!req.url) {
          res.writeHead(400);
          res.end('Bad request');
          return;
        }

        const parsedUrl = parse(req.url, true);

        // Handle OAuth callback
        if (parsedUrl.pathname === '/oauth/callback') {
          const code = parsedUrl.query.code as string;
          const error = parsedUrl.query.error as string;

          if (error) {
            res.writeHead(200, { 'Content-Type': 'text/html' });
            res.end(`
              <!DOCTYPE html>
              <html>
                <head>
                  <title>Authentication Failed</title>
                  <style>
                    body { font-family: system-ui; max-width: 600px; margin: 50px auto; padding: 20px; }
                    .error { color: #d32f2f; background: #ffebee; padding: 20px; border-radius: 8px; }
                  </style>
                </head>
                <body>
                  <div class="error">
                    <h1>❌ Authentication Failed</h1>
                    <p>Error: ${error}</p>
                    <p>You can close this window and try again.</p>
                  </div>
                </body>
              </html>
            `);
            server.close();
            reject(new Error(`OAuth error: ${error}`));
            return;
          }

          if (!code) {
            res.writeHead(400, { 'Content-Type': 'text/html' });
            res.end('Missing authorization code');
            server.close();
            reject(new Error('Missing authorization code'));
            return;
          }

          try {
            // Exchange code for tokens
            await this.oauth.getTokenFromCode(code);

            // Success page
            res.writeHead(200, { 'Content-Type': 'text/html' });
            res.end(`
              <!DOCTYPE html>
              <html>
                <head>
                  <title>Authentication Successful</title>
                  <style>
                    body { font-family: system-ui; max-width: 600px; margin: 50px auto; padding: 20px; }
                    .success { color: #2e7d32; background: #e8f5e9; padding: 20px; border-radius: 8px; }
                  </style>
                </head>
                <body>
                  <div class="success">
                    <h1>✅ Authentication Successful!</h1>
                    <p>You have successfully connected your Google Drive account.</p>
                    <p>You can close this window and return to the application.</p>
                  </div>
                </body>
              </html>
            `);

            server.close();
            resolve();
          } catch (err) {
            res.writeHead(500, { 'Content-Type': 'text/html' });
            res.end(`
              <!DOCTYPE html>
              <html>
                <head>
                  <title>Authentication Error</title>
                  <style>
                    body { font-family: system-ui; max-width: 600px; margin: 50px auto; padding: 20px; }
                    .error { color: #d32f2f; background: #ffebee; padding: 20px; border-radius: 8px; }
                  </style>
                </head>
                <body>
                  <div class="error">
                    <h1>❌ Authentication Error</h1>
                    <p>Failed to complete authentication. Please try again.</p>
                    <p>${err instanceof Error ? err.message : 'Unknown error'}</p>
                  </div>
                </body>
              </html>
            `);
            server.close();
            reject(err);
          }
        } else {
          // Handle other requests
          res.writeHead(404);
          res.end('Not found');
        }
      });

      // Start server
      server.listen(3000, async () => {
        console.log('🌐 OAuth callback server started on http://localhost:3000');
        console.log('📋 Opening browser for authentication...');

        // Get authorization URL
        const authUrl = this.oauth.getAuthorizationUrl('myga-auth-state');

        // Open browser
        try {
          await open(authUrl);
          console.log('⏳ Waiting for authorization...\n');
        } catch (err) {
          console.error('❌ Failed to open browser automatically.');
          console.error('Please open this URL manually:\n');
          console.error(authUrl);
          console.error();
        }
      });

      // Handle server errors
      server.on('error', (err) => {
        reject(err);
      });
    });
  }

  /**
   * Check if user is already authenticated
   */
  async isAuthenticated(): Promise<boolean> {
    return await this.oauth.isAuthenticated();
  }

  /**
   * Logout and clear stored tokens
   */
  async logout(): Promise<void> {
    await this.oauth.logout();
  }

  /**
   * Get the OAuth client for use with DriveClient
   */
  getOAuthClient(): OAuth2Client {
    return this.oauth;
  }

  /**
   * Get token storage
   */
  getTokenStorage(): FileTokenStorage {
    return this.tokenStorage;
  }

  /**
   * Get access token (for advanced usage)
   */
  async getAccessToken(): Promise<string> {
    return await this.oauth.getAccessToken();
  }
}

/**
 * Quick authentication function for testing/scripts
 *
 * @example
 * ```typescript
 * import { authenticateGDrive } from '@mygamesanywhere/gdrive-client';
 *
 * // Simply authenticate
 * await authenticateGDrive();
 *
 * // Check if already authenticated
 * const auth = await authenticateGDrive();
 * if (await auth.isAuthenticated()) {
 *   console.log('Already authenticated!');
 * } else {
 *   await auth.authenticate();
 * }
 * ```
 */
export async function authenticateGDrive(): Promise<GDriveAuth> {
  const auth = new GDriveAuth();

  // Check if already authenticated
  if (await auth.isAuthenticated()) {
    console.log('✅ Already authenticated with Google Drive');
    return auth;
  }

  // Start authentication flow
  console.log('🔐 Starting Google Drive authentication...\n');
  await auth.authenticate();
  console.log('\n✅ Authentication complete!');

  return auth;
}
