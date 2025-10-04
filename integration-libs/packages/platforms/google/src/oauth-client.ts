/**
 * OAuth 2.0 Client for Google Drive
 */

import type {
  OAuth2Config,
  OAuth2Tokens,
  StoredTokens,
  TokenStorage,
} from './types.js';
import {
  OAuth2Error,
  TokenExpiredError,
  AuthenticationError,
  NetworkError,
} from './errors.js';

/**
 * OAuth 2.0 Client
 * Handles authentication flow with Google OAuth 2.0
 */
export class OAuth2Client {
  private static readonly AUTH_URL = 'https://accounts.google.com/o/oauth2/v2/auth';
  private static readonly TOKEN_URL = 'https://oauth2.googleapis.com/token';
  private static readonly DEFAULT_SCOPES = [
    'https://www.googleapis.com/auth/drive.readonly',  // Read access to all files
  ];

  private config: OAuth2Config;
  private tokenStorage?: TokenStorage;
  private cachedTokens?: StoredTokens;

  constructor(config: OAuth2Config, tokenStorage?: TokenStorage) {
    this.config = {
      ...config,
      scopes: config.scopes || OAuth2Client.DEFAULT_SCOPES,
    };
    this.tokenStorage = tokenStorage;
  }

  /**
   * Generate authorization URL for user to visit
   */
  getAuthorizationUrl(state?: string): string {
    const params = new URLSearchParams({
      client_id: this.config.clientId,
      redirect_uri: this.config.redirectUri,
      response_type: 'code',
      scope: this.config.scopes!.join(' '),
      access_type: 'offline',
      prompt: 'consent',
    });

    if (state) {
      params.set('state', state);
    }

    return `${OAuth2Client.AUTH_URL}?${params.toString()}`;
  }

  /**
   * Exchange authorization code for access token
   */
  async getTokenFromCode(code: string): Promise<StoredTokens> {
    const body = new URLSearchParams({
      code,
      client_id: this.config.clientId,
      client_secret: this.config.clientSecret,
      redirect_uri: this.config.redirectUri,
      grant_type: 'authorization_code',
    });

    try {
      const response = await fetch(OAuth2Client.TOKEN_URL, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
        },
        body: body.toString(),
      });

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}));
        throw new OAuth2Error(
          `Failed to exchange code for token: ${response.statusText}`,
          response.status,
          errorData
        );
      }

      const tokens = (await response.json()) as OAuth2Tokens;
      const storedTokens = this.toStoredTokens(tokens);

      // Save tokens if storage is available
      if (this.tokenStorage) {
        await this.tokenStorage.saveTokens(storedTokens);
      }

      this.cachedTokens = storedTokens;
      return storedTokens;
    } catch (error) {
      if (error instanceof OAuth2Error) {
        throw error;
      }
      throw new NetworkError(
        `Network error during token exchange: ${
          error instanceof Error ? error.message : 'Unknown error'
        }`,
        error instanceof Error ? error : undefined
      );
    }
  }

  /**
   * Refresh access token using refresh token
   */
  async refreshAccessToken(refreshToken: string): Promise<StoredTokens> {
    const body = new URLSearchParams({
      refresh_token: refreshToken,
      client_id: this.config.clientId,
      client_secret: this.config.clientSecret,
      grant_type: 'refresh_token',
    });

    try {
      const response = await fetch(OAuth2Client.TOKEN_URL, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/x-www-form-urlencoded',
        },
        body: body.toString(),
      });

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}));
        throw new OAuth2Error(
          `Failed to refresh token: ${response.statusText}`,
          response.status,
          errorData
        );
      }

      const tokens = (await response.json()) as OAuth2Tokens;
      // Preserve the refresh token if not returned
      if (!tokens.refresh_token) {
        tokens.refresh_token = refreshToken;
      }

      const storedTokens = this.toStoredTokens(tokens);

      // Save tokens if storage is available
      if (this.tokenStorage) {
        await this.tokenStorage.saveTokens(storedTokens);
      }

      this.cachedTokens = storedTokens;
      return storedTokens;
    } catch (error) {
      if (error instanceof OAuth2Error) {
        throw error;
      }
      throw new NetworkError(
        `Network error during token refresh: ${
          error instanceof Error ? error.message : 'Unknown error'
        }`,
        error instanceof Error ? error : undefined
      );
    }
  }

  /**
   * Get valid access token, refreshing if necessary
   */
  async getAccessToken(): Promise<string> {
    // Try to load from cache first
    if (!this.cachedTokens && this.tokenStorage) {
      const loadedTokens = await this.tokenStorage.loadTokens();
      if (loadedTokens) {
        this.cachedTokens = loadedTokens;
      }
    }

    if (!this.cachedTokens) {
      throw new AuthenticationError(
        'No stored tokens found. Please authenticate first.'
      );
    }

    // Check if token is expired
    const now = Date.now();
    const expiresAt = this.cachedTokens.expiresAt;

    // Refresh if expired or expiring soon (5 minute buffer)
    const buffer = 5 * 60 * 1000; // 5 minutes
    if (now >= expiresAt - buffer) {
      if (!this.cachedTokens.refresh_token) {
        throw new TokenExpiredError(
          'Access token expired and no refresh token available'
        );
      }

      // Refresh the token
      await this.refreshAccessToken(this.cachedTokens.refresh_token);

      if (!this.cachedTokens) {
        throw new AuthenticationError('Failed to refresh access token');
      }
    }

    return this.cachedTokens.access_token;
  }

  /**
   * Revoke access token
   */
  async revokeToken(token: string): Promise<void> {
    try {
      const response = await fetch(
        `https://oauth2.googleapis.com/revoke?token=${token}`,
        {
          method: 'POST',
          headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
          },
        }
      );

      if (!response.ok) {
        throw new OAuth2Error(
          `Failed to revoke token: ${response.statusText}`,
          response.status
        );
      }

      // Clear stored tokens
      if (this.tokenStorage) {
        await this.tokenStorage.clearTokens();
      }
      this.cachedTokens = undefined;
    } catch (error) {
      if (error instanceof OAuth2Error) {
        throw error;
      }
      throw new NetworkError(
        `Network error during token revocation: ${
          error instanceof Error ? error.message : 'Unknown error'
        }`,
        error instanceof Error ? error : undefined
      );
    }
  }

  /**
   * Check if user is authenticated
   */
  async isAuthenticated(): Promise<boolean> {
    try {
      await this.getAccessToken();
      return true;
    } catch {
      return false;
    }
  }

  /**
   * Convert OAuth2Tokens to StoredTokens with expiration timestamp
   */
  private toStoredTokens(tokens: OAuth2Tokens): StoredTokens {
    const now = Date.now();
    const expiresIn = tokens.expires_in * 1000; // Convert to milliseconds
    const expiresAt = now + expiresIn;

    return {
      ...tokens,
      expiresAt,
    };
  }

  /**
   * Clear cached tokens
   */
  async logout(): Promise<void> {
    try {
      if (this.cachedTokens) {
        await this.revokeToken(this.cachedTokens.access_token);
      }
    } finally {
      this.cachedTokens = undefined;
      if (this.tokenStorage) {
        await this.tokenStorage.clearTokens();
      }
    }
  }
}
