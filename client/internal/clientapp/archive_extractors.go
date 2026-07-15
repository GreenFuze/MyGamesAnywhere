package clientapp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/bodgit/sevenzip"
	rardecode "github.com/nwaples/rardecode/v2"
)

const maxManagedArchiveEntries = 100_000

type managedArchiveExtractor interface {
	Validate(string) (uint64, error)
	Extract(context.Context, string, string, uint64, CommandProgressReporter) error
}

func archiveExtractorForFormat(format string) (managedArchiveExtractor, error) {
	switch devicev1.NormalizeArchiveFormat(format) {
	case devicev1.ArchiveFormatZIP:
		return zipManagedArchiveExtractor{}, nil
	case devicev1.ArchiveFormat7Z:
		return sevenZipManagedArchiveExtractor{}, nil
	case devicev1.ArchiveFormatRAR:
		return rarManagedArchiveExtractor{}, nil
	default:
		return nil, fmt.Errorf("unsupported archive format %q", format)
	}
}

type zipManagedArchiveExtractor struct{}

func (zipManagedArchiveExtractor) Validate(path string) (uint64, error) {
	return validateZIPArchive(path)
}

func (zipManagedArchiveExtractor) Extract(ctx context.Context, archivePath, destination string, total uint64, report CommandProgressReporter) error {
	return extractZIP(ctx, archivePath, destination, total, report)
}

type sevenZipManagedArchiveExtractor struct{}

func (sevenZipManagedArchiveExtractor) Validate(path string) (uint64, error) {
	reader, err := sevenzip.OpenReader(path)
	if err != nil {
		return 0, fmt.Errorf("open 7z archive: %w", err)
	}
	defer reader.Close()
	if len(reader.Volumes()) != 1 {
		return 0, errors.New("multi-volume 7z archives are not supported")
	}
	if len(reader.File) == 0 {
		return 0, errors.New("7z archive is empty")
	}
	var total uint64
	count := 0
	for _, entry := range reader.File {
		if _, err := validateManagedArchiveMember(entry.Name, entry.FileInfo().IsDir(), entry.Mode(), entry.UncompressedSize, &count, &total); err != nil {
			return 0, err
		}
	}
	return total, nil
}

func (sevenZipManagedArchiveExtractor) Extract(ctx context.Context, archivePath, destination string, expectedTotal uint64, report CommandProgressReporter) error {
	reader, err := sevenzip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open 7z archive: %w", err)
	}
	defer reader.Close()
	if len(reader.Volumes()) != 1 {
		return errors.New("multi-volume 7z archives are not supported")
	}
	var total, extracted uint64
	count := 0
	if err := reportProgress(report, "extracting", "Extracting files", 46, "install", 10); err != nil {
		return err
	}
	for _, entry := range reader.File {
		if err := ctx.Err(); err != nil {
			return err
		}
		relative, err := validateManagedArchiveMember(entry.Name, entry.FileInfo().IsDir(), entry.Mode(), entry.UncompressedSize, &count, &total)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, filepath.FromSlash(relative))
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		source, err := entry.Open()
		if err != nil {
			return fmt.Errorf("open 7z entry %q: %w", entry.Name, err)
		}
		copyErr := extractManagedArchiveFile(ctx, source, target, entry.UncompressedSize, expectedTotal, &extracted, report)
		closeErr := source.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if total != expectedTotal || extracted != expectedTotal {
		return fmt.Errorf("7z archive size changed during extraction: expected %d bytes, found %d", expectedTotal, total)
	}
	return nil
}

type rarManagedArchiveExtractor struct{}

func (rarManagedArchiveExtractor) Validate(path string) (uint64, error) {
	reader, err := rardecode.OpenReader(path)
	if err != nil {
		return 0, fmt.Errorf("open RAR archive: %w", err)
	}
	defer reader.Close()
	if len(reader.Volumes()) != 1 {
		return 0, errors.New("multi-volume RAR archives are not supported")
	}
	var total uint64
	count := 0
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("read RAR archive: %w", err)
		}
		size, err := validatedRARSize(header)
		if err != nil {
			return 0, err
		}
		if _, err := validateManagedArchiveMember(header.Name, header.IsDir, header.Mode(), size, &count, &total); err != nil {
			return 0, err
		}
	}
	if count == 0 {
		return 0, errors.New("RAR archive is empty")
	}
	return total, nil
}

