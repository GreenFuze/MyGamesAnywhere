/**
 * Name Extractor
 * Extracts clean game names from various filename formats
 */

import type { ExtractedName } from '../types/index.js';

/**
 * Common platform identifiers in filenames
 */
const PLATFORM_PATTERNS: Record<string, RegExp> = {
  'PlayStation 3': /\.(ps3|psn)/i,
  'Super Nintendo': /\.(snes|sfc)/i,
  'Nintendo Entertainment System': /\.(nes)/i,
  'Game Boy': /\.(gb|gbc)/i,
  'Game Boy Advance': /\.(gba)/i,
  'Nintendo DS': /\.(nds)/i,
  'Nintendo 64': /\.(n64)/i,
  'Sega Genesis': /\.(gen|md|smd)/i,
  'Sega Master System': /\.(sms)/i,
  'Sega Game Gear': /\.(gg)/i,
  'Atari 2600': /\.(a26)/i,
  'DOS': /\.(bat|com)$/i, // Removed .exe - modern installers are Windows
};

/**
 * Region codes (from No-Intro naming convention)
 */
const REGION_PATTERNS: Record<string, RegExp> = {
  USA: /\(U\)|\(USA\)|\(US\)/i,
  Europe: /\(E\)|\(EUR\)|\(Europe\)/i,
  Japan: /\(J\)|\(JPN\)|\(Japan\)/i,
  World: /\(W\)|\(World\)/i,
  Korea: /\(K\)|\(Korea\)/i,
  Brazil: /\(B\)|\(Brazil\)/i,
  China: /\(C\)|\(China\)/i,
};

/**
 * Language codes
 */
const LANGUAGE_PATTERN = /\((?:En|Ja|Fr|De|Es|It|Pt|Ru|Ko|Zh)(?:,(?:En|Ja|Fr|De|Es|It|Pt|Ru|Ko|Zh))*\)/i;

/**
 * Version/revision patterns
 * Matches: v1.0, 1.0, 1.0.5, 1.0.5-gog1, v1.922.0.0, etc.
 */
const VERSION_PATTERN = /(?:^|[_\s.-])(?:v)?(\d+\.\d+(?:\.\d+)*(?:-gog\d*)?)/i;

/**
 * GOG-specific patterns to remove
 */
const GOG_PATTERNS = [
  /_\(\d+\)/g, // _(18156), _(27230)
  /_\(64bit\)|_\(32bit\)/gi, // _(64bit), _(32bit)
  /-gog\d*/gi, // -gog1, -gog, -goggame-123
  /_[a-z]{2}$/i, // Language codes at end: _cs, _en, _fr (ONLY at end!)
];

/**
 * Multi-part file patterns
 */
const MULTIPART_PATTERNS = [
  /-(\d+)\.bin$/i, // setup-1.bin
  /\.part(\d+)/i, // archive.part1
  /\.z(\d+)$/i, // archive.z01
  /\.(\d+)$/i, // archive.001
  /\.r(\d+)$/i, // archive.r00
];

/**
 * Name extractor class
 */
export class NameExtractor {
  /**
   * Extract clean game name from filename
   */
  extract(filename: string): ExtractedName {
    // Remove file extension
    let name = this.removeExtension(filename);

    // Check for multi-part files
    const partInfo = this.extractPartNumber(name);
    if (partInfo) {
      name = partInfo.baseName;
    }

    // Extract platform
    const platform = this.extractPlatform(filename);

    // Remove GOG-specific patterns BEFORE other extraction
    name = this.removeGOGPatterns(name);

    // Extract region (BEFORE cleanName removes parentheses)
    const region = this.extractRegion(name);
    if (region) {
      name = name.replace(region.pattern, '').trim();
    }

    // Extract languages (BEFORE cleanName removes parentheses)
    const languages = this.extractLanguages(name);
    if (languages.length > 0) {
      name = name.replace(LANGUAGE_PATTERN, '').trim();
    }

    // Extract version
    const version = this.extractVersion(name);
    if (version) {
      name = name.replace(VERSION_PATTERN, '').trim();
    }

    // Remove common prefixes (setup_, install_, etc.)
    name = this.removeCommonPrefixes(name);

    // Remove trailing numbers (e.g., "Dark 3 1 0" → "Dark 3")
    name = this.removeTrailingNumbers(name);

    // Clean up the name
    name = this.cleanName(name);

    // Calculate confidence
    const confidence = this.calculateConfidence(name, {
      hasPlatform: !!platform,
      hasRegion: !!region,
      hasVersion: !!version,
      isPart: !!partInfo,
    });

    return {
      cleanName: name,
      platform,
      region: region?.code,
      version,
      languages,
      isPart: !!partInfo,
      partNumber: partInfo?.partNumber,
      confidence,
    };
  }

  /**
   * Remove file extension
   */
  private removeExtension(filename: string): string {
    return filename.replace(/\.[^.]+$/, '');
  }

