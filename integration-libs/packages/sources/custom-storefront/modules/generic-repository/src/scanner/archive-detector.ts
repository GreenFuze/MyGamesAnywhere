/**
 * Archive Detector
 * Detects archives and multi-part archives
 */

import { basename, dirname, join } from 'path';
import type {
  ArchiveInfo,
  ClassifiedFile,
  RepositoryAdapter,
} from '../types.js';
import { FileClassification, ArchiveType } from '../types.js';

/**
 * Multi-part archive patterns
 */
interface MultiPartPattern {
  /** Regex to match the pattern */
  pattern: RegExp;
  /** Function to generate all part names */
  generateParts: (matchedPart: string) => string[];
}

/**
 * Known multi-part patterns
 */
const MULTI_PART_PATTERNS: MultiPartPattern[] = [
  // .part1.rar, .part2.rar, etc.
  {
    pattern: /^(.+)\.part(\d+)\.(rar|zip|7z)$/i,
    generateParts: (matched) => {
      const match = matched.match(/^(.+)\.part(\d+)\.(rar|zip|7z)$/i);
      if (!match) return [];
      const [, name, , ext] = match;
      const parts: string[] = [];
      // Try up to 99 parts
      for (let i = 1; i <= 99; i++) {
        parts.push(`${name}.part${i}.${ext}`);
      }
      return parts;
    },
  },

  // .z01, .z02, etc. (with .zip as the last part)
  {
    pattern: /^(.+)\.z(\d{2})$/i,
    generateParts: (matched) => {
      const match = matched.match(/^(.+)\.z(\d{2})$/i);
      if (!match) return [];
      const [, name] = match;
      const parts: string[] = [];
      // .z01 to .z99, then .zip
      for (let i = 1; i <= 99; i++) {
        parts.push(`${name}.z${i.toString().padStart(2, '0')}`);
      }
      parts.push(`${name}.zip`);
      return parts;
    },
  },

  // .001, .002, etc.
  {
    pattern: /^(.+)\.(\d{3})$/i,
    generateParts: (matched) => {
      const match = matched.match(/^(.+)\.(\d{3})$/i);
      if (!match) return [];
      const [, name] = match;
      const parts: string[] = [];
      // .001 to .999
      for (let i = 1; i <= 999; i++) {
        parts.push(`${name}.${i.toString().padStart(3, '0')}`);
      }
      return parts;
    },
  },

  // .rar, .r00, .r01, etc.
  {
    pattern: /^(.+)\.(r\d{2})$/i,
    generateParts: (matched) => {
      const match = matched.match(/^(.+)\.(r\d{2})$/i);
      if (!match) return [];
      const [, name] = match;
      const parts: string[] = [
        `${name}.rar`,
      ];
      // .r00 to .r99
      for (let i = 0; i <= 99; i++) {
        parts.push(`${name}.r${i.toString().padStart(2, '0')}`);
      }
      return parts;
    },
  },
];

/**
 * Archive detector
 */
export class ArchiveDetector {
  private adapter: RepositoryAdapter;

  constructor(adapter: RepositoryAdapter) {
    this.adapter = adapter;
  }

  /**
   * Detect if a file is an archive and get information
   */
  async detectArchive(
    classifiedFile: ClassifiedFile
  ): Promise<ArchiveInfo | null> {
    // Must be classified as archive
    if (classifiedFile.classification !== FileClassification.ARCHIVE) {
      return null;
    }

    const archiveType = this.getArchiveType(classifiedFile.extension);
    if (!archiveType) {
      return null;
    }

    // Check if it's multi-part
    const multiPartInfo = await this.detectMultiPart(classifiedFile);

    if (multiPartInfo) {
      return {
        mainPath: multiPartInfo.mainPath,
        type: archiveType,
        isMultiPart: true,
        parts: multiPartInfo.parts,
      };
    }

    // Single-part archive
    return {
      mainPath: classifiedFile.path,
      type: archiveType,
      isMultiPart: false,
    };
  }

  /**
   * Detect multi-part archive
   */
  private async detectMultiPart(
    classifiedFile: ClassifiedFile
  ): Promise<{ mainPath: string; parts: string[] } | null> {
    const filename = basename(classifiedFile.path);
    const dirPath = dirname(classifiedFile.path);

    // Try each multi-part pattern
    for (const { pattern, generateParts } of MULTI_PART_PATTERNS) {
      if (pattern.test(filename)) {
        const potentialParts = generateParts(filename);

        // Check which parts actually exist
        const existingParts: string[] = [];
        for (const partName of potentialParts) {
          const partPath = join(dirPath, partName);
          if (await this.adapter.exists(partPath)) {
            existingParts.push(partPath);
          } else {
            // If we hit a missing part, stop looking
            break;
          }
        }

        // If we found multiple parts, it's multi-part
        if (existingParts.length > 1) {
          return {
            mainPath: existingParts[0],
            parts: existingParts,
          };
        }
      }
    }

    return null;
  }

  /**
   * Get archive type from extension
   */
  private getArchiveType(extension: string): ArchiveType | null {
    const ext = extension.toLowerCase();

    const typeMap: Record<string, ArchiveType> = {
      zip: ArchiveType.ZIP,
      '7z': ArchiveType.SEVEN_ZIP,
      rar: ArchiveType.RAR,
      tar: ArchiveType.TAR,
      gz: ArchiveType.TAR_GZ,
      bz2: ArchiveType.TAR_BZ2,
      xz: ArchiveType.TAR_XZ,
    };

    return typeMap[ext] || null;
  }

  /**
   * Check if filename suggests it's part of a multi-part archive
   */
  isMultiPartName(filename: string): boolean {
    return MULTI_PART_PATTERNS.some(({ pattern }) => pattern.test(filename));
  }

  /**
   * Group related archive parts
   * Given a list of files, group them by archive sets
   */
  groupArchiveParts(files: ClassifiedFile[]): Map<string, ClassifiedFile[]> {
    const groups = new Map<string, ClassifiedFile[]>();

    for (const file of files) {
      if (file.classification !== FileClassification.ARCHIVE) {
        continue;
      }

      const filename = basename(file.path);
      const baseName = this.getArchiveBaseName(filename);

      if (!groups.has(baseName)) {
        groups.set(baseName, []);
      }

      groups.get(baseName)!.push(file);
    }

    return groups;
  }

  /**
   * Get base name for archive (without part numbers)
   */
  private getArchiveBaseName(filename: string): string {
    // Remove part indicators
    let baseName = filename;

    // .part1.rar -> .rar
    baseName = baseName.replace(/\.part\d+\./i, '.');

    // .z01 -> .zip
    baseName = baseName.replace(/\.z\d{2}$/i, '.zip');

    // .001 -> (base name)
    baseName = baseName.replace(/\.\d{3}$/i, '');

    // .r00 -> .rar
    baseName = baseName.replace(/\.r\d{2}$/i, '.rar');

    return baseName;
  }
}