func (rarManagedArchiveExtractor) Extract(ctx context.Context, archivePath, destination string, expectedTotal uint64, report CommandProgressReporter) error {
	reader, err := rardecode.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open RAR archive: %w", err)
	}
	defer reader.Close()
	if len(reader.Volumes()) != 1 {
		return errors.New("multi-volume RAR archives are not supported")
	}
	var total, extracted uint64
	count := 0
	if err := reportProgress(report, "extracting", "Extracting files", 46, "install", 10); err != nil {
		return err
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read RAR archive: %w", err)
		}
		size, err := validatedRARSize(header)
		if err != nil {
			return err
		}
		relative, err := validateManagedArchiveMember(header.Name, header.IsDir, header.Mode(), size, &count, &total)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, filepath.FromSlash(relative))
		if header.IsDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := extractManagedArchiveFile(ctx, reader, target, size, expectedTotal, &extracted, report); err != nil {
			return err
		}
	}
	if total != expectedTotal || extracted != expectedTotal {
		return fmt.Errorf("RAR archive size changed during extraction: expected %d bytes, found %d", expectedTotal, total)
	}
	return nil
}

func validatedRARSize(header *rardecode.FileHeader) (uint64, error) {
	if header.Encrypted || header.HeaderEncrypted {
		return 0, fmt.Errorf("RAR entry %q is encrypted; password-protected archives are not supported", header.Name)
	}
	if !header.IsDir && header.UnKnownSize {
		return 0, fmt.Errorf("RAR entry %q has an unknown unpacked size", header.Name)
	}
	if header.UnPackedSize < 0 {
		return 0, fmt.Errorf("RAR entry %q has an invalid unpacked size", header.Name)
	}
	return uint64(header.UnPackedSize), nil
}

func validateManagedArchiveMember(name string, isDir bool, mode os.FileMode, size uint64, count *int, total *uint64) (string, error) {
	*count = *count + 1
	if *count > maxManagedArchiveEntries {
		return "", fmt.Errorf("archive contains more than %d entries", maxManagedArchiveEntries)
	}
	relative, err := safeArchivePath(name)
	if err != nil {
		return "", err
	}
	if isDir {
		return relative, nil
	}
	if mode&os.ModeSymlink != 0 {
		return "", fmt.Errorf("archive contains unsupported symbolic link %q", name)
	}
	if !mode.IsRegular() {
		return "", fmt.Errorf("archive contains unsupported non-regular entry %q", name)
	}
	if math.MaxUint64-*total < size {
		return "", errors.New("archive uncompressed size overflow")
	}
	*total += size
	return relative, nil
}

func extractManagedArchiveFile(ctx context.Context, source io.Reader, target string, expectedSize, total uint64, extracted *uint64, report CommandProgressReporter) error {
	if expectedSize > math.MaxInt64-1 {
		return fmt.Errorf("archive entry is too large: %s", target)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	limited := &io.LimitedReader{R: &contextArchiveReader{ctx: ctx, source: source}, N: int64(expectedSize) + 1}
	written, copyErr := io.Copy(output, limited)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if written != int64(expectedSize) {
		return fmt.Errorf("archive entry size mismatch for %s: received %d bytes, expected %d", target, written, expectedSize)
	}
	*extracted += uint64(written)
	percent := uint8(90)
	if total > 0 {
		percent = 10 + uint8(min(uint64(80), *extracted*80/total))
	}
	overall := 40 + uint8(uint16(percent)*60/100)
	return reportProgress(report, "extracting", "Extracting files", overall, "install", percent)
}

type contextArchiveReader struct {
	ctx    context.Context
	source io.Reader
}

func (r *contextArchiveReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(buffer)
}