  /**
   * Extract part number from multi-part files
   */
  private extractPartNumber(name: string): { baseName: string; partNumber: number } | null {
    for (const pattern of MULTIPART_PATTERNS) {
      const match = name.match(pattern);
      if (match) {
        return {
          baseName: name.replace(pattern, ''),
          partNumber: parseInt(match[1]),
        };
      }
    }
    return null;
  }

  /**
   * Extract platform from filename
   */
  private extractPlatform(filename: string): string | undefined {
    for (const [platform, pattern] of Object.entries(PLATFORM_PATTERNS)) {
      if (pattern.test(filename)) {
        return platform;
      }
    }

    // Check for installer extensions (assume Windows)
    if (/\.(exe|msi)$/i.test(filename)) {
      return 'Windows';
    }

    return undefined;
  }

  /**
   * Extract region from name
   */
  private extractRegion(name: string): { code: string; pattern: RegExp } | null {
    for (const [code, pattern] of Object.entries(REGION_PATTERNS)) {
      if (pattern.test(name)) {
        return { code, pattern };
      }
    }
    return null;
  }

  /**
   * Extract languages from name
   */
  private extractLanguages(name: string): string[] {
    const match = name.match(LANGUAGE_PATTERN);
    if (!match) return [];

    const languageString = match[0].replace(/[()]/g, '');
    return languageString.split(',').map((lang) => lang.trim());
  }

  /**
   * Extract version from name
   */
  private extractVersion(name: string): string | undefined {
    const match = name.match(VERSION_PATTERN);
    return match ? match[1] : undefined; // Return captured group (version only)
  }

  /**
   * Remove GOG-specific patterns
   */
  private removeGOGPatterns(name: string): string {
    for (const pattern of GOG_PATTERNS) {
      name = name.replace(pattern, '');
    }
    return name.trim();
  }

  /**
   * Remove trailing numbers that aren't part of the game name
   * E.g., "Alone In The Dark 3 1 0" → "Alone In The Dark 3"
   */
  private removeTrailingNumbers(name: string): string {
    // Remove isolated numbers at the end (separated by spaces/underscores)
    // Keep numbers that are part of the title (e.g., "Dark 3")
    const words = name.split(/[\s_]+/);

    // Count trailing numbers
    let trailingNumberCount = 0;
    for (let i = words.length - 1; i >= 0; i--) {
      if (/^\d{1,2}$/.test(words[i])) {
        trailingNumberCount++;
      } else {
        break;
      }
    }

    // Only remove if we have 2+ trailing numbers (keep the first one)
    // E.g., "Dark 3 1 0" -> remove "1" and "0", keep "3"
    if (trailingNumberCount >= 2) {
      for (let i = 0; i < trailingNumberCount - 1; i++) {
        words.pop();
      }
    }

    return words.join(' ');
  }

  /**
   * Remove common prefixes
   */
  private removeCommonPrefixes(name: string): string {
    const prefixes = [
      /^setup[_-]?/i,
      /^install[_-]?/i,
      /^installer[_-]?/i,
      /^game[_-]?/i,
    ];

    for (const prefix of prefixes) {
      name = name.replace(prefix, '');
    }

    return name;
  }

  /**
   * Clean up name
   */
  private cleanName(name: string): string {
    // Replace underscores, dots, and multiple spaces with single space
    name = name.replace(/[_\.]+/g, ' ');
    name = name.replace(/\s+/g, ' ');

    // Remove extra parentheses and brackets
    name = name.replace(/\s*\([^)]*\)\s*/g, ' ');
    name = name.replace(/\s*\[[^\]]*\]\s*/g, ' ');

    // Trim and capitalize
    name = name.trim();
    name = this.capitalizeTitle(name);

    return name;
  }

  /**
   * Capitalize title properly
   */
  private capitalizeTitle(title: string): string {
    const words = title.split(' ');
    return words
      .map((word) => {
        if (word.length === 0) return word;
        // Keep acronyms uppercase (all caps words)
        if (word === word.toUpperCase() && word.length > 1) {
          return word;
        }
        // Capitalize first letter
        return word.charAt(0).toUpperCase() + word.slice(1).toLowerCase();
      })
      .join(' ');
  }

  /**
   * Calculate confidence score
   */
  private calculateConfidence(
    name: string,
    info: {
      hasPlatform: boolean;
      hasRegion: boolean;
      hasVersion: boolean;
      isPart: boolean;
    }
  ): number {
    let confidence = 0.5; // Base confidence

    // More metadata = higher confidence
    if (info.hasPlatform) confidence += 0.2;
    if (info.hasRegion) confidence += 0.15;
    if (info.hasVersion) confidence += 0.1;

    // Multi-part files are more specific
    if (info.isPart) confidence += 0.05;

    // Longer names are usually more specific
    if (name.length > 10) confidence += 0.05;

    // Cap at 0.95 (never 100% certain from filename alone)
    return Math.min(confidence, 0.95);
  }
}
