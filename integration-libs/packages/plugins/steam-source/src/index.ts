/**
 * Steam Source Plugin
 * Detects and manages games from Steam library
 */

import { SteamScanner, SteamClient, type SteamWebAPIConfig } from '@mygamesanywhere/platform-steam';
import {
  PluginType,
  type SourcePlugin,
  type DetectedGame,
  type PluginMetadata,
  type PluginConfigSchema,
} from '@mygamesanywhere/plugin-system';

/**
 * Steam source plugin configuration
 */
export interface SteamSourceConfig {
  /**
   * Steam API key (optional, for Web API access)
   */
  apiKey?: string;

  /**
   * Steam username (for auto-resolving Steam ID)
   */
  username?: string;

  /**
   * Include only installed games
   */
  installedOnly?: boolean;

  /**
   * Scan local library folders
   */
  scanLocal?: boolean;

  /**
   * Scan Steam Web API (requires API key + username)
   */
  scanWeb?: boolean;
}

/**
 * Steam source plugin
 */
export class SteamSourcePlugin implements SourcePlugin {
  readonly type = PluginType.SOURCE;
  readonly metadata: PluginMetadata = {
    id: 'steam-source',
    name: 'Steam',
    version: '0.1.0',
    description: 'Scan and manage games from Steam library',
    author: 'MyGamesAnywhere',
  };

  readonly configSchema: PluginConfigSchema = {
    properties: {
      apiKey: { type: 'string', description: 'Steam API key' },
      username: { type: 'string', description: 'Steam username' },
      installedOnly: { type: 'boolean', default: false },
      scanLocal: { type: 'boolean', default: true },
      scanWeb: { type: 'boolean', default: false },
    },
  };

  private scanner?: SteamScanner;
  private client?: SteamClient;
  private config?: SteamSourceConfig;
  private initialized = false;

  /**
   * Initialize the plugin
   */
  async initialize(config?: SteamSourceConfig): Promise<void> {
    this.config = {
      installedOnly: false,
      scanLocal: true,
      scanWeb: false,
      ...config,
    };

    // Initialize scanner
    this.scanner = new SteamScanner();

    // Initialize client if API key provided
    if (this.config.apiKey && this.config.username) {
      const webAPIConfig: SteamWebAPIConfig = {
        apiKey: this.config.apiKey,
        username: this.config.username,
      };
      this.client = new SteamClient(webAPIConfig);
    }

    this.initialized = true;
    console.log(`✅ Initialized Steam source plugin`);
  }

  /**
   * Check if plugin is ready
   */
  async isReady(): Promise<boolean> {
    return this.initialized;
  }

  /**
   * Scan for Steam games
   */
  async scan(): Promise<DetectedGame[]> {
    if (!this.scanner) {
      throw new Error('Steam plugin not initialized');
    }

    const detectedGames: DetectedGame[] = [];

    // Scan local library
    if (this.config?.scanLocal) {
      console.log('📁 Scanning local Steam library...');
      const scanResult = await this.scanner.scan();

      for (const game of scanResult.games) {
        const installPath = `${game.libraryPath}\\steamapps\\common\\${game.installdir}`;
        detectedGames.push({
          sourceId: this.metadata.id,
          id: `steam-local-${game.appid}`,
          name: game.name,
          path: installPath,
          installed: true,
          size: parseInt(game.sizeOnDisk) || undefined,
          platform: 'Steam',
          metadata: {
            appid: game.appid,
            buildId: game.buildId,
            lastUpdated: game.lastUpdated,
          },
        });
      }

      console.log(`  Found ${scanResult.games.length} local Steam games`);
    }

    // Scan Steam Web API
    if (this.config?.scanWeb && this.client) {
      console.log('🌐 Scanning Steam Web API...');
      const ownedGames = await this.client.getOwnedGames();

      for (const game of ownedGames) {
        // Skip if already in local games
        const localExists = detectedGames.some(
          (g) => g.metadata?.appid === game.appid.toString()
        );
        if (localExists) continue;

        // Build icon/logo URLs
        const iconUrl = game.img_icon_url
          ? this.client.getGameIconUrl(game.appid.toString(), game.img_icon_url)
          : undefined;
        const logoUrl = game.img_logo_url
          ? this.client.getGameLogoUrl(game.appid.toString(), game.img_logo_url)
          : undefined;

        detectedGames.push({
          sourceId: this.metadata.id,
          id: `steam-web-${game.appid}`,
          name: game.name,
          installed: false,
          platform: 'Steam',
          metadata: {
            appid: game.appid.toString(),
            playtimeForever: game.playtime_forever,
            iconUrl,
            logoUrl,
          },
        });
      }

      console.log(`  Found ${ownedGames.length} owned Steam games`);
    }

    // Filter by installed status if requested
    if (this.config?.installedOnly) {
      return detectedGames.filter((g) => g.installed);
    }

    return detectedGames;
  }

  /**
   * Get a specific game
   */
  async getGame(gameId: string): Promise<DetectedGame | null> {
    const games = await this.scan();
    return games.find((g) => g.id === gameId) || null;
  }

  /**
   * Launch a game
   */
  async launch(gameId: string): Promise<void> {
    // Extract appId from game ID
    const appId = gameId.replace('steam-local-', '').replace('steam-web-', '');

    if (!this.client) {
      // Create a client just for launching
      this.client = new SteamClient();
    }

    await this.client.launchGame(appId);
    console.log(`🎮 Launched Steam game: ${appId}`);
  }

  /**
   * Install a game
   */
  async install(gameId: string): Promise<void> {
    const appId = gameId.replace('steam-local-', '').replace('steam-web-', '');

    if (!this.client) {
      this.client = new SteamClient();
    }

    await this.client.installGame(appId);
    console.log(`📥 Installing Steam game: ${appId}`);
  }

  /**
   * Uninstall a game
   */
  async uninstall(gameId: string): Promise<void> {
    const appId = gameId.replace('steam-local-', '').replace('steam-web-', '');

    if (!this.client) {
      this.client = new SteamClient();
    }

    await this.client.uninstallGame(appId);
    console.log(`🗑️  Uninstalling Steam game: ${appId}`);
  }

  /**
   * Cleanup resources
   */
  async cleanup(): Promise<void> {
    this.scanner = undefined;
    this.client = undefined;
    this.initialized = false;
  }
}
