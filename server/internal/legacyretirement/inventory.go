package legacyretirement

import (
	"encoding/json"
	"fmt"
	"time"
)

// InventorySnapshot decodes the runtime and prepared-copy evidence persisted in
// device_inventories. The retired device protocol module owned these shapes
// originally; the export owns a private, read-only copy so legacy rows stay
// readable after the protocol package is removed.
//
// Only the fields the retirement plan classifies as owner-recovery evidence are
// decoded. Storage, package-manager, save-adapter, installation, and storefront
// observations are reported through their own dedicated sections or row counts.
type InventorySnapshot struct {
	EndpointID string
	CapturedAt time.Time
	Runtimes   []RuntimeObservation
	Prepared   []PreparedCopyObservation
}

type inventoryRuntimeJSON struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Version            string `json:"version"`
	Path               string `json:"path"`
	CoreProbeState     string `json:"core_probe_state"`
	FirmwareProbeState string `json:"firmware_probe_state"`
}

type inventoryPreparedCopyJSON struct {
	LocalPreparedCopyID string    `json:"local_prepared_copy_id"`
	GameID              string    `json:"game_id"`
	SourceGameID        string    `json:"source_game_id"`
	Title               string    `json:"title"`
	PreparedPath        string    `json:"prepared_path"`
	FileCount           int       `json:"file_count"`
	TotalBytes          uint64    `json:"total_bytes"`
	PreparedAt          time.Time `json:"prepared_at"`
}

// DecodeInventory converts one persisted device_inventories row into export
// evidence. Malformed legacy JSON fails the export instead of silently
// dropping recovery evidence the owner may still need.
func DecodeInventory(endpointID string, capturedAt time.Time, runtimesJSON, preparedCopiesJSON string) (*InventorySnapshot, error) {
	snapshot := &InventorySnapshot{
		EndpointID: endpointID,
		CapturedAt: capturedAt,
		Runtimes:   []RuntimeObservation{},
		Prepared:   []PreparedCopyObservation{},
	}

	// Legacy rows predating a column default may hold an empty string rather
	// than an empty JSON array; that is absence of evidence, not corruption.
	if runtimesJSON != "" {
		var decoded []inventoryRuntimeJSON
		if err := json.Unmarshal([]byte(runtimesJSON), &decoded); err != nil {
			return nil, fmt.Errorf("decode legacy runtime inventory for endpoint %s: %w", endpointID, err)
		}
		for _, runtime := range decoded {
			snapshot.Runtimes = append(snapshot.Runtimes, RuntimeObservation{
				EndpointID:         endpointID,
				RuntimeID:          runtime.ID,
				Name:               runtime.Name,
				Version:            runtime.Version,
				Path:               runtime.Path,
				CoreProbeState:     runtime.CoreProbeState,
				FirmwareProbeState: runtime.FirmwareProbeState,
				CapturedAt:         capturedAt,
			})
		}
	}

	if preparedCopiesJSON != "" {
		var decoded []inventoryPreparedCopyJSON
		if err := json.Unmarshal([]byte(preparedCopiesJSON), &decoded); err != nil {
			return nil, fmt.Errorf("decode legacy prepared copies for endpoint %s: %w", endpointID, err)
		}
		for _, prepared := range decoded {
			snapshot.Prepared = append(snapshot.Prepared, PreparedCopyObservation{
				EndpointID:          endpointID,
				LocalPreparedCopyID: prepared.LocalPreparedCopyID,
				GameID:              prepared.GameID,
				SourceGameID:        prepared.SourceGameID,
				Title:               prepared.Title,
				PreparedPath:        prepared.PreparedPath,
				FileCount:           prepared.FileCount,
				TotalBytes:          prepared.TotalBytes,
				PreparedAt:          prepared.PreparedAt.UTC(),
				CapturedAt:          capturedAt,
			})
		}
	}

	return snapshot, nil
}
