/**
 * LaunchBox XML Parser (Streaming)
 * Parses LaunchBox metadata XML files using streaming to avoid memory issues
 */

import * as fs from 'fs';
import * as path from 'path';
import XmlStream from 'xml-stream';
import type { LaunchBoxDB } from './database.js';

/**
 * LaunchBox streaming XML parser
 */
export class LaunchBoxStreamingParser {
  private db: LaunchBoxDB;

  constructor(db: LaunchBoxDB) {
    this.db = db;
  }

  /**
   * Parse all LaunchBox XML files
   */
  async parseAll(extractedDir: string): Promise<void> {
    console.log('Parsing LaunchBox metadata XML files (streaming)...\n');

    // Parse in order: Platforms first, then Games, then Files
    await this.parsePlatforms(path.join(extractedDir, 'Platforms.xml'));
    await this.parseMetadata(path.join(extractedDir, 'Metadata.xml'));
    await this.parseFiles(path.join(extractedDir, 'Files.xml'));

    // Mame.xml is optional (arcade games)
    const mamePath = path.join(extractedDir, 'Mame.xml');
    if (fs.existsSync(mamePath)) {
      await this.parseMame(mamePath);
    }

    // Update metadata info
    this.db.setMetadataInfo('last_import', new Date().toISOString());
    this.db.setMetadataInfo('source', 'launchbox');

    console.log('\n✅ XML parsing complete');
  }

  /**
   * Parse Platforms.xml (streaming)
   */
  async parsePlatforms(filePath: string): Promise<void> {
    console.log('Parsing Platforms.xml...');

    if (!fs.existsSync(filePath)) {
      throw new Error(`Platforms.xml not found at ${filePath}`);
    }

    return new Promise((resolve, reject) => {
      const stream = fs.createReadStream(filePath);
      const xml = new XmlStream(stream);

      let count = 0;
      const platforms: any[] = [];

      xml.on('endElement: Platform', (platform: any) => {
        platforms.push({
          id: platform.ID || platform.Name,
          name: platform.Name,
          category: platform.Category,
          developer: platform.Developer,
          manufacturer: platform.Manufacturer,
          releaseDate: platform.ReleaseDate,
        });
        count++;
      });

      xml.on('end', () => {
        console.log(`Found ${count} platforms`);

        this.db.beginTransaction();
        try {
          for (const platform of platforms) {
            this.db.upsertPlatform(platform);
          }
          this.db.commitTransaction();
          console.log(`✅ Imported ${count} platforms`);
          resolve();
        } catch (error) {
          this.db.rollbackTransaction();
          reject(error);
        }
      });

      xml.on('error', reject);
      stream.on('error', reject);
    });
  }

  /**
   * Parse Metadata.xml (streaming - critical for large file)
   */
  async parseMetadata(filePath: string): Promise<void> {
    console.log('Parsing Metadata.xml (streaming, this may take a few minutes)...');

    if (!fs.existsSync(filePath)) {
      throw new Error(`Metadata.xml not found at ${filePath}`);
    }

    return new Promise((resolve, reject) => {
      const stream = fs.createReadStream(filePath);
      const xml = new XmlStream(stream);

      let count = 0;
      let batch: any[] = [];
      const BATCH_SIZE = 1000; // Insert in batches for better performance

      // Track progress
      const startTime = Date.now();
      const progressInterval = setInterval(() => {
        const elapsed = Math.round((Date.now() - startTime) / 1000);
        console.log(`  Processed ${count.toLocaleString()} games (${elapsed}s elapsed)...`);
      }, 10000);

      xml.on('endElement: Game', (game: any) => {
        batch.push({
          id: game.ID || game.Name,
          name: game.Name,
          platform: game.Platform,
          developer: game.Developer,
          publisher: game.Publisher,
          releaseDate: game.ReleaseDate,
          overview: game.Overview,
          rating: game.CommunityRating ? parseFloat(game.CommunityRating) : undefined,
          genres: game.Genre
            ? Array.isArray(game.Genre)
              ? game.Genre
              : [game.Genre]
            : [],
        });

        count++;

        // Insert batch when it reaches BATCH_SIZE
        if (batch.length >= BATCH_SIZE) {
          this.insertGameBatch(batch);
          batch = [];
        }
      });

      xml.on('end', () => {
        clearInterval(progressInterval);

        // Insert remaining games
        if (batch.length > 0) {
          this.insertGameBatch(batch);
        }

        console.log(`\n✅ Imported ${count.toLocaleString()} games`);
        resolve();
      });

      xml.on('error', (error: Error) => {
        clearInterval(progressInterval);
        reject(error);
      });

      stream.on('error', (error: Error) => {
        clearInterval(progressInterval);
        reject(error);
      });
    });
  }

