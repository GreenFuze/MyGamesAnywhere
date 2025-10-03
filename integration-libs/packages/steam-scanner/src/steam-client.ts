/**
 * Steam Client Integration
 * Handles game installation, launching, and Steam Web API integration
 */

import { exec } from 'child_process';
import { promisify } from 'util';
import { platform } from 'os';

const execAsync = promisify(exec);

/**
 * Steam Web API configuration
 */
export interface SteamWebAPIConfig {
  apiKey: string;
  username: string;
}

/**
 * Game info from Steam Web API
 */
export interface SteamWebGame {
  appid: number;
  name: string;
  playtime_forever: number;
  playtime_windows_forever?: number;
  playtime_mac_forever?: number;
  playtime_linux_forever?: number;
  img_icon_url?: string;
  img_logo_url?: string;
  has_community_visible_stats?: boolean;
}

/**
 * Steam Client
 * Provides integration with Steam client for game management
 */
export class SteamClient {
  private webAPIConfig?: SteamWebAPIConfig;
  private resolvedSteamId?: string;

  constructor(webAPIConfig?: SteamWebAPIConfig) {
    this.webAPIConfig = webAPIConfig;
  }

  /**
   * Resolve Steam username to Steam ID (64-bit)
   * Uses Steam's official API to convert username to Steam ID
   * Result is cached for subsequent calls
   */
  private async resolveSteamId(): Promise<string> {
    if (!this.webAPIConfig) {
      throw new Error('Steam Web API not configured');
    }

    // Return cached result if available
    if (this.resolvedSteamId) {
      return this.resolvedSteamId;
    }

    const url = 'https://api.steampowered.com/ISteamUser/ResolveVanityURL/v1/';
    const params = new URLSearchParams({
      key: this.webAPIConfig.apiKey,
      vanityurl: this.webAPIConfig.username,
    });

    const response = await fetch(`${url}?${params}`);

    if (!response.ok) {
      throw new Error(
        `Failed to resolve Steam username: ${response.status} ${response.statusText}`
      );
    }

    const data = (await response.json()) as {
      response: {
        success: number;
        steamid?: string;
        message?: string;
      };
    };

    if (data.response.success !== 1 || !data.response.steamid) {
      throw new Error(
        `Could not find Steam ID for username "${this.webAPIConfig.username}". ${
          data.response.message || 'User may not exist or profile may be private.'
        }`
      );
    }

    // Cache the result
    this.resolvedSteamId = data.response.steamid;
    return this.resolvedSteamId;
  }

  /**
   * Install a game via Steam client
   * Opens Steam's install dialog
   */
  async installGame(appId: string): Promise<void> {
    await this.openProtocolUrl(`steam://install/${appId}`);
  }

  /**
   * Uninstall a game via Steam client
   * Opens Steam's uninstall dialog
   */
  async uninstallGame(appId: string): Promise<void> {
    await this.openProtocolUrl(`steam://uninstall/${appId}`);
  }

  /**
   * Launch a game via Steam
   */
  async launchGame(appId: string): Promise<void> {
    await this.openProtocolUrl(`steam://rungameid/${appId}`);
  }

  /**
   * Open Steam store page for a game
   */
  async openStorePage(appId: string): Promise<void> {
    await this.openProtocolUrl(`steam://store/${appId}`);
  }

  /**
   * Open Steam client to library
   */
  async openLibrary(): Promise<void> {
    await this.openProtocolUrl('steam://open/games');
  }

  /**
   * Open Steam client to downloads page
   */
  async openDownloads(): Promise<void> {
    await this.openProtocolUrl('steam://open/downloads');
  }

  /**
   * Open Steam client to settings
   */
  async openSettings(): Promise<void> {
    await this.openProtocolUrl('steam://open/settings');
  }

  /**
   * Validate a game's files via Steam
   */
  async validateGameFiles(appId: string): Promise<void> {
    await this.openProtocolUrl(`steam://validate/${appId}`);
  }

