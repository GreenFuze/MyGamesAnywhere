package contentdelivery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/internal/sourcescope"
	"github.com/hirochachacha/go-smb2"
)

type Repository interface {
	GetCopy(ctx context.Context, copyID string) (*Copy, error)
}

type Service struct {
	repository      Repository
	integrationRepo core.IntegrationRepository
	cache           core.SourceCacheService
}

func NewService(repository Repository, integrationRepo core.IntegrationRepository, cache core.SourceCacheService) (*Service, error) {
	if repository == nil {
		return nil, errors.New("content repository is required")
	}
	return &Service{repository: repository, integrationRepo: integrationRepo, cache: cache}, nil
}

func (s *Service) Manifest(ctx context.Context, copyID string) (*Manifest, error) {
	copy, err := s.loadCopy(ctx, copyID)
	if err != nil {
		return nil, err
	}
	delivery, err := s.delivery(ctx, copy.SourceGame)
	if err != nil {
		return nil, err
	}
	return BuildManifest(copy, delivery)
}

type OpenFileResult struct {
	Reader  ReadSeekCloser
	File    ManifestFile
	Name    string
	Size    int64
	ModTime time.Time
}

type ReadSeekCloser interface {
	io.ReadSeeker
	io.Closer
	Stat() (os.FileInfo, error)
}

func (s *Service) OpenFile(ctx context.Context, copyID, fileID string) (*OpenFileResult, error) {
	copy, err := s.loadCopy(ctx, copyID)
	if err != nil {
		return nil, err
	}
	delivery, err := s.delivery(ctx, copy.SourceGame)
	if err != nil {
		return nil, err
	}
	manifest, err := BuildManifest(copy, delivery)
	if err != nil {
		return nil, err
	}

	var manifestFile *ManifestFile
	var sourceFile *core.GameFile
	for i := range manifest.Files {
		if manifest.Files[i].ID != strings.TrimSpace(fileID) {
			continue
		}
		manifestFile = &manifest.Files[i]
		for sourceIndex := range copy.SourceGame.Files {
			relativePath, normalizeErr := NormalizeRelativePath(copy.SourceGame.Files[sourceIndex].Path)
			if normalizeErr == nil && relativePath == manifestFile.RelativePath {
				sourceFile = &copy.SourceGame.Files[sourceIndex]
				break
			}
		}
		break
	}
	if manifestFile == nil || sourceFile == nil {
		return nil, ErrNotFound
	}

	reader, err := s.openSourceFile(ctx, copy.SourceGame, *sourceFile, delivery)
	if err != nil {
		return nil, err
	}
	info, err := reader.Stat()
	if err != nil || info.IsDir() {
		_ = reader.Close()
		return nil, ErrNotFound
	}
	if sourceFile.Size > 0 && info.Size() != sourceFile.Size {
		_ = reader.Close()
		return nil, ErrSourceChanged
	}

	return &OpenFileResult{
		Reader:  reader,
		File:    *manifestFile,
		Name:    filepath.Base(filepath.FromSlash(manifestFile.RelativePath)),
		Size:    info.Size(),
		ModTime: info.ModTime(),
	}, nil
}

func (s *Service) Prepare(ctx context.Context, copyID string) (*core.SourceCacheJobStatus, bool, error) {
	if s.cache == nil {
		return nil, false, ErrUnavailable
	}
	copy, err := s.loadCopy(ctx, copyID)
	if err != nil {
		return nil, false, err
	}
	if _, err := BuildManifest(copy, Delivery{}); err != nil {
		return nil, false, err
	}
	if !s.cache.CanPrepareSourceGame(copy.SourceGame) {
		return nil, false, ErrUnavailable
	}
	job, immediate, err := s.cache.Prepare(ctx, core.SourceCachePrepareRequest{
		CanonicalGameID: copy.CanonicalGameID,
		CanonicalTitle:  copy.SourceGame.RawTitle,
		SourceGameID:    copy.SourceGame.ID,
		Profile:         core.FileDeliverySourceProfile,
	}, copy.SourceGame.Platform, copy.SourceGame)
	if err != nil {
		return nil, false, fmt.Errorf("prepare content: %w", err)
	}
	return job, immediate, nil
}

