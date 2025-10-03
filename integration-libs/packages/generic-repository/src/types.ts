/**
 * Generic Game Repository Types
 * Core data structures for game detection and management
 */

/**
 * Types of games that can be detected
 */
export enum GameType {
  /** Executable installer (.exe, .run) */
  INSTALLER_EXECUTABLE = 'installer_executable',
  /** Platform-specific installer (.msi, .pkg, .deb, .rpm) */
  INSTALLER_PLATFORM = 'installer_platform',
  /** Portable game directory (ready to run) */
  PORTABLE_GAME = 'portable_game',
  /** ROM file requiring an emulator */
  ROM = 'rom',
  /** Game requiring DOSBox */
  REQUIRES_DOSBOX = 'requires_dosbox',
  /** Game requiring ScummVM */
  REQUIRES_SCUMMVM = 'requires_scummvm',
  /** Game requiring other emulator */
  REQUIRES_EMULATOR = 'requires_emulator',
  /** Game is in an archive (needs extraction) */
  ARCHIVED = 'archived',
  /** Unknown/unrecognized format */
  UNKNOWN = 'unknown',
}

/**
 * File classification types
 */
export enum FileClassification {
  /** Single executable file */
  EXECUTABLE = 'executable',
  /** Installer file */
  INSTALLER = 'installer',
  /** Archive file */
  ARCHIVE = 'archive',
  /** ROM file */
  ROM = 'rom',
  /** Directory */
  DIRECTORY = 'directory',
  /** Text/document file */
  DOCUMENT = 'document',
  /** Image file */
  IMAGE = 'image',
  /** Unknown file type */
  UNKNOWN = 'unknown',
}

/**
 * Archive types
 */
export enum ArchiveType {
  ZIP = 'zip',
  SEVEN_ZIP = '7z',
  RAR = 'rar',
  TAR = 'tar',
  TAR_GZ = 'tar.gz',
  TAR_BZ2 = 'tar.bz2',
  TAR_XZ = 'tar.xz',
  UNKNOWN = 'unknown',
}

/**
 * Repository types
 */
export enum RepositoryType {
  LOCAL = 'local',
  GDRIVE = 'gdrive',
  ONEDRIVE = 'onedrive',
}

/**
 * Installation state
 */
export enum InstallationState {
  NOT_INSTALLED = 'not_installed',
  INSTALLING = 'installing',
  INSTALLED = 'installed',
  FAILED = 'failed',
}

/**
 * Detected game information
 */
export interface DetectedGame {
  /** Unique identifier */
  id: string;
  /** Detected game name */
  name: string;
  /** Game type */
  type: GameType;
  /** Location in repository */
  location: GameLocation;
  /** Metadata (if available) */
  metadata?: GameMetadata;
  /** Installation information */
  installation?: InstallationInfo;
  /** Detection confidence (0-1) */
  confidence: number;
  /** Detection timestamp */
  detectedAt: Date;
}

/**
 * Game location information
 */
export interface GameLocation {
  /** Repository type */
  repositoryType: RepositoryType;
  /** Path in repository */
  path: string;
  /** Whether the game is in an archive */
  isArchived: boolean;
  /** Archive parts (for multi-part archives) */
  archiveParts?: string[];
  /** Archive type */
  archiveType?: ArchiveType;
  /** Relative path to executable within archive/directory */
  executablePath?: string;
  /** File size in bytes */
  size?: number;
  /** Last modified date */
  modifiedAt?: Date;
}

/**
 * Game metadata
 */
export interface GameMetadata {
  /** Full game title */
  title?: string;
  /** Release year */
  releaseYear?: number;
  /** Publisher */
  publisher?: string;
  /** Developer */
  developer?: string;
  /** Genre */
  genre?: string[];
  /** Description */
  description?: string;
  /** Cover image URL */
  coverUrl?: string;
  /** IGDB ID */
  igdbId?: number;
  /** User rating */
  rating?: number;
  /** Source of metadata */
  source: MetadataSource;
}

/**
 * Metadata sources
 */
