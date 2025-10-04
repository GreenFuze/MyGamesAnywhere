/**
 * Centralized Configuration Types
 * All integration credentials and settings in one place
 */

import { z } from 'zod';

/**
 * Steam Integration Configuration
 */
export interface SteamConfig {
  /** Steam Web API Key from https://steamcommunity.com/dev/apikey */
  apiKey?: string;
  /** Steam username (e.g., "greenfuze") - will be auto-resolved to Steam ID */
  username?: string;
  /** Custom Steam installation path (optional) */
  customPath?: string;
}

/**
 * Google Drive Configuration
 */
export interface GoogleDriveConfig {
  /** Google OAuth Client ID */
  clientId?: string;
  /** Google OAuth Client Secret */
  clientSecret?: string;
  /** OAuth Redirect URI */
  redirectUri?: string;
}

/**
 * IGDB (Internet Game Database) Configuration
 */
export interface IGDBConfig {
  /** Twitch Client ID */
  clientId?: string;
  /** Twitch Client Secret */
  clientSecret?: string;
}

/**
 * Epic Games Store Configuration (Future)
 */
export interface EpicConfig {
  /** Epic Games account email */
  email?: string;
  /** Custom installation path */
  customPath?: string;
}

/**
 * GOG Configuration (Future)
 */
export interface GOGConfig {
  /** GOG API token */
  token?: string;
  /** Custom installation path */
  customPath?: string;
}

/**
 * Xbox App Configuration (Future)
 */
export interface XboxConfig {
  /** Xbox account email */
  email?: string;
}

/**
 * Complete MyGamesAnywhere Configuration
 */
export interface MyGamesAnywhereConfig {
  /** Steam integration settings */
  steam?: SteamConfig;
  /** Google Drive integration settings */
  googleDrive?: GoogleDriveConfig;
  /** IGDB integration settings */
  igdb?: IGDBConfig;
  /** Epic Games Store settings */
  epic?: EpicConfig;
  /** GOG settings */
  gog?: GOGConfig;
  /** Xbox settings */
  xbox?: XboxConfig;
}

/**
 * Zod Schemas for Validation
 */

export const SteamConfigSchema = z.object({
  apiKey: z.string().optional(),
  username: z.string().optional(),
  customPath: z.string().optional(),
});

export const GoogleDriveConfigSchema = z.object({
  clientId: z.string().optional(),
  clientSecret: z.string().optional(),
  redirectUri: z.string().url().optional(),
});

export const IGDBConfigSchema = z.object({
  clientId: z.string().optional(),
  clientSecret: z.string().optional(),
});

export const EpicConfigSchema = z.object({
  email: z.string().email().optional(),
  customPath: z.string().optional(),
});

export const GOGConfigSchema = z.object({
  token: z.string().optional(),
  customPath: z.string().optional(),
});

export const XboxConfigSchema = z.object({
  email: z.string().email().optional(),
});

export const MyGamesAnywhereConfigSchema = z.object({
  steam: SteamConfigSchema.optional(),
  googleDrive: GoogleDriveConfigSchema.optional(),
  igdb: IGDBConfigSchema.optional(),
  epic: EpicConfigSchema.optional(),
  gog: GOGConfigSchema.optional(),
  xbox: XboxConfigSchema.optional(),
});
