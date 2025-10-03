/**
 * @mygamesanywhere/config
 *
 * Centralized configuration management for all integrations
 */

export { ConfigManager, getConfigManager } from './config-manager.js';

export type {
  MyGamesAnywhereConfig,
  SteamConfig,
  GoogleDriveConfig,
  IGDBConfig,
  EpicConfig,
  GOGConfig,
  XboxConfig,
} from './types.js';

export {
  MyGamesAnywhereConfigSchema,
  SteamConfigSchema,
  GoogleDriveConfigSchema,
  IGDBConfigSchema,
  EpicConfigSchema,
  GOGConfigSchema,
  XboxConfigSchema,
} from './types.js';
