package emulation

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
)

type CapabilityFact struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	Details string `json:"details,omitempty"`
}

type Emulator struct {
	ID            string           `json:"id"`
	Name          string           `json:"name"`
	Platforms     []core.Platform  `json:"platforms"`
	AdapterState  string           `json:"adapter_state"`
	SetupHint     string           `json:"setup_hint,omitempty"`
	Capabilities  []CapabilityFact `json:"capabilities"`
	Cores         []CoreSpec       `json:"-"`
	SetupProvider string           `json:"setup_provider,omitempty"`
}

type CoreSpec struct {
	ID                     string
	Name                   string
	Platforms              []core.Platform
	AdapterState           string
	FirmwarePolicy         string
	FirmwareRequirementID  string
	RetroAchievementsState string
}

type Catalog struct {
	emulators  map[string]Emulator
	byPlatform map[core.Platform][]string
}

func NewCatalog(emulators []Emulator) (*Catalog, error) {
	if len(emulators) == 0 {
		return nil, errors.New("emulator catalog must not be empty")
	}
	catalog := &Catalog{emulators: make(map[string]Emulator), byPlatform: make(map[core.Platform][]string)}
	for _, emulator := range emulators {
		emulator.ID = strings.ToLower(strings.TrimSpace(emulator.ID))
		emulator.Name = strings.TrimSpace(emulator.Name)
		if emulator.ID == "" || emulator.Name == "" || len(emulator.Platforms) == 0 {
			return nil, errors.New("emulator id, name, and platforms are required")
		}
		if _, exists := catalog.emulators[emulator.ID]; exists {
			return nil, fmt.Errorf("duplicate emulator %q", emulator.ID)
		}
		seenPlatforms := make(map[core.Platform]bool)
		for _, platform := range emulator.Platforms {
			if strings.TrimSpace(string(platform)) == "" || seenPlatforms[platform] {
				return nil, fmt.Errorf("invalid or duplicate platform %q for emulator %q", platform, emulator.ID)
			}
			seenPlatforms[platform] = true
			catalog.byPlatform[platform] = append(catalog.byPlatform[platform], emulator.ID)
		}
		emulator.Platforms = append([]core.Platform(nil), emulator.Platforms...)
		emulator.Capabilities = append([]CapabilityFact(nil), emulator.Capabilities...)
		seenCores := make(map[string]bool)
		for index := range emulator.Cores {
			entry := &emulator.Cores[index]
			entry.ID = strings.ToLower(strings.TrimSpace(entry.ID))
			entry.Name = strings.TrimSpace(entry.Name)
			if entry.ID == "" || entry.Name == "" || len(entry.Platforms) == 0 || seenCores[entry.ID] {
				return nil, fmt.Errorf("invalid or duplicate core for emulator %q", emulator.ID)
			}
			seenCores[entry.ID] = true
		}
		catalog.emulators[emulator.ID] = emulator
	}
	return catalog, nil
}

