/**
 * LaunchBox Game Identifier
 * Identifies games using LaunchBox Games Database
 */

import Fuse from 'fuse.js';
import { LaunchBoxDB, LaunchBoxDownloader, LaunchBoxStreamingParser } from '@mygamesanywhere/platform-launchbox';
import type { LaunchBoxMetadata } from '@mygamesanywhere/platform-launchbox';
import { NameExtractor } from '../parsers/name-extractor.js';
import type {
  GameIdentifier,
  DetectedGame,
  IdentifiedGame,
  GameMetadata,
} from '../types/index.js';

/**
 * LaunchBox identifier configuration
 */
export interface LaunchBoxIdentifierConfig {
  /**
   * Auto-download metadata if not exists
   */
  autoDownload?: boolean;

  /**
   * Minimum confidence for fuzzy matching
   */
  minConfidence?: number;

  /**
   * Maximum results for fuzzy search
   */
  maxResults?: number;
}

/**
 * LaunchBox game identifier
 */
export class LaunchBoxIdentifier implements GameIdentifier {
  private db: LaunchBoxDB;
  private downloader: LaunchBoxDownloader;
  private parser: LaunchBoxStreamingParser;
  private nameExtractor: NameExtractor;
  private config: Required<LaunchBoxIdentifierConfig>;

  constructor(config?: LaunchBoxIdentifierConfig) {
    this.db = new LaunchBoxDB();
    this.downloader = new LaunchBoxDownloader();
    this.parser = new LaunchBoxStreamingParser(this.db);
    this.nameExtractor = new NameExtractor();

    this.config = {
      autoDownload: config?.autoDownload ?? true,
      minConfidence: config?.minConfidence ?? 0.5,
      maxResults: config?.maxResults ?? 10,
    };
  }

  /**
   * Check if metadata is ready
   */
  async isReady(): Promise<boolean> {
    return this.db.isPopulated();
  }

  /**
   * Update metadata database
   */
  async update(): Promise<void> {
    console.log('Updating LaunchBox metadata...\n');

    // Download metadata
    const zipPath = await this.downloader.download({ force: true });
    console.log(`Downloaded to: ${zipPath}\n`);

    // Extract
    const extractedDir = await this.downloader.extract();
    console.log(`Extracted to: ${extractedDir}\n`);

    // Parse and populate database
    await this.parser.parseAll(extractedDir);

    // Show stats
    const stats = this.db.getStats();
    console.log('\nDatabase Statistics:');
    console.log(`  Games: ${stats.games.toLocaleString()}`);
    console.log(`  Platforms: ${stats.platforms.toLocaleString()}`);
    console.log(`  Genres: ${stats.genres.toLocaleString()}`);
    console.log(`  Files: ${stats.files.toLocaleString()}`);

    console.log('\n✅ LaunchBox metadata updated successfully');
  }

  /**
   * Ensure metadata is available
   */
  private async ensureMetadata(): Promise<void> {
    if (await this.isReady()) {
      return;
    }

    if (!this.config.autoDownload) {
      throw new Error(
        'LaunchBox metadata not available. Run update() first or enable autoDownload.'
      );
    }

    console.log('LaunchBox metadata not found. Downloading...\n');
    await this.update();
  }

  /**
   * Identify a detected game
   */
  async identify(detectedGame: DetectedGame): Promise<IdentifiedGame> {
    await this.ensureMetadata();

    // Extract clean name from filename
    const extracted = this.nameExtractor.extract(detectedGame.name);

    console.log(`\nIdentifying: ${detectedGame.name}`);
    console.log(`  → Extracted: "${extracted.cleanName}"`);
    if (extracted.platform) {
      console.log(`  → Platform: ${extracted.platform}`);
    }
    if (extracted.version) {
      console.log(`  → Version: ${extracted.version}`);
    }
    if (extracted.region) {
      console.log(`  → Region: ${extracted.region}`);
    }
    console.log(`  → Searching LaunchBox database...`);

    // Search for matches
    const matches = await this.search(extracted.cleanName, extracted.platform);

    if (matches.length === 0) {
      console.log(`  ❌ No matches found for "${extracted.cleanName}"`);
      return {
        detectedGame,
        matchConfidence: 0,
      };
    }

    // Show top candidates
    if (matches.length > 1) {
      console.log(`  → Found ${matches.length} candidates:`);
      for (let i = 0; i < Math.min(3, matches.length); i++) {
        const match = matches[i];
        console.log(`     ${i + 1}. "${match.title}" (${match.platform || 'Unknown'})`);
      }
    }

    // Use best match
    const bestMatch = matches[0];
    const matchConfidence = this.calculateMatchConfidence(
      extracted.cleanName,
      bestMatch.title,
      extracted.platform,
      bestMatch.platform
    );

    console.log(`  ✅ Best match: ${bestMatch.title} (${bestMatch.platform || 'Unknown'}) - ${Math.round(matchConfidence * 100)}%`);

    return {
      detectedGame,
      metadata: bestMatch,
      matchConfidence,
      source: 'launchbox',
    };
  }