  /**
   * Get user's owned games from Steam Web API
   * Requires Steam Web API key and Steam username
   */
  async getOwnedGames(): Promise<SteamWebGame[]> {
    if (!this.webAPIConfig) {
      throw new Error(
        'Steam Web API not configured. Provide apiKey and username in constructor.'
      );
    }

    // Resolve username to Steam ID
    const steamId = await this.resolveSteamId();

    const url = 'https://api.steampowered.com/IPlayerService/GetOwnedGames/v1/';
    const params = new URLSearchParams({
      key: this.webAPIConfig.apiKey,
      steamid: steamId,
      include_appinfo: '1',
      include_played_free_games: '1',
      format: 'json',
    });

    const response = await fetch(`${url}?${params}`);

    if (!response.ok) {
      throw new Error(
        `Steam Web API error: ${response.status} ${response.statusText}`
      );
    }

    const data = (await response.json()) as {
      response: { game_count: number; games: SteamWebGame[] };
    };

    return data.response.games || [];
  }

  /**
   * Get recently played games from Steam Web API
   */
  async getRecentlyPlayedGames(count: number = 10): Promise<SteamWebGame[]> {
    if (!this.webAPIConfig) {
      throw new Error('Steam Web API not configured');
    }

    // Resolve username to Steam ID
    const steamId = await this.resolveSteamId();

    const url =
      'https://api.steampowered.com/IPlayerService/GetRecentlyPlayedGames/v1/';
    const params = new URLSearchParams({
      key: this.webAPIConfig.apiKey,
      steamid: steamId,
      count: count.toString(),
      format: 'json',
    });

    const response = await fetch(`${url}?${params}`);

    if (!response.ok) {
      throw new Error(
        `Steam Web API error: ${response.status} ${response.statusText}`
      );
    }

    const data = (await response.json()) as {
      response: { total_count: number; games: SteamWebGame[] };
    };

    return data.response.games || [];
  }

  /**
   * Get app details from Steam Store API (no auth required)
   */
  async getAppDetails(appId: string): Promise<unknown> {
    const url = `https://store.steampowered.com/api/appdetails?appids=${appId}`;

    const response = await fetch(url);

    if (!response.ok) {
      throw new Error(
        `Steam Store API error: ${response.status} ${response.statusText}`
      );
    }

    const data = (await response.json()) as Record<
      string,
      { success: boolean; data?: unknown }
    >;

    if (!data[appId]?.success) {
      throw new Error(`Failed to get app details for ${appId}`);
    }

    return data[appId].data;
  }

  /**
   * Check if Steam client is running
   */
  async isSteamRunning(): Promise<boolean> {
    try {
      const platformName = platform();

      if (platformName === 'win32') {
        const { stdout } = await execAsync(
          'tasklist /FI "IMAGENAME eq steam.exe" /NH'
        );
        return stdout.toLowerCase().includes('steam.exe');
      } else if (platformName === 'darwin') {
        const { stdout } = await execAsync('pgrep -x Steam');
        return stdout.trim().length > 0;
      } else {
        // Linux
        const { stdout } = await execAsync('pgrep -x steam');
        return stdout.trim().length > 0;
      }
    } catch {
      return false;
    }
  }

  /**
   * Open a Steam protocol URL
   */
  private async openProtocolUrl(url: string): Promise<void> {
    const platformName = platform();

    if (platformName === 'win32') {
      // Windows: use 'start' command
      await execAsync(`start "" "${url}"`);
    } else if (platformName === 'darwin') {
      // macOS: use 'open' command
      await execAsync(`open "${url}"`);
    } else {
      // Linux: use 'xdg-open'
      await execAsync(`xdg-open "${url}"`);
    }
  }

  /**
   * Get Steam icon/logo URL for a game
   */
  getGameIconUrl(appId: string, iconHash: string): string {
    return `https://media.steampowered.com/steamcommunity/public/images/apps/${appId}/${iconHash}.jpg`;
  }

  /**
   * Get Steam logo URL for a game
   */
  getGameLogoUrl(appId: string, logoHash: string): string {
    return `https://media.steampowered.com/steamcommunity/public/images/apps/${appId}/${logoHash}.jpg`;
  }

  /**
   * Get Steam header image URL for a game
   */
  getGameHeaderUrl(appId: string): string {
    return `https://cdn.cloudflare.steamstatic.com/steam/apps/${appId}/header.jpg`;
  }

  /**
   * Get Steam library hero image URL
   */
  getGameHeroUrl(appId: string): string {
    return `https://cdn.cloudflare.steamstatic.com/steam/apps/${appId}/library_hero.jpg`;
  }

  /**
   * Get Steam library capsule image URL
   */
  getGameCapsuleUrl(appId: string): string {
    return `https://cdn.cloudflare.steamstatic.com/steam/apps/${appId}/library_600x900.jpg`;
  }
}
