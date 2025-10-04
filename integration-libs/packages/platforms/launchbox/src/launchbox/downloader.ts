/**
 * LaunchBox Metadata Downloader
 * Downloads and manages LaunchBox Games Database metadata
 */

import * as fs from 'fs';
import * as path from 'path';
import * as https from 'https';
import { createWriteStream } from 'fs';
import { pipeline } from 'stream/promises';
import { homedir } from 'os';

/**
 * LaunchBox metadata source URL
 */
const METADATA_URL = 'https://gamesdb.launchbox-app.com/Metadata.zip';

/**
 * Download metadata configuration
 */
export interface DownloadConfig {
  /**
   * Directory to store metadata
   * Default: ~/.mygamesanywhere/metadata/launchbox
   */
  metadataDir?: string;

  /**
   * Force re-download even if exists
   */
  force?: boolean;

  /**
   * Progress callback (bytes downloaded, total bytes)
   */
  onProgress?: (downloaded: number, total: number) => void;
}

/**
 * LaunchBox metadata downloader
 */
export class LaunchBoxDownloader {
  private metadataDir: string;

  constructor(config?: DownloadConfig) {
    this.metadataDir =
      config?.metadataDir ||
      path.join(homedir(), '.mygamesanywhere', 'metadata', 'launchbox');

    // Ensure directory exists
    fs.mkdirSync(this.metadataDir, { recursive: true });
  }

  /**
   * Get metadata directory path
   */
  getMetadataDir(): string {
    return this.metadataDir;
  }

  /**
   * Get path to Metadata.zip
   */
  getZipPath(): string {
    return path.join(this.metadataDir, 'Metadata.zip');
  }

  /**
   * Get path to extracted metadata directory
   */
  getExtractedDir(): string {
    return path.join(this.metadataDir, 'extracted');
  }

  /**
   * Check if metadata exists
   */
  exists(): boolean {
    return fs.existsSync(this.getZipPath());
  }

  /**
   * Check if metadata is extracted
   */
  isExtracted(): boolean {
    const extractedDir = this.getExtractedDir();
    if (!fs.existsSync(extractedDir)) return false;

    // Check for required XML files
    const requiredFiles = [
      'Metadata.xml',
      'Platforms.xml',
      'Files.xml',
      'Mame.xml',
    ];

    return requiredFiles.every((file) =>
      fs.existsSync(path.join(extractedDir, file))
    );
  }

  /**
   * Get metadata file info
   */
  getMetadataInfo(): { exists: boolean; size?: number; modified?: Date } {
    const zipPath = this.getZipPath();

    if (!fs.existsSync(zipPath)) {
      return { exists: false };
    }

    const stats = fs.statSync(zipPath);
    return {
      exists: true,
      size: stats.size,
      modified: stats.mtime,
    };
  }

  /**
   * Download metadata from LaunchBox
   */
  async download(config?: DownloadConfig): Promise<string> {
    const zipPath = this.getZipPath();

    // Check if already exists and not forcing re-download
    if (!config?.force && this.exists()) {
      console.log('Metadata already exists, skipping download');
      return zipPath;
    }

    console.log(`Downloading LaunchBox metadata from ${METADATA_URL}...`);

    return new Promise((resolve, reject) => {
      https.get(METADATA_URL, (response) => {
        if (response.statusCode === 302 || response.statusCode === 301) {
          // Handle redirect
          const redirectUrl = response.headers.location;
          if (!redirectUrl) {
            reject(new Error('Redirect without location header'));
            return;
          }

          // Follow redirect
          https.get(redirectUrl, (redirectResponse) => {
            this.handleDownloadResponse(
              redirectResponse,
              zipPath,
              config,
              resolve,
              reject
            );
          });
          return;
        }

        this.handleDownloadResponse(response, zipPath, config, resolve, reject);
      });
    });
  }

  /**
   * Handle download response
   */
  private handleDownloadResponse(
    response: any,
    zipPath: string,
    config: DownloadConfig | undefined,
    resolve: (value: string) => void,
    reject: (reason: any) => void
  ): void {
    if (response.statusCode !== 200) {
      reject(
        new Error(`Download failed with status code ${response.statusCode}`)
      );
      return;
    }

    const totalBytes = parseInt(response.headers['content-length'] || '0', 10);
    let downloadedBytes = 0;

    const fileStream = createWriteStream(zipPath);

    // Track progress
    response.on('data', (chunk: Buffer) => {
      downloadedBytes += chunk.length;
      if (config?.onProgress) {
        config.onProgress(downloadedBytes, totalBytes);
      }
    });

    // Pipe to file
    pipeline(response, fileStream)
      .then(() => {
        console.log(`Downloaded ${downloadedBytes} bytes to ${zipPath}`);
        resolve(zipPath);
      })
      .catch((error) => {
        reject(error);
      });
  }

  /**
   * Extract metadata ZIP file
   */
  async extract(): Promise<string> {
    const zipPath = this.getZipPath();
    const extractedDir = this.getExtractedDir();

    if (!this.exists()) {
      throw new Error('Metadata.zip not found. Download first.');
    }

    // Create extraction directory
    fs.mkdirSync(extractedDir, { recursive: true });

    console.log(`Extracting ${zipPath} to ${extractedDir}...`);

    // Use Node.js built-in unzip (via child_process)
    const { execSync } = await import('child_process');

    try {
      // Platform-specific extraction
      if (process.platform === 'win32') {
        // Windows: Use PowerShell Expand-Archive
        execSync(
          `powershell -command "Expand-Archive -Path '${zipPath}' -DestinationPath '${extractedDir}' -Force"`,
          { stdio: 'inherit' }
        );
      } else {
        // Unix: Use unzip
        execSync(`unzip -o "${zipPath}" -d "${extractedDir}"`, {
          stdio: 'inherit',
        });
      }

      console.log('Extraction complete');
      return extractedDir;
    } catch (error) {
      throw new Error(`Failed to extract metadata: ${error}`);
    }
  }

  /**
   * Download and extract metadata
   */
  async downloadAndExtract(config?: DownloadConfig): Promise<string> {
    // Download
    await this.download(config);

    // Extract
    return await this.extract();
  }

  /**
   * Clean up metadata files
   */
  async cleanup(options?: { keepZip?: boolean; keepExtracted?: boolean }): Promise<void> {
    const zipPath = this.getZipPath();
    const extractedDir = this.getExtractedDir();

    if (!options?.keepZip && fs.existsSync(zipPath)) {
      fs.unlinkSync(zipPath);
      console.log('Removed Metadata.zip');
    }

    if (!options?.keepExtracted && fs.existsSync(extractedDir)) {
      fs.rmSync(extractedDir, { recursive: true, force: true });
      console.log('Removed extracted metadata');
    }
  }
}
