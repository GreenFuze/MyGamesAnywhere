/**
 * Types for Steam Scanner
 */

import { z } from 'zod';

/**
 * Represents a Steam library folder path
 */
export interface SteamLibraryFolder {
  path: string;
  label: string;
  contentid: string;
  totalsize: string;
}

/**
 * Represents an installed Steam game
 */
export interface SteamGame {
  appid: string;
  name: string;
  installdir: string;
  libraryPath: string;
  lastUpdated: number;
  sizeOnDisk: string;
  buildId?: string;
  lastOwner?: string;
}

/**
 * Steam installation paths for different platforms
 */
export interface SteamPaths {
  steamPath: string;
  libraryFoldersVdf: string;
  steamAppsPath: string;
}

/**
 * Configuration for Steam scanner
 */
export interface ScanConfig {
  steamPath?: string;
  includeSizeOnDisk?: boolean;
  includeLastUpdated?: boolean;
}

/**
 * Result of a Steam scan operation
 */
export interface ScanResult {
  games: SteamGame[];
  libraryFolders: SteamLibraryFolder[];
  steamPath: string;
  scanDuration: number;
}

/**
 * VDF file structure (Valve Data Format)
 */
export type VDFValue = string | number | VDFObject;

export interface VDFObject {
  [key: string]: VDFValue;
}

// Zod schemas for runtime validation

export const SteamLibraryFolderSchema = z.object({
  path: z.string(),
  label: z.string(),
  contentid: z.string(),
  totalsize: z.string(),
});

export const SteamGameSchema = z.object({
  appid: z.string(),
  name: z.string(),
  installdir: z.string(),
  libraryPath: z.string(),
  lastUpdated: z.number(),
  sizeOnDisk: z.string(),
  buildId: z.string().optional(),
  lastOwner: z.string().optional(),
});

export const ScanResultSchema = z.object({
  games: z.array(SteamGameSchema),
  libraryFolders: z.array(SteamLibraryFolderSchema),
  steamPath: z.string(),
  scanDuration: z.number(),
});