func (s *Service) GetJob(ctx context.Context, jobID string) (*core.SourceCacheJobStatus, error) {
	if s.cache == nil {
		return nil, ErrUnavailable
	}
	return s.cache.GetJob(ctx, strings.TrimSpace(jobID))
}

func (s *Service) CancelJob(ctx context.Context, jobID string) (*core.SourceCacheJobStatus, bool, error) {
	if s.cache == nil {
		return nil, false, ErrUnavailable
	}
	job, cancelled, err := s.cache.CancelJob(ctx, strings.TrimSpace(jobID))
	if err != nil {
		return nil, false, err
	}
	if job == nil {
		return nil, false, ErrNotFound
	}
	if !cancelled {
		return job, false, ErrJobNotCancellable
	}
	return job, true, nil
}

func (s *Service) loadCopy(ctx context.Context, copyID string) (*Copy, error) {
	if core.ProfileIDFromContext(ctx) == "" || strings.TrimSpace(copyID) == "" {
		return nil, ErrNotFound
	}
	copy, err := s.repository.GetCopy(ctx, strings.TrimSpace(copyID))
	if err != nil {
		return nil, fmt.Errorf("load content copy: %w", err)
	}
	if copy == nil || copy.SourceGame == nil {
		return nil, ErrNotFound
	}
	return copy, nil
}

func (s *Service) delivery(ctx context.Context, sourceGame *core.SourceGame) (Delivery, error) {
	if sourceGame == nil {
		return Delivery{}, ErrNotFound
	}
	if supportsDirectSourceGame(sourceGame) {
		return Delivery{Mode: core.SourceDeliveryModeDirect, Ready: true}, nil
	}
	if s.cache == nil || !s.cache.CanPrepareSourceGame(sourceGame) {
		return Delivery{Mode: core.SourceDeliveryModeUnavailable}, nil
	}
	ready, err := s.cache.IsReady(ctx, sourceGame, core.FileDeliverySourceProfile)
	if err != nil {
		return Delivery{}, fmt.Errorf("inspect materialized content: %w", err)
	}
	return Delivery{
		Mode:                    core.SourceDeliveryModeMaterialized,
		Ready:                   ready,
		MaterializationRequired: !ready,
	}, nil
}

func (s *Service) openSourceFile(ctx context.Context, sourceGame *core.SourceGame, file core.GameFile, delivery Delivery) (ReadSeekCloser, error) {
	switch delivery.Mode {
	case core.SourceDeliveryModeDirect:
		if sourceGame.PluginID == "game-source-smb" {
			return s.openSMBFile(ctx, sourceGame, file)
		}
		// A local connection resolves against its own configured base, not
		// against RootPath: the scanner writes RootPath as a relative group
		// directory that is already a prefix of file.Path, so joining the two
		// would count the group directory twice.
		if sourceGame.PluginID == localSourcePluginID {
			return s.openLocalFile(ctx, sourceGame, file)
		}
		resolved, err := resolveLocalFile(sourceGame.RootPath, file.Path)
		if err != nil {
			return nil, ErrNotFound
		}
		opened, err := os.Open(resolved)
		if err != nil {
			return nil, ErrNotFound
		}
		return opened, nil
	case core.SourceDeliveryModeMaterialized:
		if !delivery.Ready {
			return nil, ErrMaterializationRequired
		}
		_, cachedFile, fullPath, err := s.cache.ResolveCachedFile(ctx, sourceGame.ID, core.FileDeliverySourceProfile, file.Path)
		if err != nil || cachedFile == nil || strings.TrimSpace(fullPath) == "" {
			return nil, ErrMaterializationRequired
		}
		opened, err := os.Open(fullPath)
		if err != nil {
			return nil, ErrMaterializationRequired
		}
		return opened, nil
	default:
		return nil, ErrUnavailable
	}
}