export enum MetadataSource {
  /** From IGDB API */
  IGDB = 'igdb',
  /** From sidecar file (.yaml, .json) */
  SIDECAR = 'sidecar',
  /** From executable metadata */
  EXECUTABLE = 'executable',
  /** From filename heuristics */
  HEURISTIC = 'heuristic',
  /** Manually provided */
  MANUAL = 'manual',
}

/**
 * Installation information
 */
export interface InstallationInfo {
  /** Installation state */
  state: InstallationState;
  /** Installed path (if installed) */
  installedPath?: string;
  /** Installation date */
  installedAt?: Date;
  /** Emulator path (for ROMs and games requiring emulators) */
  emulatorPath?: string;
  /** Emulator arguments */
  emulatorArgs?: string[];
  /** Installation size in bytes */
  installSize?: number;
  /** Launcher script path (if created) */
  launcherPath?: string;
}

/**
 * File classification result
 */
export interface ClassifiedFile {
  /** File path */
  path: string;
  /** File classification */
  classification: FileClassification;
  /** File extension */
  extension: string;
  /** File size */
  size: number;
  /** Is executable */
  isExecutable: boolean;
  /** Additional metadata */
  metadata?: Record<string, unknown>;
}

/**
 * Archive detection result
 */
export interface ArchiveInfo {
  /** Main archive path */
  mainPath: string;
  /** Archive type */
  type: ArchiveType;
  /** Is multi-part archive */
  isMultiPart: boolean;
  /** All parts (if multi-part) */
  parts?: string[];
  /** Estimated extracted size */
  extractedSize?: number;
}

/**
 * Scan result
 */
export interface ScanResult {
  /** Repository path */
  repositoryPath: string;
  /** Detected games */
  games: DetectedGame[];
  /** Scan duration in ms */
  duration: number;
  /** Number of files scanned */
  filesScanned: number;
  /** Number of directories scanned */
  directoriesScanned: number;
  /** Errors encountered */
  errors: ScanError[];
}

/**
 * Scan error
 */
export interface ScanError {
  /** Error path */
  path: string;
  /** Error message */
  message: string;
  /** Error code */
  code?: string;
}

/**
 * ROM file extensions database
 */
export interface ROMExtension {
  /** File extension (without dot) */
  extension: string;
  /** System name */
  system: string;
  /** Common emulators */
  emulators: string[];
}

/**
 * Emulator information
 */
export interface EmulatorInfo {
  /** Emulator name */
  name: string;
  /** Executable path */
  path: string;
  /** Supported systems */
  systems: string[];
  /** Command line template */
  commandTemplate: string;
  /** Whether it's installed */
  isInstalled: boolean;
}

/**
 * Repository adapter interface
 */
export interface RepositoryAdapter {
  /** Repository type */
  type: RepositoryType;

  /** List files in a directory */
  listFiles(path: string): Promise<string[]>;

  /** Get file metadata */
  getFileInfo(path: string): Promise<FileInfo>;

  /** Check if path exists */
  exists(path: string): Promise<boolean>;

  /** Check if path is a directory */
  isDirectory(path: string): Promise<boolean>;

  /** Download file to local temp */
  downloadToTemp(path: string): Promise<string>;

  /** Get file size */
  getSize(path: string): Promise<number>;
}

/**
 * File information
 */
export interface FileInfo {
  /** File path */
  path: string;
  /** File name */
  name: string;
  /** File size in bytes */
  size: number;
  /** Is directory */
  isDirectory: boolean;
  /** Last modified date */
  modifiedAt: Date;
  /** File extension */
  extension: string;
}

/**
 * Scanner configuration
 */
export interface ScannerConfig {
  /** Maximum directory depth */
  maxDepth?: number;
  /** Include hidden files */
  includeHidden?: boolean;
  /** File patterns to exclude */
  excludePatterns?: string[];
  /** Enable archive extraction */
  extractArchives?: boolean;
  /** Enable metadata fetching */
  fetchMetadata?: boolean;
  /** Enable parallel scanning */
  parallel?: boolean;
  /** Maximum parallel operations */
  maxParallel?: number;
}
