package clientapp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	devicev1 "github.com/GreenFuze/MyGamesAnywhere/protocol/device/v1"
	"github.com/google/uuid"
)

const preparedCopyCatalogSchemaVersion = 1

type PreparedCopyRecord struct {
	LocalPreparedCopyID string    `json:"local_prepared_copy_id"`
	BindingID           string    `json:"binding_id"`
	GameID              string    `json:"game_id"`
	SourceGameID        string    `json:"source_game_id"`
	Title               string    `json:"title"`
	PreparedRoot        string    `json:"prepared_root"`
	PreparedPath        string    `json:"prepared_path"`
	FileCount           int       `json:"file_count"`
	TotalBytes          uint64    `json:"total_bytes"`
	PreparedAt          time.Time `json:"prepared_at"`
}

func (r PreparedCopyRecord) validate() error {
	if _, err := uuid.Parse(r.LocalPreparedCopyID); err != nil {
		return errors.New("local_prepared_copy_id must be a UUID")
	}
	if _, err := uuid.Parse(r.BindingID); err != nil {
		return errors.New("binding_id must be a UUID")
	}
	observation := devicev1.PreparedCopyObservation{
		LocalPreparedCopyID: r.LocalPreparedCopyID, GameID: r.GameID, SourceGameID: r.SourceGameID,
		Title: r.Title, PreparedPath: r.PreparedPath, FileCount: r.FileCount,
		TotalBytes: r.TotalBytes, PreparedAt: r.PreparedAt,
	}
	if err := observation.Validate(); err != nil {
		return err
	}
	if !filepath.IsAbs(r.PreparedRoot) {
		return errors.New("prepared_root must be absolute")
	}
	inside, err := pathWithinRoot(r.PreparedRoot, r.PreparedPath)
	if err != nil || !inside || sameLocalPath(r.PreparedRoot, r.PreparedPath) {
		return errors.New("prepared_path must be a child of prepared_root")
	}
	return nil
}

type preparedCopyCatalogDocument struct {
	SchemaVersion int                  `json:"schema_version"`
	Copies        []PreparedCopyRecord `json:"copies"`
}

func (d preparedCopyCatalogDocument) validate() error {
	if d.SchemaVersion != preparedCopyCatalogSchemaVersion {
		return fmt.Errorf("unsupported prepared copy catalog schema %d", d.SchemaVersion)
	}
	ids := map[string]bool{}
	paths := map[string]bool{}
	for index, record := range d.Copies {
		if err := record.validate(); err != nil {
			return fmt.Errorf("prepared copy %d: %w", index, err)
		}
		id := strings.ToLower(record.LocalPreparedCopyID)
		path := localPathKey(record.PreparedPath)
		if ids[id] || paths[path] {
			return errors.New("prepared copy catalog contains duplicate ID or path")
		}
		ids[id], paths[path] = true, true
	}
	return nil
}

// PreparedCopyCatalog is the per-OS-user persisted source of truth for files
// downloaded but not installed. Records remain binding-scoped.
type PreparedCopyCatalog struct {
	mu   sync.Mutex
	path string
	doc  preparedCopyCatalogDocument
}

func OpenPreparedCopyCatalog(path string) (*PreparedCopyCatalog, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("prepared copy catalog path is required")
	}
	catalog := &PreparedCopyCatalog{
		path: path,
		doc:  preparedCopyCatalogDocument{SchemaVersion: preparedCopyCatalogSchemaVersion, Copies: []PreparedCopyRecord{}},
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return catalog, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read prepared copy catalog: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog.doc); err != nil {
		return nil, fmt.Errorf("decode prepared copy catalog: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("prepared copy catalog contains trailing JSON")
	}
	if err := catalog.doc.validate(); err != nil {
		return nil, err
	}
	return catalog, nil
}

func (c *PreparedCopyCatalog) ListForBinding(bindingID string) []PreparedCopyRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]PreparedCopyRecord, 0)
	for _, record := range c.doc.Copies {
		if strings.EqualFold(record.BindingID, bindingID) {
			result = append(result, record)
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left].PreparedAt.After(result[right].PreparedAt) })
	return result
}

func (c *PreparedCopyCatalog) Add(record PreparedCopyRecord) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := record.validate(); err != nil {
		return err
	}
	for _, existing := range c.doc.Copies {
		if sameLocalPath(existing.PreparedPath, record.PreparedPath) {
			return fmt.Errorf("prepared copy destination is already recorded: %s", record.PreparedPath)
		}
	}
	previousLength := len(c.doc.Copies)
	c.doc.Copies = append(c.doc.Copies, record)
	if err := c.saveLocked(); err != nil {
		c.doc.Copies = c.doc.Copies[:previousLength]
		return err
	}
	return nil
}

func (c *PreparedCopyCatalog) saveLocked() error {
	if err := c.doc.validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c.doc, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o700); err != nil {
		return err
	}
	temp := c.path + ".tmp"
	if err := os.Remove(temp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.WriteFile(temp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(temp, c.path); err != nil {
		_ = os.Remove(temp)
		return err
	}
	return nil
}
