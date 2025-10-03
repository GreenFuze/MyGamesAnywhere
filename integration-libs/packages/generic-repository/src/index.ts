/**
 * Generic Game Repository Scanner
 * Main entry point
 */

// Types
export * from './types.js';

// Storage adapters
export { BaseRepositoryAdapter } from './storage/repository-adapter.js';
export { LocalRepository } from './storage/local-repository.js';

// Scanner components
export { RepositoryScanner } from './scanner/repository-scanner.js';
export { RecursiveWalker } from './scanner/recursive-walker.js';
export { FileClassifier } from './scanner/file-classifier.js';
export { ArchiveDetector } from './scanner/archive-detector.js';

/**
 * Quick-start helper: Scan a local directory
 */
export async function scanLocalDirectory(
  path: string,
  config?: import('./types.js').ScannerConfig
) {
  const { LocalRepository } = await import('./storage/local-repository.js');
  const { RepositoryScanner } = await import(
    './scanner/repository-scanner.js'
  );

  const repository = new LocalRepository(path);
  const scanner = new RepositoryScanner(repository, config);

  return await scanner.scan();
}
