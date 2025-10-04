/**
 * Custom Storefront Source Plugin
 * Detects games from custom storage locations (Google Drive, local folders, etc.)
 */

import { DriveClient, type DriveClientConfig, type OAuth2Config, type StoredTokens } from '@mygamesanywhere/platform-google';
import { GDriveRepository, RepositoryScanner } from '@mygamesanywhere/generic-repository';
import type { DetectedGame as RepoDetectedGame } from '@mygamesanywhere/generic-repository';
import {
  PluginType,
  type SourcePlugin,
  type DetectedGame,
  type PluginMetadata,
  type PluginConfigSchema,
} from '@mygamesanywhere/plugin-system';

/**
 * Custom storefront plugin configuration
 */
export interface CustomStorefrontConfig {
  /**
   * Google Drive folder ID to scan
   */
  gdriveFolderId?: string;

  /**
   * Google Drive OAuth tokens
   */
  gdriveTokens?: StoredTokens;

  /**
   * Google Drive OAuth credentials
   */
  gdriveCredentials?: {
    clientId: string;
    clientSecret: string;
    redirectUri: string;
  };
}

/**
 * Custom storefront source plugin
 */
export class CustomStorefrontSourcePlugin implements SourcePlugin {
  readonly type = PluginType.SOURCE;
  readonly metadata: PluginMetadata = {
    id: 'custom-storefront-source',
    name: 'Custom Storefront',
    version: '0.1.0',
    description: 'Scan games from custom storage locations (Google Drive, etc.)',
    author: 'MyGamesAnywhere',
  };

  readonly configSchema: PluginConfigSchema = {
    properties: {
      gdriveFolderId: { type: 'string', description: 'Google Drive folder ID' },
      gdriveTokens: { type: 'object', description: 'Google Drive OAuth tokens' },
      gdriveCredentials: { type: 'object', description: 'Google Drive OAuth credentials' },
    },
  };

  private scanner?: RepositoryScanner;
  private driveClient?: DriveClient;
  private repository?: GDriveRepository;
  private initialized = false;

  /**
   * Initialize the plugin
   */
  async initialize(config?: CustomStorefrontConfig): Promise<void> {
    if (!config?.gdriveFolderId || !config?.gdriveTokens || !config?.gdriveCredentials) {
      throw new Error(
        'Custom Storefront plugin requires gdriveFolderId, gdriveTokens, and gdriveCredentials'
      );
    }

    // Build OAuth2Config
    const oauth: OAuth2Config = {
      clientId: config.gdriveCredentials.clientId,
      clientSecret: config.gdriveCredentials.clientSecret,
      redirectUri: config.gdriveCredentials.redirectUri,
    };

    // Build DriveClientConfig
    const driveConfig: DriveClientConfig = {
      oauth,
      tokenStorage: {
        saveTokens: async (_tokens) => {
          // No-op for now, tokens managed externally
        },
        loadTokens: async () => config.gdriveTokens || null,
        clearTokens: async () => {
          // No-op
        },
      },
    };

    // Initialize Google Drive client
    this.driveClient = new DriveClient(driveConfig);

    // Set tokens directly
    (this.driveClient as any).oauth.setTokens(config.gdriveTokens);

    // Initialize repository
    this.repository = new GDriveRepository(this.driveClient, config.gdriveFolderId);

    // Initialize scanner
    this.scanner = new RepositoryScanner(this.repository);

    this.initialized = true;
    console.log(`✅ Initialized Custom Storefront source plugin`);
  }

  /**
   * Check if plugin is ready
   */
  async isReady(): Promise<boolean> {
    return this.initialized && !!this.scanner;
  }

  /**
   * Scan for games
   */
  async scan(): Promise<DetectedGame[]> {
    if (!this.scanner) {
      throw new Error('Custom Storefront plugin not initialized');
    }

    console.log('📁 Scanning custom storefront...');
    const scanResult = await this.scanner.scan();

    // Convert generic-repository DetectedGame to plugin-system DetectedGame
    const detectedGames: DetectedGame[] = scanResult.games.map((game: RepoDetectedGame) =>
      this.convertToPluginDetectedGame(game)
    );

    console.log(`  Found ${detectedGames.length} games in custom storefront`);
    return detectedGames;
  }

  /**
   * Convert generic-repository DetectedGame to plugin-system DetectedGame
   */
  private convertToPluginDetectedGame(repoGame: RepoDetectedGame): DetectedGame {
    return {
      sourceId: this.metadata.id,
      id: `custom-${repoGame.id}`,
      name: repoGame.name,
      path: repoGame.location.path,
      size: repoGame.location.size,
      installed: repoGame.installation?.state === 'installed',
      platform: this.detectPlatformFromType(repoGame.type),
      metadata: {
        gameType: repoGame.type,
        repositoryType: repoGame.location.repositoryType,
        isArchived: repoGame.location.isArchived,
        archiveType: repoGame.location.archiveType,
        archiveParts: repoGame.location.archiveParts,
        confidence: repoGame.confidence,
        detectedAt: repoGame.detectedAt,
        // Include original metadata if available
        ...repoGame.metadata,
      },
    };
  }

  /**
   * Detect platform from game type
   */
  private detectPlatformFromType(gameType: string): string | undefined {
    switch (gameType) {
      case 'rom':
        return 'Emulated';
      case 'requires_dosbox':
        return 'DOS';
      case 'requires_scummvm':
        return 'ScummVM';
      case 'installer_executable':
      case 'installer_platform':
      case 'portable_game':
        return 'Windows';
      default:
        return undefined;
    }
  }

  /**
   * Get a specific game
   */
  async getGame(gameId: string): Promise<DetectedGame | null> {
    const games = await this.scan();
    return games.find((g) => g.id === gameId) || null;
  }

  /**
   * Cleanup resources
   */
  async cleanup(): Promise<void> {
    this.scanner = undefined;
    this.driveClient = undefined;
    this.repository = undefined;
    this.initialized = false;
  }
}
