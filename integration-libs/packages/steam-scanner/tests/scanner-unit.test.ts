/**
 * Unit tests for Steam Scanner
 */

import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { SteamScanner } from '../src/scanner.js';
import {
  SteamNotFoundError,
  FileAccessError,
  VDFParseError,
} from '../src/errors.js';
import * as fs from 'fs/promises';
import { platform } from 'os';

// Mock fs module
vi.mock('fs/promises');
vi.mock('os');

describe('SteamScanner - Unit Tests', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe('detectSteamPath', () => {
    it('should detect Steam on Windows', async () => {
      vi.mocked(platform).mockReturnValue('win32');
      vi.mocked(fs.stat).mockResolvedValue({
        isDirectory: () => true,
      } as any);

      const scanner = new SteamScanner();
      const path = await scanner.detectSteamPath();

      expect(path).toContain('Steam');
      expect(vi.mocked(fs.stat)).toHaveBeenCalled();
    });

    it('should detect Steam on macOS', async () => {
      vi.mocked(platform).mockReturnValue('darwin');
      vi.mocked(fs.stat).mockResolvedValue({
        isDirectory: () => true,
      } as any);

      const scanner = new SteamScanner();
      const path = await scanner.detectSteamPath();

      expect(path).toContain('Library');
      expect(path).toContain('Application Support');
      expect(path).toContain('Steam');
    });

    it('should detect Steam on Linux', async () => {
      vi.mocked(platform).mockReturnValue('linux');
      vi.mocked(fs.stat).mockResolvedValue({
        isDirectory: () => true,
      } as any);

      const scanner = new SteamScanner();
      const path = await scanner.detectSteamPath();

      expect(path).toMatch(/\.steam|\.local\/share\/Steam/);
    });

    it('should throw SteamNotFoundError when Steam not found', async () => {
      vi.mocked(platform).mockReturnValue('win32');
      vi.mocked(fs.stat).mockRejectedValue(new Error('ENOENT'));

      const scanner = new SteamScanner();

      await expect(scanner.detectSteamPath()).rejects.toThrow(
        SteamNotFoundError
      );
    });

    it('should throw SteamNotFoundError on unsupported platform', async () => {
      vi.mocked(platform).mockReturnValue('freebsd');

      const scanner = new SteamScanner();

      await expect(scanner.detectSteamPath()).rejects.toThrow(
        SteamNotFoundError
      );
      await expect(scanner.detectSteamPath()).rejects.toThrow(
        /Unsupported platform/
      );
    });

    it('should try multiple paths on Windows', async () => {
      vi.mocked(platform).mockReturnValue('win32');
      let callCount = 0;

      vi.mocked(fs.stat).mockImplementation(async () => {
        callCount++;
        if (callCount === 1) {
          throw new Error('ENOENT'); // First path fails
        }
        return {
          isDirectory: () => true,
        } as any;
      });

      const scanner = new SteamScanner();
      const path = await scanner.detectSteamPath();

      expect(path).toBeTruthy();
      expect(callCount).toBeGreaterThan(1);
    });
  });

  describe('initialize', () => {
    it('should initialize with auto-detected path', async () => {
      vi.mocked(platform).mockReturnValue('win32');
      vi.mocked(fs.stat).mockResolvedValue({
        isDirectory: () => true,
      } as any);
      vi.mocked(fs.access).mockResolvedValue(undefined);

      const scanner = new SteamScanner();
      await expect(scanner.initialize()).resolves.not.toThrow();
    });

    it('should initialize with custom Steam path', async () => {
      vi.mocked(fs.access).mockResolvedValue(undefined);

      const scanner = new SteamScanner();
      await expect(
        scanner.initialize({ steamPath: 'C:\\Custom\\Steam' })
      ).resolves.not.toThrow();
    });

    it('should throw FileAccessError if Steam path not accessible', async () => {
      vi.mocked(platform).mockReturnValue('win32');
      vi.mocked(fs.stat).mockResolvedValue({
        isDirectory: () => true,
      } as any);
      vi.mocked(fs.access).mockRejectedValue(new Error('EACCES'));

      const scanner = new SteamScanner();

      await expect(scanner.initialize()).rejects.toThrow(FileAccessError);
    });

    it('should throw FileAccessError if libraryfolders.vdf not found', async () => {
      vi.mocked(platform).mockReturnValue('win32');
      vi.mocked(fs.stat).mockResolvedValue({
        isDirectory: () => true,
      } as any);

      let accessCallCount = 0;
      vi.mocked(fs.access).mockImplementation(async () => {
        accessCallCount++;
        if (accessCallCount === 2) {
          // Second call for libraryfolders.vdf
          throw new Error('ENOENT');
        }
      });

      const scanner = new SteamScanner();

      await expect(scanner.initialize()).rejects.toThrow(FileAccessError);
    });
  });

  describe('scan', () => {
    const mockLibraryFoldersVDF = `
"libraryfolders"
{
  "0"
  {
    "path"  "C:\\\\Program Files (x86)\\\\Steam"
    "label"  ""
    "contentid"  "123"
    "totalsize"  "500000000000"
  }
}
    `;

    const mockAppManifest = `
"AppState"
{
  "appid"  "440"
  "name"  "Team Fortress 2"
  "installdir"  "Team Fortress 2"
  "LastUpdated"  "1638360000"
  "SizeOnDisk"  "26843545600"
  "buildid"  "8654321"
}
    `;

    beforeEach(() => {
      vi.mocked(platform).mockReturnValue('win32');
      vi.mocked(fs.stat).mockResolvedValue({
        isDirectory: () => true,
      } as any);
      vi.mocked(fs.access).mockResolvedValue(undefined);
    });

    it('should scan and return game list', async () => {
      vi.mocked(fs.readFile)
        .mockResolvedValueOnce(mockLibraryFoldersVDF) // libraryfolders.vdf
        .mockResolvedValueOnce(mockAppManifest); // appmanifest

      vi.mocked(fs.readdir).mockResolvedValue([
        'appmanifest_440.acf',
      ] as any);

      const scanner = new SteamScanner();
      const result = await scanner.scan();

      expect(result.games).toHaveLength(1);
      expect(result.games[0].appid).toBe('440');
      expect(result.games[0].name).toBe('Team Fortress 2');
      expect(result.libraryFolders).toHaveLength(1);
      expect(result.scanDuration).toBeGreaterThan(0);
    });

    it('should handle multiple library folders', async () => {
      const multiLibraryVDF = `
"libraryfolders"
{
  "0"
  {
    "path"  "C:\\\\Steam"
    "label"  ""
    "contentid"  "123"
    "totalsize"  "500000000000"
  }
  "1"
  {
    "path"  "D:\\\\Games"
    "label"  "Games"
    "contentid"  "456"
    "totalsize"  "1000000000000"
  }
}
      `;

      vi.mocked(fs.readFile)
        .mockResolvedValueOnce(multiLibraryVDF)
        .mockResolvedValue(mockAppManifest);

      vi.mocked(fs.readdir).mockResolvedValue([
        'appmanifest_440.acf',
      ] as any);

      const scanner = new SteamScanner();
      const result = await scanner.scan();

      expect(result.libraryFolders).toHaveLength(2);
      expect(result.games.length).toBeGreaterThan(0);
    });

    it('should handle empty library folder', async () => {
      vi.mocked(fs.readFile).mockResolvedValueOnce(mockLibraryFoldersVDF);
      vi.mocked(fs.readdir).mockResolvedValue([] as any); // No manifest files

      const scanner = new SteamScanner();
      const result = await scanner.scan();

      expect(result.games).toHaveLength(0);
      expect(result.libraryFolders).toHaveLength(1);
    });

    it('should skip corrupted manifest files', async () => {
      const corruptedManifest = 'corrupted content';

      vi.mocked(fs.readFile)
        .mockResolvedValueOnce(mockLibraryFoldersVDF)
        .mockResolvedValueOnce(corruptedManifest) // Corrupted
        .mockResolvedValueOnce(mockAppManifest); // Valid

      vi.mocked(fs.readdir).mockResolvedValue([
        'appmanifest_999.acf',
        'appmanifest_440.acf',
      ] as any);

      const consoleSpy = vi
        .spyOn(console, 'warn')
        .mockImplementation(() => {});

      const scanner = new SteamScanner();
      const result = await scanner.scan();

      expect(result.games).toHaveLength(1);
      expect(result.games[0].appid).toBe('440');
      expect(consoleSpy).toHaveBeenCalled();

      consoleSpy.mockRestore();
    });

    it('should handle library folder access error gracefully', async () => {
      vi.mocked(fs.readFile).mockResolvedValueOnce(mockLibraryFoldersVDF);
      vi.mocked(fs.readdir).mockRejectedValue(new Error('EACCES'));

      const consoleSpy = vi
        .spyOn(console, 'warn')
        .mockImplementation(() => {});

      const scanner = new SteamScanner();
      const result = await scanner.scan();

      expect(result.games).toHaveLength(0);
      expect(consoleSpy).toHaveBeenCalled();

      consoleSpy.mockRestore();
    });

    it('should auto-initialize if not initialized', async () => {
      vi.mocked(fs.readFile).mockResolvedValueOnce(mockLibraryFoldersVDF);
      vi.mocked(fs.readdir).mockResolvedValue([] as any);

      const scanner = new SteamScanner();
      // Don't call initialize() - should auto-initialize
      const result = await scanner.scan();

      expect(result).toBeDefined();
      expect(result.steamPath).toBeTruthy();
    });

    it('should include scan duration', async () => {
      vi.mocked(fs.readFile).mockResolvedValueOnce(mockLibraryFoldersVDF);
      vi.mocked(fs.readdir).mockResolvedValue([] as any);

      const scanner = new SteamScanner();
      const result = await scanner.scan();

      expect(result.scanDuration).toBeGreaterThanOrEqual(0);
      expect(typeof result.scanDuration).toBe('number');
    });
  });

  describe('Edge Cases', () => {
    it('should handle manifest without optional fields', async () => {
      const minimalManifest = `
"AppState"
{
  "appid"  "440"
  "name"  "Test Game"
  "installdir"  "testgame"
  "LastUpdated"  "0"
  "SizeOnDisk"  "0"
}
      `;

      vi.mocked(platform).mockReturnValue('win32');
      vi.mocked(fs.stat).mockResolvedValue({
        isDirectory: () => true,
      } as any);
      vi.mocked(fs.access).mockResolvedValue(undefined);
      vi.mocked(fs.readFile)
        .mockResolvedValueOnce(
          '"libraryfolders"\n{\n  "0"\n  {\n    "path"  "C:\\\\Steam"\n    "label"  ""\n    "contentid"  "123"\n    "totalsize"  "0"\n  }\n}'
        )
        .mockResolvedValueOnce(minimalManifest);
      vi.mocked(fs.readdir).mockResolvedValue([
        'appmanifest_440.acf',
      ] as any);

      const scanner = new SteamScanner();
      const result = await scanner.scan();

      expect(result.games).toHaveLength(1);
      expect(result.games[0].buildId).toBeUndefined();
      expect(result.games[0].lastOwner).toBeUndefined();
    });

    it('should handle library folder with no apps field', async () => {
      const noAppsVDF = `
"libraryfolders"
{
  "0"
  {
    "path"  "C:\\\\Steam"
    "label"  ""
    "contentid"  "123"
    "totalsize"  "500000000000"
  }
}
      `;

      vi.mocked(platform).mockReturnValue('win32');
      vi.mocked(fs.stat).mockResolvedValue({
        isDirectory: () => true,
      } as any);
      vi.mocked(fs.access).mockResolvedValue(undefined);
      vi.mocked(fs.readFile).mockResolvedValueOnce(noAppsVDF);
      vi.mocked(fs.readdir).mockResolvedValue([] as any);

      const scanner = new SteamScanner();
      const result = await scanner.scan();

      expect(result.libraryFolders).toHaveLength(1);
      expect(result.games).toHaveLength(0);
    });
  });
});