func NewDefaultCatalog() *Catalog {
	catalog, err := NewCatalog([]Emulator{
		{
			ID: "retroarch", Name: "RetroArch", AdapterState: "core_required", SetupProvider: "winget",
			Platforms:    []core.Platform{core.PlatformArcade, core.PlatformNES, core.PlatformSNES, core.PlatformGB, core.PlatformGBC, core.PlatformGBA, core.PlatformN64, core.PlatformGenesis, core.PlatformSegaMasterSystem, core.PlatformGameGear, core.PlatformSegaCD, core.PlatformSega32X, core.PlatformPS1, core.PlatformMSDOS},
			SetupHint:    "Add a compatible core with RetroArch's Online Updater.",
			Capabilities: []CapabilityFact{{ID: "retroachievements", State: "depends_on_core", Details: "Support varies by core and game."}, {ID: "save_sync", State: "local_files"}},
			Cores: []CoreSpec{
				{ID: "fbneo", Name: "FinalBurn Neo", Platforms: []core.Platform{core.PlatformArcade}, AdapterState: "ready", FirmwarePolicy: "game_dependent", RetroAchievementsState: "supported"},
				{ID: "fceumm", Name: "FCEUmm", Platforms: []core.Platform{core.PlatformNES}, AdapterState: "ready", FirmwarePolicy: "not_required", RetroAchievementsState: "supported"},
				{ID: "mesen", Name: "Mesen", Platforms: []core.Platform{core.PlatformNES}, AdapterState: "ready", FirmwarePolicy: "not_required", RetroAchievementsState: "supported"},
				{ID: "quicknes", Name: "QuickNES", Platforms: []core.Platform{core.PlatformNES}, AdapterState: "ready", FirmwarePolicy: "not_required", RetroAchievementsState: "supported"},
				{ID: "snes9x", Name: "Snes9x", Platforms: []core.Platform{core.PlatformSNES}, AdapterState: "ready", FirmwarePolicy: "not_required", RetroAchievementsState: "supported"},
				{ID: "mesen_s", Name: "Mesen-S", Platforms: []core.Platform{core.PlatformSNES}, AdapterState: "ready", FirmwarePolicy: "not_required", RetroAchievementsState: "supported"},
				{ID: "gambatte", Name: "Gambatte", Platforms: []core.Platform{core.PlatformGB, core.PlatformGBC}, AdapterState: "ready", FirmwarePolicy: "not_required", RetroAchievementsState: "supported"},
				{ID: "sameboy", Name: "SameBoy", Platforms: []core.Platform{core.PlatformGB, core.PlatformGBC}, AdapterState: "ready", FirmwarePolicy: "not_required", RetroAchievementsState: "supported"},
				{ID: "mgba", Name: "mGBA", Platforms: []core.Platform{core.PlatformGBA}, AdapterState: "ready", FirmwarePolicy: "not_required", RetroAchievementsState: "supported"},
				{ID: "mupen64plus_next", Name: "Mupen64Plus-Next", Platforms: []core.Platform{core.PlatformN64}, AdapterState: "ready", FirmwarePolicy: "not_required", RetroAchievementsState: "supported"},
				{ID: "parallel_n64", Name: "ParaLLEl N64", Platforms: []core.Platform{core.PlatformN64}, AdapterState: "ready", FirmwarePolicy: "not_required", RetroAchievementsState: "supported"},
				{ID: "genesis_plus_gx", Name: "Genesis Plus GX", Platforms: []core.Platform{core.PlatformGenesis, core.PlatformSegaMasterSystem, core.PlatformGameGear, core.PlatformSegaCD}, AdapterState: "ready", FirmwarePolicy: "platform_dependent", FirmwareRequirementID: "segacd_bios", RetroAchievementsState: "supported"},
				{ID: "picodrive", Name: "PicoDrive", Platforms: []core.Platform{core.PlatformGenesis, core.PlatformSegaMasterSystem, core.PlatformGameGear, core.PlatformSegaCD, core.PlatformSega32X}, AdapterState: "ready", FirmwarePolicy: "platform_dependent", FirmwareRequirementID: "segacd_bios", RetroAchievementsState: "supported"},
				{ID: "mednafen_psx_hw", Name: "Beetle PSX HW", Platforms: []core.Platform{core.PlatformPS1}, AdapterState: "ready", FirmwarePolicy: "user_required", FirmwareRequirementID: "ps1_bios", RetroAchievementsState: "supported"},
				{ID: "mednafen_psx", Name: "Beetle PSX", Platforms: []core.Platform{core.PlatformPS1}, AdapterState: "ready", FirmwarePolicy: "user_required", FirmwareRequirementID: "ps1_bios", RetroAchievementsState: "supported"},
				{ID: "swanstation", Name: "SwanStation", Platforms: []core.Platform{core.PlatformPS1}, AdapterState: "ready", FirmwarePolicy: "user_required", FirmwareRequirementID: "ps1_bios", RetroAchievementsState: "supported"},
				{ID: "dosbox_pure", Name: "DOSBox Pure", Platforms: []core.Platform{core.PlatformMSDOS}, AdapterState: "ready", FirmwarePolicy: "not_required", RetroAchievementsState: "unknown"},
			},
		},
		{ID: "scummvm", Name: "ScummVM", AdapterState: "ready", SetupProvider: "winget", Platforms: []core.Platform{core.PlatformScummVM}, Capabilities: []CapabilityFact{{ID: "retroachievements", State: "unsupported"}, {ID: "save_sync", State: "local_files"}}},
		{ID: "dosbox", Name: "DOSBox", AdapterState: "planned", SetupProvider: "winget", Platforms: []core.Platform{core.PlatformMSDOS}, SetupHint: "Typed DOS configuration and launch setup is not ready yet.", Capabilities: []CapabilityFact{{ID: "retroachievements", State: "unsupported"}, {ID: "save_sync", State: "local_files"}}},
		{ID: "duckstation", Name: "DuckStation", AdapterState: "planned", SetupProvider: "winget", Platforms: []core.Platform{core.PlatformPS1}, SetupHint: "Add legally obtained PlayStation firmware in DuckStation.", Capabilities: []CapabilityFact{{ID: "retroachievements", State: "supported_when_configured"}, {ID: "save_sync", State: "local_files"}}},
		{ID: "pcsx2", Name: "PCSX2", AdapterState: "planned", SetupProvider: "winget", Platforms: []core.Platform{core.PlatformPS2}, SetupHint: "Add a legally obtained PlayStation 2 BIOS in PCSX2.", Capabilities: []CapabilityFact{{ID: "retroachievements", State: "supported_when_configured"}, {ID: "save_sync", State: "local_files"}}},
	})
	if err != nil {
		panic(err)
	}
	return catalog
}

func (c *Catalog) CoresFor(platform core.Platform, emulatorID string) []CoreSpec {
	emulator, ok := c.emulators[strings.ToLower(strings.TrimSpace(emulatorID))]
	if !ok {
		return nil
	}
	result := make([]CoreSpec, 0)
	for _, candidate := range emulator.Cores {
		for _, supported := range candidate.Platforms {
			if supported == platform {
				result = append(result, candidate)
				break
			}
		}
	}
	return result
}

func (c *Catalog) SupportsCore(platform core.Platform, emulatorID, coreID string) bool {
	for _, candidate := range c.CoresFor(platform, emulatorID) {
		if candidate.ID == strings.ToLower(strings.TrimSpace(coreID)) {
			return true
		}
	}
	return false
}

func (c *Catalog) Emulator(emulatorID string) (Emulator, bool) {
	emulator, ok := c.emulators[strings.ToLower(strings.TrimSpace(emulatorID))]
	return emulator, ok
}

func (c *Catalog) ForPlatform(platform core.Platform) []Emulator {
	if c == nil {
		return nil
	}
	ids := c.byPlatform[platform]
	result := make([]Emulator, 0, len(ids))
	for _, id := range ids {
		result = append(result, c.emulators[id])
	}
	return result
}

func (c *Catalog) Supports(platform core.Platform, emulatorID string) bool {
	for _, emulator := range c.ForPlatform(platform) {
		if emulator.ID == strings.ToLower(strings.TrimSpace(emulatorID)) {
			return true
		}
	}
	return false
}

func (c *Catalog) Platforms() []core.Platform {
	platforms := make([]core.Platform, 0, len(c.byPlatform))
	for platform := range c.byPlatform {
		platforms = append(platforms, platform)
	}
	sort.Slice(platforms, func(i, j int) bool { return platforms[i] < platforms[j] })
	return platforms
}
