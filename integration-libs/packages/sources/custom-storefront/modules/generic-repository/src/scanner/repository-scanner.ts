/**
 * Repository Scanner
 * Main entry point for scanning game repositories
 */

import { v4 as uuidv4 } from 'uuid';
import type {
  RepositoryAdapter,
  ScannerConfig,
  ScanResult,
  DetectedGame,
  ScanError,
  FileInfo,
  ClassifiedFile,
} from '../types.js';
import { GameType } from '../types.js';
import { RecursiveWalker } from './recursive-walker.js';
import { FileClassifier } from './file-classifier.js';
import { ArchiveDetector } from './archive-detector.js';

/**
 * Repository scanner
 * Scans a repository and detects games
 */
export class RepositoryScanner {
  private adapter: RepositoryAdapter;
  private walker: RecursiveWalker;
  private classifier: FileClassifier;
  private archiveDetector: ArchiveDetector;
  private detectedGames: DetectedGame[] = [];
  private errors: ScanError[] = [];
  private classifiedFiles: ClassifiedFile[] = [];

  constructor(adapter: RepositoryAdapter, config?: ScannerConfig) {
    this.adapter = adapter;
    this.walker = new RecursiveWalker(adapter, config);
    this.classifier = new FileClassifier();
    this.archiveDetector = new ArchiveDetector(adapter);
  }

  /**
   * Scan the repository and detect games
   */
  async scan(path: string = ''): Promise<ScanResult> {
    const startTime = Date.now();

    // Reset state
    this.detectedGames = [];
    this.errors = [];
    this.classifiedFiles = [];

    try {
      // Walk the directory tree and classify files
      await this.walker.walk(path, async (fileInfo) => {
        await this.processFile(fileInfo);
      });

      // Analyze classified files to detect games
      await this.detectGames();
    } catch (error) {
      this.errors.push({
        path,
        message: error instanceof Error ? error.message : String(error),
      });
    }

    const duration = Date.now() - startTime;

    return {
      repositoryPath: path,
      games: this.detectedGames,
      duration,
      filesScanned: this.walker.getFilesScanned(),
      directoriesScanned: this.walker.getDirectoriesScanned(),
      errors: this.errors,
    };
  }

  /**
   * Process a file
   */
  private async processFile(fileInfo: FileInfo): Promise<void> {
    try {
      // Classify the file
      const classified = this.classifier.classify(fileInfo);
      this.classifiedFiles.push(classified);
    } catch (error) {
      this.errors.push({
        path: fileInfo.path,
        message: error instanceof Error ? error.message : String(error),
      });
    }
  }

  /**
   * Detect games from classified files
   */
  private async detectGames(): Promise<void> {
    // Group files by directory
    const filesByDirectory = this.groupFilesByDirectory(this.classifiedFiles);

    // Process each directory
    for (const [directory, files] of filesByDirectory) {
      await this.detectGamesInDirectory(directory, files);
    }
  }

  /**
   * Detect games in a specific directory
   */
  private async detectGamesInDirectory(
    _directory: string,
    files: ClassifiedFile[]
  ): Promise<void> {
    // Check for installers
    const installers = files.filter((f) => this.classifier.isInstaller(f));
    for (const installer of installers) {
      this.addDetectedGame(installer, GameType.INSTALLER_EXECUTABLE, 0.9);
    }

    // Check for ROMs
    const roms = files.filter((f) => this.classifier.isROM(f));
    for (const rom of roms) {
      this.addDetectedGame(rom, GameType.ROM, 0.95);
    }

    // Check for archives
    const archives = files.filter((f) => this.classifier.isArchive(f));
    for (const archive of archives) {
      const archiveInfo = await this.archiveDetector.detectArchive(archive);
      if (archiveInfo) {
        this.addDetectedGame(
          archive,
          GameType.ARCHIVED,
          archiveInfo.isMultiPart ? 0.85 : 0.8,
          archiveInfo
        );
      }
    }

    // Check for portable games (directories with executables)
    const executables = files.filter((f) =>
      this.classifier.isGameExecutable(f)
    );
    if (executables.length > 0) {
      // Pick the most likely executable
      const mainExe = this.findMainExecutable(executables);
      if (mainExe) {
        this.addDetectedGame(mainExe, GameType.PORTABLE_GAME, 0.7);
      }
    }
  }

  /**
   * Add a detected game
   */
  private addDetectedGame(
    file: ClassifiedFile,
    type: GameType,
    confidence: number,
    _additionalData?: unknown
  ): void {
    const game: DetectedGame = {
      id: uuidv4(),
      name: this.extractGameName(file.path),
      type,
      location: {
        repositoryType: this.adapter.type,
        path: file.path,
        isArchived: type === GameType.ARCHIVED,
        size: file.size,
      },
      confidence,
      detectedAt: new Date(),
    };

    this.detectedGames.push(game);
  }

  /**
   * Extract game name from file path
   */
  private extractGameName(path: string): string {
    // Get filename without extension
    const parts = path.split(/[/\\]/);
    const filename = parts[parts.length - 1];
    const nameWithoutExt = filename.replace(/\.[^.]+$/, '');

    // Clean up common patterns
    let name = nameWithoutExt;

    // Remove version numbers like (v1.0), [1.2], etc.
    name = name.replace(/[\[(]v?\d+\.?\d*[\])]/gi, '');

    // Remove common tags like (USA), [English], etc.
    name = name.replace(/[\[(](USA|Europe|Japan|English|Multi\d*)[\])]/gi, '');

    // Remove underscores and clean up
    name = name.replace(/_/g, ' ').trim();

    // Capitalize first letter of each word
    name = name.replace(/\b\w/g, (c) => c.toUpperCase());

    return name || filename;
  }

  /**
   * Find the main executable in a list
   */
  private findMainExecutable(executables: ClassifiedFile[]): ClassifiedFile | null {
    if (executables.length === 0) return null;
    if (executables.length === 1) return executables[0];

    // Scoring system for exe selection
    const scores = executables.map((exe) => {
      let score = 0;
      const name = exe.path.toLowerCase();

      // Prefer exe in root
      const depth = exe.path.split(/[/\\]/).length;
      score += (10 - depth) * 10;

      // Prefer larger files (usually the main exe)
      score += Math.min(exe.size / 1024 / 1024, 100); // Max 100 points for size

      // Penalize common non-game executables
      if (name.includes('unins')) score -= 100; // Uninstaller
      if (name.includes('setup')) score -= 100; // Setup
      if (name.includes('config')) score -= 50; // Config tool
      if (name.includes('launcher')) score -= 20; // Launcher (might be legit)

      return { exe, score };
    });

    // Return exe with highest score
    scores.sort((a, b) => b.score - a.score);
    return scores[0].exe;
  }

  /**
   * Group files by directory
   */
  private groupFilesByDirectory(
    files: ClassifiedFile[]
  ): Map<string, ClassifiedFile[]> {
    const groups = new Map<string, ClassifiedFile[]>();

    for (const file of files) {
      const parts = file.path.split(/[/\\]/);
      const directory = parts.slice(0, -1).join('/') || '/';

      if (!groups.has(directory)) {
        groups.set(directory, []);
      }

      groups.get(directory)!.push(file);
    }

    return groups;
  }
}