type smbConfig struct {
	Host     string `json:"host"`
	Share    string `json:"share"`
	Username string `json:"username"`
	Password string `json:"password"`
	Path     string `json:"path"`
}

type smbFile struct {
	file   *smb2.File
	share  *smb2.Share
	conn   net.Conn
	logoff func() error
}

func (f *smbFile) Read(p []byte) (int, error)                   { return f.file.Read(p) }
func (f *smbFile) Seek(offset int64, whence int) (int64, error) { return f.file.Seek(offset, whence) }
func (f *smbFile) Stat() (os.FileInfo, error)                   { return f.file.Stat() }
func (f *smbFile) Close() error {
	var first error
	for _, closeFn := range []func() error{
		func() error {
			if f.file != nil {
				return f.file.Close()
			}
			return nil
		},
		func() error {
			if f.share != nil {
				return f.share.Umount()
			}
			return nil
		},
		func() error {
			if f.logoff != nil {
				return f.logoff()
			}
			return nil
		},
		func() error {
			if f.conn != nil {
				return f.conn.Close()
			}
			return nil
		},
	} {
		if err := closeFn(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (s *Service) openSMBFile(ctx context.Context, sourceGame *core.SourceGame, file core.GameFile) (ReadSeekCloser, error) {
	if s.integrationRepo == nil {
		return nil, ErrUnavailable
	}
	integration, err := s.integrationRepo.GetByID(ctx, sourceGame.IntegrationID)
	if err != nil || integration == nil || integration.ID != sourceGame.IntegrationID {
		return nil, ErrNotFound
	}
	var rawConfig map[string]any
	if err := json.Unmarshal([]byte(integration.ConfigJSON), &rawConfig); err != nil {
		return nil, ErrUnavailable
	}
	if err := sourcescope.ValidateConfig("game-source-smb", rawConfig); err != nil {
		return nil, ErrUnavailable
	}
	var config smbConfig
	if err := json.Unmarshal([]byte(integration.ConfigJSON), &config); err != nil {
		return nil, ErrUnavailable
	}
	if strings.TrimSpace(config.Host) == "" || strings.TrimSpace(config.Share) == "" {
		return nil, ErrUnavailable
	}
	sharePath, err := resolveSMBPath(file.Path)
	if err != nil {
		return nil, ErrNotFound
	}
	if !includeScopeAllows(sharePath, sourcescope.ReadIncludePaths("game-source-smb", rawConfig)) {
		return nil, ErrNotFound
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(config.Host, "445"))
	if err != nil {
		return nil, ErrUnavailable
	}
	dialer := &smb2.Dialer{Initiator: &smb2.NTLMInitiator{User: config.Username, Password: config.Password}}
	session, err := dialer.Dial(conn)
	if err != nil {
		_ = conn.Close()
		return nil, ErrUnavailable
	}
	share, err := session.Mount(config.Share)
	if err != nil {
		_ = session.Logoff()
		_ = conn.Close()
		return nil, ErrUnavailable
	}
	opened, err := share.Open(sharePath)
	if err != nil {
		_ = share.Umount()
		_ = session.Logoff()
		_ = conn.Close()
		return nil, ErrNotFound
	}
	return &smbFile{file: opened, share: share, conn: conn, logoff: session.Logoff}, nil
}

const localSourcePluginID = "game-source-local"

type localConfig struct {
	BasePath string `json:"base_path"`
}

// openLocalFile serves a file from a local directory connection.
//
// Four separate gates have to agree before a byte is read, and each one refuses
// rather than guessing: the integration must belong to the requesting profile,
// the connection must declare an absolute base, the file must sit inside the
// configured include scope, and it must still resolve inside the base once every
// symlink on both sides is followed. Errors deliberately carry no filesystem
// path, so a probe learns nothing about the server's layout.
func (s *Service) openLocalFile(ctx context.Context, sourceGame *core.SourceGame, file core.GameFile) (ReadSeekCloser, error) {
	if s.integrationRepo == nil {
		return nil, ErrUnavailable
	}

	// The repository is profile-scoped, so a copy belonging to another profile
	// cannot reach its connection config through this path.
	integration, err := s.integrationRepo.GetByID(ctx, sourceGame.IntegrationID)
	if err != nil || integration == nil || integration.ID != sourceGame.IntegrationID {
		return nil, ErrNotFound
	}

	var rawConfig map[string]any
	if err := json.Unmarshal([]byte(integration.ConfigJSON), &rawConfig); err != nil {
		return nil, ErrUnavailable
	}
	if err := sourcescope.ValidateConfig(localSourcePluginID, rawConfig); err != nil {
		return nil, ErrUnavailable
	}
	var config localConfig
	if err := json.Unmarshal([]byte(integration.ConfigJSON), &config); err != nil {
		return nil, ErrUnavailable
	}
	basePath := strings.TrimSpace(config.BasePath)
	if basePath == "" || !filepath.IsAbs(basePath) {
		return nil, ErrUnavailable
	}

	// Scope first, then containment. Scope answers "was this folder ever
	// connected"; containment answers "does this path still land inside it".
	logicalPath, err := NormalizeRelativePath(file.Path)
	if err != nil {
		return nil, ErrNotFound
	}
	if !includeScopeAllows(logicalPath, sourcescope.ReadIncludePaths(localSourcePluginID, rawConfig)) {
		return nil, ErrNotFound
	}
	resolved, err := resolveLocalFile(basePath, logicalPath)
	if err != nil {
		return nil, ErrNotFound
	}

	opened, err := os.Open(resolved)
	if err != nil {
		return nil, ErrNotFound
	}
	info, err := opened.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = opened.Close()
		return nil, ErrNotFound
	}
	return opened, nil
}

// supportsDirectSourceGame reports whether the server can read this source
// game's bytes itself rather than asking the plugin to materialize a copy.
//
// This is not the same question sourcecache and play_support ask. Those want a
// real file already sitting on this machine that a runtime can open by path, so
// an SMB share does not qualify there. Here it does, because this package knows
// how to open one. Keep the three separate.
func supportsDirectSourceGame(sourceGame *core.SourceGame) bool {
	if sourceGame == nil {
		return false
	}
	switch sourceGame.PluginID {
	case "game-source-smb", localSourcePluginID:
		// Both resolve each file against the connection's own configured root,
		// so the scanner's relative RootPath does not disqualify them.
		return true
	}
	return filepath.IsAbs(strings.TrimSpace(sourceGame.RootPath))
}

func resolveLocalFile(rootPath, relativePath string) (string, error) {
	if !filepath.IsAbs(strings.TrimSpace(rootPath)) {
		return "", errors.New("source root is not absolute")
	}
	normalized, err := NormalizeRelativePath(relativePath)
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(filepath.Clean(rootPath))
	if err != nil {
		return "", err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	candidate, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(normalized)))
	if err != nil {
		return "", err
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("file escapes source root")
	}
	return candidate, nil
}

func resolveSMBPath(relativePath string) (string, error) {
	normalized, err := NormalizeRelativePath(relativePath)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.FromSlash(normalized)), nil
}

// includeScopeAllows reports whether a logical path sits inside the connection's
// configured include scope and outside every exclude under it. Shared by every
// filesystem-backed source; nothing about it is SMB-specific.
func includeScopeAllows(logicalPath string, includes []sourcescope.IncludePath) bool {
	logicalPath = sourcescope.NormalizeLogicalPath(logicalPath)
	if logicalPath == "" {
		return false
	}
	for _, include := range includes {
		if !sourcescope.ScopeContainsRootPath(logicalPath, []sourcescope.IncludePath{include}) {
			continue
		}
		excluded := false
		for _, excludedPath := range include.ExcludePaths {
			excludedPath = sourcescope.NormalizeLogicalPath(excludedPath)
			if excludedPath != "" && (logicalPath == excludedPath || strings.HasPrefix(logicalPath, excludedPath+"/")) {
				excluded = true
				break
			}
		}
		if !excluded {
			return true
		}
	}
	return false
}
