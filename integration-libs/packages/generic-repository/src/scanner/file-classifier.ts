/**
 * File Classifier
 * Identifies file types based on extension, content, and heuristics
 */

import { FileClassification, type ClassifiedFile, type FileInfo } from '../types.js';

/**
 * Extension to classification mapping
 */
const EXTENSION_MAP: Record<string, FileClassification> = {
  // Archives
  zip: FileClassification.ARCHIVE,
  '7z': FileClassification.ARCHIVE,
  rar: FileClassification.ARCHIVE,
  tar: FileClassification.ARCHIVE,
  gz: FileClassification.ARCHIVE,
  bz2: FileClassification.ARCHIVE,
  xz: FileClassification.ARCHIVE,

  // Executables - Windows
  exe: FileClassification.EXECUTABLE,
  bat: FileClassification.EXECUTABLE,
  cmd: FileClassification.EXECUTABLE,
  msi: FileClassification.INSTALLER,

  // Executables - Linux/Mac
  sh: FileClassification.EXECUTABLE,
  run: FileClassification.EXECUTABLE,
  app: FileClassification.EXECUTABLE,
  deb: FileClassification.INSTALLER,
  rpm: FileClassification.INSTALLER,
  pkg: FileClassification.INSTALLER,
  dmg: FileClassification.INSTALLER,

  // ROMs - Nintendo
  nes: FileClassification.ROM,
  snes: FileClassification.ROM,
  sfc: FileClassification.ROM,
  n64: FileClassification.ROM,
  z64: FileClassification.ROM,
  v64: FileClassification.ROM,
  nds: FileClassification.ROM,
  '3ds': FileClassification.ROM,
  gba: FileClassification.ROM,
  gbc: FileClassification.ROM,
  gb: FileClassification.ROM,

  // ROMs - Sega
  smd: FileClassification.ROM,
  gen: FileClassification.ROM,
  bin: FileClassification.ROM, // Also could be other things
  sms: FileClassification.ROM,
  gg: FileClassification.ROM,
  '32x': FileClassification.ROM,
  cue: FileClassification.ROM, // Sega CD
  iso: FileClassification.ROM, // Also general disc image

  // ROMs - Sony
  ps1: FileClassification.ROM,
  ps2: FileClassification.ROM,
  psp: FileClassification.ROM,

  // ROMs - Other
  smc: FileClassification.ROM,
  fig: FileClassification.ROM,
  swc: FileClassification.ROM,

  // Documents
  txt: FileClassification.DOCUMENT,
  md: FileClassification.DOCUMENT,
  pdf: FileClassification.DOCUMENT,
  doc: FileClassification.DOCUMENT,
  docx: FileClassification.DOCUMENT,
  nfo: FileClassification.DOCUMENT,
  rtf: FileClassification.DOCUMENT,

  // Images
  jpg: FileClassification.IMAGE,
  jpeg: FileClassification.IMAGE,
  png: FileClassification.IMAGE,
  gif: FileClassification.IMAGE,
  bmp: FileClassification.IMAGE,
  tga: FileClassification.IMAGE,
};

/**
 * Installer detection patterns
 */
const INSTALLER_PATTERNS = [
  /setup/i,
  /install/i,
  /installer/i,
  /^inst/i,
];

/**
 * File classifier
 */
export class FileClassifier {
  /**
   * Classify a file
   */
  classify(fileInfo: FileInfo): ClassifiedFile {
    const extension = fileInfo.extension.toLowerCase();
    const filename = fileInfo.name.toLowerCase();

    // Check if it's a directory
    if (fileInfo.isDirectory) {
      return {
        path: fileInfo.path,
        classification: FileClassification.DIRECTORY,
        extension,
        size: fileInfo.size,
        isExecutable: false,
      };
    }

    // Get base classification from extension
    let classification = EXTENSION_MAP[extension] || FileClassification.UNKNOWN;

    // Refine executable/installer detection
    if (classification === FileClassification.EXECUTABLE) {
      if (this.isInstallerFilename(filename)) {
        classification = FileClassification.INSTALLER;
      }
    }

    // Check for special cases
    const isExecutable = this.isExecutableFile(extension, classification);

    return {
      path: fileInfo.path,
      classification,
      extension,
      size: fileInfo.size,
      isExecutable,
      metadata: {
        filename: fileInfo.name,
        modifiedAt: fileInfo.modifiedAt,
      },
    };
  }

  /**
   * Check if a file is an installer based on filename
   */
  private isInstallerFilename(filename: string): boolean {
    return INSTALLER_PATTERNS.some((pattern) => pattern.test(filename));
  }

  /**
   * Check if file is executable
   */
  private isExecutableFile(
    extension: string,
    classification: FileClassification
  ): boolean {
    return (
      classification === FileClassification.EXECUTABLE ||
      classification === FileClassification.INSTALLER ||
      ['exe', 'bat', 'cmd', 'sh', 'run', 'app'].includes(extension)
    );
  }

  /**
   * Check if file is likely a ROM
   */
  isROM(classifiedFile: ClassifiedFile): boolean {
    return classifiedFile.classification === FileClassification.ROM;
  }

  /**
   * Check if file is an archive
   */
  isArchive(classifiedFile: ClassifiedFile): boolean {
    return classifiedFile.classification === FileClassification.ARCHIVE;
  }

  /**
   * Check if file is an installer
   */
  isInstaller(classifiedFile: ClassifiedFile): boolean {
    return classifiedFile.classification === FileClassification.INSTALLER;
  }

  /**
   * Check if file is a game executable (not installer)
   */
  isGameExecutable(classifiedFile: ClassifiedFile): boolean {
    return (
      classifiedFile.classification === FileClassification.EXECUTABLE &&
      classifiedFile.isExecutable &&
      !this.isInstaller(classifiedFile)
    );
  }

  /**
   * Get ROM system from extension
   */
  getROMSystem(extension: string): string | null {
    const ext = extension.toLowerCase();

    const systemMap: Record<string, string> = {
      // Nintendo
      nes: 'NES',
      snes: 'SNES',
      sfc: 'SNES',
      n64: 'Nintendo 64',
      z64: 'Nintendo 64',
      v64: 'Nintendo 64',
      nds: 'Nintendo DS',
      '3ds': 'Nintendo 3DS',
      gba: 'Game Boy Advance',
      gbc: 'Game Boy Color',
      gb: 'Game Boy',

      // Sega
      smd: 'Sega Genesis',
      gen: 'Sega Genesis',
      sms: 'Sega Master System',
      gg: 'Game Gear',
      '32x': 'Sega 32X',

      // Sony
      ps1: 'PlayStation',
      ps2: 'PlayStation 2',
      psp: 'PlayStation Portable',

      // General
      iso: 'Disc Image',
      cue: 'Disc Image',
    };

    return systemMap[ext] || null;
  }
}
