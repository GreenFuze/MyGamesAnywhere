/**
 * Centralized Configuration Manager
 * Handles loading/saving configuration from multiple sources
 */

import { readFile, writeFile, mkdir } from 'fs/promises';
import { existsSync } from 'fs';
import { dirname, join } from 'path';
import { homedir } from 'os';
import type { MyGamesAnywhereConfig } from './types.js';
import { MyGamesAnywhereConfigSchema } from './types.js';

/**
 * Configuration Manager
 * Loads config from file, environment variables, or provides defaults
 */
export class ConfigManager {
  private config: MyGamesAnywhereConfig = {};
  private configPath: string;

  /**
   * Create a new ConfigManager
   * @param configPath Path to config file (default: ~/.mygamesanywhere/config.json)
   */
  constructor(configPath?: string) {
    this.configPath =
      configPath ||
      join(homedir(), '.mygamesanywhere', 'config.json');
  }

  /**
   * Load configuration from file and environment variables
   */
  async load(): Promise<MyGamesAnywhereConfig> {
    // 1. Load from file if exists
    if (existsSync(this.configPath)) {
      const content = await readFile(this.configPath, 'utf-8');
      const fileConfig = JSON.parse(content);
      this.config = MyGamesAnywhereConfigSchema.parse(fileConfig);
    }

    // 2. Override with environment variables
    this.loadFromEnv();

    return this.config;
  }

  /**
   * Save configuration to file
   */
  async save(config: MyGamesAnywhereConfig): Promise<void> {
    // Validate
    const validated = MyGamesAnywhereConfigSchema.parse(config);

    // Ensure directory exists
    const dir = dirname(this.configPath);
    if (!existsSync(dir)) {
      await mkdir(dir, { recursive: true });
    }

    // Save as pretty JSON
    const json = JSON.stringify(validated, null, 2);
    await writeFile(this.configPath, json, 'utf-8');

    this.config = validated;
  }

  /**
   * Get current configuration
   */
  get(): MyGamesAnywhereConfig {
    return this.config;
  }

  /**
   * Get Steam configuration
   */
  getSteam() {
    return this.config.steam;
  }

  /**
   * Get Google Drive configuration
   */
  getGoogleDrive() {
    return this.config.googleDrive;
  }

  /**
   * Get IGDB configuration
   */
  getIGDB() {
    return this.config.igdb;
  }

  /**
   * Update Steam configuration
   */
  async updateSteam(steam: Partial<MyGamesAnywhereConfig['steam']>): Promise<void> {
    this.config.steam = {
      ...this.config.steam,
      ...steam,
    };
    await this.save(this.config);
  }

  /**
   * Update Google Drive configuration
   */
  async updateGoogleDrive(
    googleDrive: Partial<MyGamesAnywhereConfig['googleDrive']>
  ): Promise<void> {
    this.config.googleDrive = {
      ...this.config.googleDrive,
      ...googleDrive,
    };
    await this.save(this.config);
  }

  /**
   * Update IGDB configuration
   */
  async updateIGDB(igdb: Partial<MyGamesAnywhereConfig['igdb']>): Promise<void> {
    this.config.igdb = {
      ...this.config.igdb,
      ...igdb,
    };
    await this.save(this.config);
  }

  /**
   * Clear all configuration
   */
  async clear(): Promise<void> {
    this.config = {};
    if (existsSync(this.configPath)) {
      await writeFile(this.configPath, '{}', 'utf-8');
    }
  }

  /**
   * Load configuration from environment variables
   * Supports both direct env vars and .env files
   */
  private loadFromEnv(): void {
    // Steam
    if (process.env.STEAM_API_KEY || process.env.STEAM_USERNAME) {
      this.config.steam = {
        ...this.config.steam,
        ...(process.env.STEAM_API_KEY && { apiKey: process.env.STEAM_API_KEY }),
        ...(process.env.STEAM_USERNAME && { username: process.env.STEAM_USERNAME }),
      };
    }

    // Google Drive
    if (
      process.env.GOOGLE_CLIENT_ID ||
      process.env.GOOGLE_CLIENT_SECRET ||
      process.env.GOOGLE_REDIRECT_URI
    ) {
      this.config.googleDrive = {
        ...this.config.googleDrive,
        ...(process.env.GOOGLE_CLIENT_ID && {
          clientId: process.env.GOOGLE_CLIENT_ID,
        }),
        ...(process.env.GOOGLE_CLIENT_SECRET && {
          clientSecret: process.env.GOOGLE_CLIENT_SECRET,
        }),
        ...(process.env.GOOGLE_REDIRECT_URI && {
          redirectUri: process.env.GOOGLE_REDIRECT_URI,
        }),
      };
    }

    // IGDB
    if (process.env.IGDB_CLIENT_ID || process.env.IGDB_CLIENT_SECRET) {
      this.config.igdb = {
        ...this.config.igdb,
        ...(process.env.IGDB_CLIENT_ID && {
          clientId: process.env.IGDB_CLIENT_ID,
        }),
        ...(process.env.IGDB_CLIENT_SECRET && {
          clientSecret: process.env.IGDB_CLIENT_SECRET,
        }),
      };
    }
  }

  /**
   * Get config file path
   */
  getConfigPath(): string {
    return this.configPath;
  }

  /**
   * Check if a platform is configured
   */
  isConfigured(platform: keyof MyGamesAnywhereConfig): boolean {
    return !!this.config[platform];
  }

  /**
   * Validate current configuration
   */
  validate(): boolean {
    try {
      MyGamesAnywhereConfigSchema.parse(this.config);
      return true;
    } catch {
      return false;
    }
  }
}

/**
 * Global singleton instance
 */
let globalConfigManager: ConfigManager | null = null;

/**
 * Get or create the global ConfigManager instance
 */
export function getConfigManager(configPath?: string): ConfigManager {
  if (!globalConfigManager) {
    globalConfigManager = new ConfigManager(configPath);
  }
  return globalConfigManager;
}