  /**
   * Search for games by name
   */
  async search(query: string, platform?: string): Promise<GameMetadata[]> {
    await this.ensureMetadata();

    // Try exact match first
    const exactMatches = this.db.searchByExactName(query, platform);
    if (exactMatches.length > 0) {
      return exactMatches.map((m) => this.convertToGameMetadata(m));
    }

    // Try fuzzy match
    const fuzzyMatches = this.db.searchByFuzzyName(query, platform, this.config.maxResults);
    if (fuzzyMatches.length > 0) {
      return fuzzyMatches.map((m) => this.convertToGameMetadata(m));
    }

    // Try Fuse.js for more advanced fuzzy matching
    return this.fuzzySearch(query, platform);
  }

  /**
   * Fuzzy search using Fuse.js
   */
  private async fuzzySearch(query: string, platform?: string): Promise<GameMetadata[]> {
    // Get all games for platform (or all if no platform)
    let games: LaunchBoxMetadata[];

    if (platform) {
      // Get platform ID from name
      const platforms = this.db.getAllPlatforms();
      const platformMatch = platforms.find(
        (p: any) => p.name.toLowerCase() === platform.toLowerCase()
      );

      if (platformMatch) {
        games = this.db.searchByFuzzyName('', platformMatch.id, 10000); // Get all games for platform
      } else {
        games = [];
      }
    } else {
      // Get all games (may be slow for large databases)
      games = this.db.searchByFuzzyName('', undefined, 10000);
    }

    // Use Fuse.js for fuzzy matching
    const fuse = new Fuse(games, {
      keys: ['name'],
      threshold: 0.4, // 0 = exact match, 1 = match anything
      includeScore: true,
      minMatchCharLength: 3,
    });

    const results = fuse.search(query, { limit: this.config.maxResults });

    return results
      .filter((r) => r.score !== undefined && r.score <= 0.6) // Only good matches
      .map((r) => this.convertToGameMetadata(r.item));
  }

  /**
   * Convert LaunchBox metadata to GameMetadata
   */
  private convertToGameMetadata(lbMetadata: LaunchBoxMetadata): GameMetadata {
    return {
      id: lbMetadata.id,
      title: lbMetadata.name,
      platform: lbMetadata.platform,
      developer: lbMetadata.developer,
      publisher: lbMetadata.publisher,
      releaseDate: lbMetadata.releaseDate,
      description: lbMetadata.overview,
      genres: lbMetadata.genres,
      rating: lbMetadata.rating,
      source: 'launchbox',
    };
  }

  /**
   * Calculate match confidence
   */
  private calculateMatchConfidence(
    extractedName: string,
    matchedName: string,
    extractedPlatform?: string,
    matchedPlatform?: string
  ): number {
    let confidence = 0.5;

    // Name similarity
    const nameSimilarity = this.stringSimilarity(
      extractedName.toLowerCase(),
      matchedName.toLowerCase()
    );
    confidence += nameSimilarity * 0.4;

    // Platform match
    if (extractedPlatform && matchedPlatform) {
      if (extractedPlatform.toLowerCase() === matchedPlatform.toLowerCase()) {
        confidence += 0.2;
      } else {
        // Partial platform match
        const platformSimilarity = this.stringSimilarity(
          extractedPlatform.toLowerCase(),
          matchedPlatform.toLowerCase()
        );
        confidence += platformSimilarity * 0.1;
      }
    }

    return Math.min(confidence, 1.0);
  }

  /**
   * Calculate string similarity (Levenshtein distance)
   */
  private stringSimilarity(str1: string, str2: string): number {
    const longer = str1.length > str2.length ? str1 : str2;
    const shorter = str1.length > str2.length ? str2 : str1;

    if (longer.length === 0) return 1.0;

    const distance = this.levenshteinDistance(longer, shorter);
    return (longer.length - distance) / longer.length;
  }

  /**
   * Levenshtein distance
   */
  private levenshteinDistance(str1: string, str2: string): number {
    const matrix: number[][] = [];

    for (let i = 0; i <= str2.length; i++) {
      matrix[i] = [i];
    }

    for (let j = 0; j <= str1.length; j++) {
      matrix[0][j] = j;
    }

    for (let i = 1; i <= str2.length; i++) {
      for (let j = 1; j <= str1.length; j++) {
        if (str2.charAt(i - 1) === str1.charAt(j - 1)) {
          matrix[i][j] = matrix[i - 1][j - 1];
        } else {
          matrix[i][j] = Math.min(
            matrix[i - 1][j - 1] + 1,
            matrix[i][j - 1] + 1,
            matrix[i - 1][j] + 1
          );
        }
      }
    }

    return matrix[str2.length][str1.length];
  }

  /**
   * Get database statistics
   */
  getStats() {
    return this.db.getStats();
  }

  /**
   * Close database connection
   */
  close(): void {
    this.db.close();
  }
}