  /**
   * Insert a batch of games into database
   */
  private insertGameBatch(games: any[]): void {
    this.db.beginTransaction();

    try {
      for (const game of games) {
        // Insert game
        this.db.upsertGame({
          id: game.id,
          name: game.name,
          platform: game.platform,
          developer: game.developer,
          publisher: game.publisher,
          releaseDate: game.releaseDate,
          overview: game.overview,
          rating: game.rating,
        });

        // Insert genres
        if (game.genres && game.genres.length > 0) {
          for (const genreName of game.genres) {
            const genreId = this.db.insertGenre(genreName);
            this.db.linkGameGenre(game.id, genreId);
          }
        }
      }

      this.db.commitTransaction();
    } catch (error) {
      this.db.rollbackTransaction();
      throw error;
    }
  }

  /**
   * Parse Files.xml (streaming)
   */
  async parseFiles(filePath: string): Promise<void> {
    console.log('Parsing Files.xml...');

    if (!fs.existsSync(filePath)) {
      console.log('⚠️  Files.xml not found, skipping');
      return;
    }

    return new Promise((resolve, reject) => {
      const stream = fs.createReadStream(filePath);
      const xml = new XmlStream(stream);

      let count = 0;
      let batch: any[] = [];
      const BATCH_SIZE = 5000;

      xml.on('endElement: File', (file: any) => {
        batch.push({
          gameId: file.GameID,
          filename: file.FileName,
          crc: file.CRC || null,
          md5: file.MD5 || null,
          sha1: file.SHA1 || null,
        });

        count++;

        if (batch.length >= BATCH_SIZE) {
          this.insertFileBatch(batch);
          batch = [];
        }
      });

      xml.on('end', () => {
        // Insert remaining files
        if (batch.length > 0) {
          this.insertFileBatch(batch);
        }

        console.log(`✅ Imported ${count.toLocaleString()} file entries`);
        resolve();
      });

      xml.on('error', reject);
      stream.on('error', reject);
    });
  }

  /**
   * Insert a batch of files into database
   */
  private insertFileBatch(files: any[]): void {
    this.db.beginTransaction();

    try {
      const stmt = this.db['db'].prepare(`
        INSERT INTO game_files (game_id, filename, crc, md5, sha1)
        VALUES (?, ?, ?, ?, ?)
      `);

      for (const file of files) {
        stmt.run(file.gameId, file.filename, file.crc, file.md5, file.sha1);
      }

      this.db.commitTransaction();
    } catch (error) {
      this.db.rollbackTransaction();
      throw error;
    }
  }

  /**
   * Parse Mame.xml (streaming)
   */
  async parseMame(filePath: string): Promise<void> {
    console.log('Parsing Mame.xml...');

    if (!fs.existsSync(filePath)) {
      console.log('⚠️  Mame.xml not found, skipping');
      return;
    }

    return new Promise((resolve, reject) => {
      const stream = fs.createReadStream(filePath);
      const xml = new XmlStream(stream);

      let count = 0;
      let batch: any[] = [];
      const BATCH_SIZE = 1000;

      xml.on('endElement: Game', (game: any) => {
        batch.push({
          id: game.ID || game.Name,
          name: game.Name,
          platform: 'Arcade',
          developer: game.Developer,
          publisher: game.Publisher,
          releaseDate: game.ReleaseDate,
          overview: game.Overview,
          rating: game.CommunityRating ? parseFloat(game.CommunityRating) : undefined,
        });

        count++;

        if (batch.length >= BATCH_SIZE) {
          this.insertMameBatch(batch);
          batch = [];
        }
      });

      xml.on('end', () => {
        // Insert remaining games
        if (batch.length > 0) {
          this.insertMameBatch(batch);
        }

        console.log(`✅ Imported ${count.toLocaleString()} MAME games`);
        resolve();
      });

      xml.on('error', reject);
      stream.on('error', reject);
    });
  }

  /**
   * Insert a batch of MAME games into database
   */
  private insertMameBatch(games: any[]): void {
    this.db.beginTransaction();

    try {
      for (const game of games) {
        this.db.upsertGame(game);
      }

      this.db.commitTransaction();
    } catch (error) {
      this.db.rollbackTransaction();
      throw error;
    }
  }
}
