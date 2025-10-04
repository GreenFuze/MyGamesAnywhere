/**
 * LaunchBox Identifier Plugin
 * Identifies games using LaunchBox Games Database
 */

import { LaunchBoxIdentifier } from '@mygamesanywhere/game-identifier';
import type { LaunchBoxIdentifierConfig } from '@mygamesanywhere/game-identifier';
import {
  PluginType,
  type IdentifierPlugin,
  type DetectedGame,
  type IdentifiedGame,
  type GameMetadata,
  type PluginMetadata,
  type PluginConfigSchema,
} from '@mygamesanywhere/plugin-system';

/**
 * LaunchBox identifier plugin
 */
export class LaunchBoxIdentifierPlugin implements IdentifierPlugin {
  readonly type = PluginType.IDENTIFIER;
  readonly metadata: PluginMetadata = {
    id: 'launchbox-identifier',
    name: 'LaunchBox Games Database',
    version: '0.1.0',
    description: 'Identify games using LaunchBox Games Database (100,000+ games)',
    author: 'MyGamesAnywhere',
  };

  readonly configSchema: PluginConfigSchema = {
    properties: {
      autoDownload: {
        type: 'boolean',
        default: true,
        description: 'Auto-download metadata if not exists',
      },
      minConfidence: {
        type: 'number',
        default: 0.5,
        description: 'Minimum confidence for fuzzy matching',
      },
      maxResults: {
        type: 'number',
        default: 10,
        description: 'Maximum results for fuzzy search',
      },
    },
  };

  private identifier?: LaunchBoxIdentifier;
  private config?: LaunchBoxIdentifierConfig;

  /**
   * Initialize the plugin
   */
  async initialize(config?: LaunchBoxIdentifierConfig): Promise<void> {
    this.config = {
      autoDownload: true,
      minConfidence: 0.5,
      maxResults: 10,
      ...config,
    };

    // Initialize identifier
    this.identifier = new LaunchBoxIdentifier(this.config);

    console.log(`✅ Initialized LaunchBox identifier plugin`);
  }

  /**
   * Check if plugin is ready
   */
  async isReady(): Promise<boolean> {
    if (!this.identifier) return false;
    return await this.identifier.isReady();
  }

  /**
   * Identify a detected game
   */
  async identify(game: DetectedGame): Promise<IdentifiedGame> {
    if (!this.identifier) {
      throw new Error('LaunchBox identifier plugin not initialized');
    }

    // Convert plugin-system DetectedGame to game-identifier DetectedGame
    const identifierGame: import('@mygamesanywhere/game-identifier').DetectedGame = {
      id: game.id,
      name: game.name,
      type: 'file' as const,
      confidence: 0.8,
      path: game.path || '',
      size: game.size || 0,
    };

    // Identify using LaunchBox
    const result = await this.identifier.identify(identifierGame);

    // Convert back to plugin-system IdentifiedGame
    return {
      detectedGame: game,
      metadata: result.metadata,
      matchConfidence: result.matchConfidence,
      source: result.source,
    };
  }

  /**
   * Search for games
   */
  async search(query: string, platform?: string): Promise<GameMetadata[]> {
    if (!this.identifier) {
      throw new Error('LaunchBox identifier plugin not initialized');
    }

    return await this.identifier.search(query, platform);
  }

  /**
   * Update metadata database
   */
  async update(): Promise<void> {
    if (!this.identifier) {
      throw new Error('LaunchBox identifier plugin not initialized');
    }

    await this.identifier.update();
  }

  /**
   * Cleanup resources
   */
  async cleanup(): Promise<void> {
    if (this.identifier) {
      this.identifier.close();
    }
    this.identifier = undefined;
  }
}
