# Roadmap: Deterministic local game / installer classification and identification

This roadmap breaks down the plan into milestones and tasks. Implementation is in **Go** (structs + validation). Pipeline runs when the server **scan API** is called. Persistence uses **new SQLite tables**. A **FileSystem abstraction** supports local, SMB, Drive, etc. List-games API will support a parameter to include **non-detected** games for later manual review.

**Progress:** Milestones 1–8 complete (inventory scan through list-games with non-detected filter). Next: Phase S "Later" items (evidence traces, manual review queue) and backend design M8 (admin + hardening).

---

## Goals

- [ ] Detect whether local filesystem objects represent:
  - a game
  - a game installer
  - installer payload files
  - emulator ROM / media
  - extracted game directory
  - extras / manuals / media / noise
  - unknown
- [ ] Group files into one package
- [ ] Group base game + addon / DLC / patch / expansion into one game, not separate games
- [ ] Represent installation as an ordered list of installation units
- [ ] Derive normalized title, platform, emulator family, and release candidates
- [ ] Bind to zero or more external game databases (via plugins)
- [ ] Produce one internal canonical game ID, while allowing multiple releases and multiple sources

---

## Core principles

- Prefer deterministic signals over heuristics
- Do not classify a single file directly into a canonical game
- Keep these layers separate:
  - Artifact
  - Package
  - InstallationUnit
  - Release
  - CanonicalGame
  - ExternalBinding
- `unknown` and `ambiguous` are valid outcomes
- No external match does not mean unknown
- Addon/DLC/patch/expansion are installation units attached to a base game
- Path and filesystem structure are useful signals, not required for correctness

**Final rule:** Model installation as one canonical game; one or more releases; each release has an ordered installation sequence; each sequence contains base game first, then addon/DLC/patch/expansion units. This keeps addon/DLC grouped correctly without losing their identity or install order.

---

## High-level model

### Artifact
A concrete filesystem object.

Examples: file, directory, archive file, cue file, bin file, iso file.

Fields: `artifact_id`, `path`, `name`, `extension`, `is_file`, `is_dir`, `size`, `mtime`, `parent_path`, `path_tokens`, `sibling_names`, `scan_state`

### Package
A set of artifacts that belong together operationally.

Examples: `setup_game.exe` + `setup_game-1.bin` + `setup_game-2.bin`; `game.cue` + `track1.bin` + `track2.bin`; one DOS game directory; one PS3 extracted game directory; one MAME ZIP; one portable ZIP archive.

Fields: `package_id`, `root_artifact_id`, `package_kind`, `member_artifact_ids`, `required_member_artifact_ids`, `optional_member_artifact_ids`, `evidence`, `confidence`

### InstallationUnit
A package that can participate in an installation sequence.

Examples: base game installer, DLC installer, season pass installer, expansion installer, patch installer, already-installed portable game directory, extracted console game directory, emulator ROM set.

Fields: `installation_unit_id`, `package_id`, `unit_kind`, `install_role`, `base_game_key_candidate`, `addon_kind`, `install_order_hint`, `depends_on_installation_unit_ids`, `evidence`, `confidence`

`unit_kind` examples: `base_game`, `addon`, `dlc`, `expansion`, `patch`, `portable_game`, `disc_media`, `rom_media`, `extras`, `unknown`

### Release
A specific releasable representation of a game.

Examples: one GOG offline installer release, one PS2 disc image release, one MAME ROM release, one DOS unpacked release.

Fields: `release_id`, `canonical_title_candidate`, `title_variants`, `platform`, `emulator_family`, `release_type`, `region`, `languages`, `version_tokens`, `installation_unit_ids`, `installation_sequence`, `authoritative_ids`, `external_bindings`, `confidence_local`

### CanonicalGame
The abstract game identity.

Fields: `game_id`, `preferred_title`, `title_aliases`, `platform_families`, `release_ids`, `external_bindings`, `confidence_canonical`

### ExternalBinding
Fields: `source`, `external_id`, `external_url`, `bound_level` (release or canonical_game), `match_method`, `evidence`, `confidence`, `last_verified_at`

---

## Milestones

### Milestone 1 — FileSystem abstraction and inventory scan

**Scope:** FileSystem abstraction + Phase A (inventory scan).

- [x] Define `FileSystem` interface (List, Stat, optional path hints: library zone, platform hint, role hint)
- [x] Implement local filesystem adapter for the interface
- [x] Implement inventory scanner: collect path, name, extension, is_file/is_dir, size, mtime, parent_path, path_tokens, sibling names
- [x] Add optional path hints (library zone, platform hint, role hint) when available from adapter
- [x] Add SQLite schema: tables for scan runs and raw inventory (e.g. `scan_runs`, `artifacts`)
- [x] Wire inventory scan into scan API path (no package/classification yet)

**Out of scope:** Artifact kind detection, package assembly.

---

### Milestone 2 — Artifact kind detection by extension

**Scope:** Phase B.

- [x] Define enums and Go structs for artifact kinds (executable-like, archive-like, disc/media-like, data/asset-like, unknown)
- [x] Implement artifact kind detection by extension only (no header parsing)
- [x] Map extensions to kinds: .exe/.com/.bat, .zip/.7z/.rar, .iso/.cue/.bin/.img/.ccd/.sub/.mdf/.mds/.chd, .sfo/.png/.pdf/.mp3, etc.
- [x] Persist or pass artifact kind on each artifact to next stage
- [x] Add/update SQLite schema if persisting artifact kinds
- [x] Refactor/dedup/over-engineering pass (see Code health section)

**Depends on:** Milestone 1.

---

### Milestone 3 — Package assembly

**Scope:** Phase C.

- [x] Define Package struct and package_kind enum (multipart_installer, optical_disc_set, optical_disc_image, portable_game_directory, dos_game_directory, extracted_console_game_directory, archive_candidate, extras_collection, etc.)
- [x] Implement Rule C1: multipart installer (stem.exe + stem-1.bin, stem-2.bin, …)
- [x] Implement Rule C2: cue/bin optical disc package
- [x] Implement Rule C3: single ISO package
- [x] Implement Rule C4: portable native game directory
- [x] Implement Rule C5: DOS game directory
- [x] Implement Rule C6: extracted console game directory (e.g. PS3_GAME, PARAM.SFO)
- [x] Implement Rule C7: archive as package root (archive_candidate)
- [x] Implement Rule C8: MAME-style / ROM-style single archive
- [x] Implement Rule C9: extras collection directory
- [x] Add SQLite tables for packages and package membership (e.g. `packages`, `package_artifacts`)
- [x] Wire package assembly into scan flow after artifact kind detection
- [x] Refactor/dedup/over-engineering pass (see Code health section)

**Depends on:** Milestone 2.

---

### Milestone 4 — Installation units and addon binding

**Scope:** Phases D, E, F.

- [x] Define InstallationUnit struct and enums (unit_kind, install_role: base_game, addon, dlc, expansion, patch, portable_game, disc_media, rom_media, extras, unknown)
- [x] Implement InstallationUnit detection (Rule D1–D4): base-game vs addon vs extras vs unknown
- [x] Implement base-game binding for addons (Phase E): title containment, shared stem/family, sibling proximity, path hints
- [x] Implement installation sequence model (Phase F): ordered list of units per release; base before addons
- [x] Add SQLite tables: `installation_units`, `installation_sequences` (or equivalent)
- [x] Wire units and sequences into scan flow after package assembly
- [x] Refactor/dedup/over-engineering pass (see Code health section)

**Depends on:** Milestone 3.

---

### Milestone 5 — Role, platform, title, and local release ID

**Scope:** Phases G, H, I, J (conditional), K.

- [x] Implement role classifier (Phase G): installer, installer_payload, emulator_rom, disc_media, extracted_console_game, portable_game, extras, noise, unknown
- [x] Implement platform detector (Phase H): windows_pc, ms_dos, arcade, gba, ps1, ps2, ps3, psp, xbox_360, scummvm, unknown (using package structure, title, path hints, layout)
- [x] Implement title normalization (Phase I): region/language/version/disc/addon markers; normalized_title, base_title_candidate, title_variants
- [x] Implement archive member listing (Phase J): list members only for unresolved archives; resolve portable vs installer vs ROM vs extras
- [x] Implement local release descriptor and synthetic release_id (Phase K); DLC units do not create their own canonical game
- [x] Add SQLite tables: `releases`, link installation_units to releases
- [x] Wire into scan flow; persist Release and InstallationUnit

**Depends on:** Milestone 4.

---

### Milestone 6 — Canonical game, relationships, confidence, output schema

**Scope:** Phases N, O, P, Q, R.

- [x] Implement canonical game merge (Phase N): merge releases into one canonical game when evidence agrees; addons/DLC/patch stay attached to base
- [x] Implement relationships graph (Phase O): store edges (contains, requires, payload_of, addon_for, patch_for, expansion_for, manual_for, etc.)
- [x] Implement confidence model (Phase P): local vs external; levels (certain, strong, moderate, weak, unknown)
- [x] Implement evidence precedence (Phase Q): explicit package relations first, then addon/base title, siblings, dir structure, path hints, archive listing, external catalog, general metadata, manual review
- [x] Define and expose output schema (Phase R): release and installation unit fields as specified in plan
- [x] Add SQLite tables: `canonical_games`, `releases.canonical_game_id`, `relationships` (evidence on relationship/entity; no dedicated evidence table in M6)
- [x] Expose API shape for list games consistent with output schema
- [x] Refactor/dedup/over-engineering pass (see Code health section)

**Depends on:** Milestone 5.

---

### Milestone 7 — External binding: authoritative catalog + metadata plugins

**Scope:** Phases L and M. **New plugin type: external DB (metadata).**

- [x] **Contracts:** Define new plugin category/capability (e.g. `metadata` or `external_binding`) in contracts/schemas
- [x] **Contracts:** Define request/response for metadata plugins: input = release/canonical candidate (title, platform, installation units, etc.); output = list of ExternalBindings (source, external_id, external_url, confidence, etc.)
- [x] **Plugin loader:** Support discovering and loading plugins by category (e.g. list “metadata” plugins); extend manifest/schema so plugins can declare metadata capability
- [x] Implement authoritative catalog binding (Phase L): Redump, No-Intro, MAME/software-list where package type supports it (ROM, cue/bin, iso) — MVP: built-in stub returning no bindings; pipeline ready for future DAT/API
- [x] **Scan flow:** After local pipeline produces Release/CanonicalGame, call each metadata plugin with release/canonical candidate; persist returned ExternalBindings to SQLite; do not override strong local evidence
- [x] Add SQLite table(s) for ExternalBinding and wire to releases/canonical_games
- [x] Document in spec/roadmap: external DB is an additional plugin category; server invokes it during scan/binding step

**Depends on:** Milestone 6.

---

### Milestone 8 — List non-detected games and (later) manual review

**Scope:** List-games parameter; evidence traces; optional manual review queue later.

- [x] Add list-games API parameter to include non-detected games (e.g. `include_non_detected=true` or `detection_status=unknown`)
- [x] Ensure non-detected items (unknown install_role or low confidence) are queryable and surfaced for future manual review
- [ ] (Later) Implement explanation/evidence traces in API responses
- [ ] (Later) Implement manual review queue (mark as game/installer/noise, bind to canonical game, override classification)

**Depends on:** Milestone 6 (parameter can be added earlier if needed).

---

## Code health and quality gates

**When:** After Milestones 2, 4, and 6 (and optionally after 3 and 5 if duplication or complexity appears).

- **Refactor:** Extract shared logic (e.g. extension sets, rule loops), keep interfaces small, single responsibility per type. No new behavior; tests must still pass.
- **Deduplication:** Look for repeated patterns (extension maps, similar rule logic across C1–C9 or D1–D4). Prefer one extension→kind map, one rule runner, or small shared helpers instead of copy-paste.
- **Over-engineering check:** Before adding a new abstraction, table, or optional feature: (1) Is it required for the current milestone or an immediate next step? (2) Can we ship the milestone with a simpler design and refactor later? If "not required now" or "yes, we can simplify", do not add it yet.

---

## Phase and milestone naming

**Phases** use the format `Phase [Letter]. [Short name]`: letter A through S (S = implementation-order checklist), short name lowercase and descriptive. Use when referring to the pipeline spec (e.g. "Phase C", "Phase E").

**Milestones** use the format `Milestone [Number] — [Short title]`: number 1 through 8, title concise and implementation-oriented. Use when planning work, checkboxes, and "Depends on" (e.g. "Milestone 2", "Milestone 4").

**Cross-references:** In milestone **Scope**, reference phases (e.g. `**Scope:** Phase A` or `Phases D, E, F`). In task bullets, add `(Phase X)` or `(Phases X, Y)` where it helps.

| Phase(s) | Milestone | Description |
|----------|-----------|-------------|
| A | 1 | FileSystem abstraction and inventory scan |
| B | 2 | Artifact kind detection by extension |
| C | 3 | Package assembly |
| D, E, F | 4 | Installation units and addon binding |
| G, H, I, J, K | 5 | Role, platform, title, local release ID |
| N, O, P, Q, R | 6 | Canonical game, relationships, confidence, output schema |
| L, M | 7 | External binding (authoritative + metadata plugins) |
| — | 8 | List non-detected games and manual review (later) |

Phase S is the recommended implementation-order checklist; it aligns with milestones 1–8 and the Phases (detailed) section.

---

## Phases (detailed)

### Phase A. Inventory scan

Collect only cheap metadata: path, extension, file/dir, size, mtime, parent path, sibling names.

Also collect optional path hints: library zone, platform hint from path, role hint from path.

Important: these are hints only; the rest of the pipeline must still work if these hints are missing or misleading.

---

### Phase B. Artifact kind detection by extension

Do not inspect headers in normal flow. Detect coarse artifact kinds by extension only.

**Executable-like**  
Extensions: `.exe`, `.com`, `.bat`  
Kinds: `windows_executable`, `dos_executable`, `script_launcher`

**Archive-like**  
Extensions: `.zip`, `.7z`, `.rar`  
Kind: `archive`

**Disc / media-like**  
Extensions: `.iso`, `.cue`, `.bin`, `.img`, `.ccd`, `.sub`, `.mdf`, `.mds`, `.chd`  
Kinds: `disc_image`, `disc_descriptor`, `disc_track`

**Data / asset-like**  
Examples: `.sfo`, `.sfb`, `.png`, `.pdf`, `.mp3`, `.ogg`, `.voc`, `.cmf`, `.drv`, `.def`, `.dat`  
Kinds: `game_data`, `manual_or_document`, `image_asset`, `audio_media`

**Unknown**  
Anything else becomes `unknown_artifact`

This phase detects only artifact kind. It does not decide whether something is a game, installer, or addon.

---

### Phase C. Package assembly

Assemble packages using deterministic relationships.

- **Rule C1. Multipart installer package:** If there is `stem.exe` and sibling files `stem-1.bin`, `stem-2.bin`, ... then create one package with `package_kind = multipart_installer`. The `.exe` is the package root; the `.bin` files are required members.
- **Rule C2. Cue/bin optical disc package:** If a `.cue` exists, create one package rooted at the `.cue` and attach referenced or matching sibling `.bin` files. Package kind: `optical_disc_set`.
- **Rule C3. Single ISO package:** If there is one `.iso`, package root is the `.iso`. Package kind: `optical_disc_image`.
- **Rule C4. Portable native game directory:** If a directory contains one or more executable-like files plus asset/data files and the content appears self-contained, classify the directory as one package: `portable_game_directory`.
- **Rule C5. DOS game directory:** If a directory contains `.exe`, `.com`, or `.bat` plus DOS-era data/config assets, classify the entire directory as one package: `dos_game_directory`.
- **Rule C6. Extracted console game directory:** If a directory contains a known extracted-layout signature (e.g. `PS3_GAME`, `PS3_DISC.SFB`, `PARAM.SFO`), classify the whole directory as one package: `extracted_console_game_directory`.
- **Rule C7. Archive package:** A `.zip` or `.7z` starts as a package root: `archive_candidate`. Its role is resolved later by member listing.
- **Rule C8. MAME-style or ROM-style single archive:** If a single archive appears to be the whole game payload and no multipart companion files exist, treat it as a standalone package root.
- **Rule C9. Extras collection:** Directories containing only manuals, images, soundtracks, save files, themes, or unrelated media: `extras_collection`.

---

### Phase D. InstallationUnit detection

Convert packages into installable units.

**Possible install roles:** `base_game`, `addon`, `dlc`, `expansion`, `patch`, `portable_game`, `disc_media`, `rom_media`, `extras`, `unknown`.

- **Rule D1. Base-game indicators:** Normal title; installer without addon markers; portable game directory; disc package; ROM package; extracted console package → `install_role = base_game`.
- **Rule D2. Addon/DLC indicators:** From normalized title: dlc, season pass, character pack, bundle pack, level pack, expansion, bonus content, soundtrack, artbook, update, patch → `install_role = addon` or `dlc` or `patch`; do not create a separate CanonicalGame; store as InstallationUnit attached to the base game candidate.
- **Rule D3. Extras indicators:** Manuals, music, wallpapers, theme files, saves → `install_role = extras`.
- **Rule D4. Unknown:** If not enough deterministic evidence exists → `install_role = unknown`.

---

### Phase E. Base-game binding for addon / DLC / patch

This phase is required so addons do not become separate games.

**Goal:** Bind each addon-like installation unit to one base-game candidate.

**Evidence sources in priority order:**

1. Explicit title containment — e.g. `lego batman 3 beyond gotham season pass` → base candidate `lego batman 3 beyond gotham`
2. Shared package family or shared stem — same vendor/store naming family; same version/build family if meaningful
3. Sibling proximity — addon installer is stored near base installer; addon and base use similar naming patterns
4. Path/library hints — useful, but not required
5. External database reconciliation — if local base-game candidate is ambiguous

**Output:** Each addon-like unit gets `base_game_key_candidate`, `depends_on_installation_unit_ids`, `install_order_hint`.

Important: addon/DLC/patch units remain visible; they are just not promoted to separate canonical games.

---

### Phase F. Installation sequence model

Represent each game release as an ordered list of installation units.

**Why:** This solves base installer + DLC; base installer + patch; multiple expansions; base portable game + extras; disc 1 / disc 2 style ordered media if needed later.

**Model:** Each Release contains `installation_sequence: list[InstallationSequenceItem]`. Each item: `installation_unit_id`, `sequence_index`, `sequence_role`, `is_required`, `depends_on`, `notes`.

**Typical examples:**

- Base game only: 1. base game installer
- Base game + DLC: 1. base game installer, 2. season pass, 3. character pack, 4. level pack
- Base game + patch: 1. base game installer, 2. patch 1, 3. patch 2
- Portable game directory: 1. portable game directory
- Optical disc media: 1. disc package

**Rules:** Base game must appear before addon/DLC/patch; addon/DLC/patch must reference a base unit; sequence order is part of the release model, not the canonical game model.

---

### Phase G. Role classification

Classify each package / installation unit into one operational role.

Possible roles: `portable_game`, `installer`, `installer_payload`, `emulator_rom`, `disc_media`, `extracted_console_game`, `extras`, `noise`, `unknown`.

**Examples:** multipart installer → `installer`; `.bin` payload sibling in installer package → `installer_payload`; MAME ZIP → `emulator_rom`; PS1 cue/bin → `disc_media`; PS2 ISO → `disc_media`; PS3 extracted layout → `extracted_console_game`; DOS self-contained directory → `portable_game`; manual PDF → `extras`; stray MP3 in ROM folder → `noise` or `extras`.

---

### Phase H. Platform detection

Detect platform independently from role.

**Possible evidence sources:** package structure, title conventions, path hints, known extracted layout, later external binding.

**Possible outputs:** `windows_pc`, `ms_dos`, `arcade`, `gba`, `ps1`, `ps2`, `ps3`, `psp`, `xbox_360`, `scummvm`, `unknown`.

Important: path can help a lot; but platform detection must still work when path is absent.

---

### Phase I. Title normalization

Build a normalization pipeline that preserves meaning and removes junk.

**Extract and store separately:** region tags, language tags, version/build tokens, bitness, disc/track markers, edition markers, addon markers, patch markers.

**Normalize:** lowercase; replace `_` and `.` separators with spaces when appropriate; collapse repeated spaces; normalize punctuation; preserve sequel numbers and Roman numerals; preserve important expansion/addon terms separately.

**Produce:** `raw_title_candidate`, `normalized_title`, `base_title_candidate`, `title_variants`, `addon_markers`, `version_tokens`.

Important: if title includes addon markers, derive both `normalized_title` and `base_title_candidate`.

---

### Phase J. Archive member listing

This is the first deeper phase. Use it only for archives whose role is unresolved. Do not fully extract; list member names only.

**Goals:** Resolve: portable game archive, installer archive, ROM archive, extras archive, unknown archive.

**Rules:** If archive contains:
- one top-level game directory with `.exe` + assets → `portable_game_archive`
- installer-like names such as `setup.exe`, `install.exe`, `unins*` → `installer_archive`
- ROM-like payload members → `rom_archive`
- mostly manuals/media → `extras_archive`
- unclear mixed content → `unknown_archive`

---

### Phase K. Local release identification

Create an internal release identity before public database lookup.

**Release descriptor** — Build from: normalized title, base title candidate, platform, release type, installation sequence, addon composition, region, languages, version/build tokens, package membership.

**Internal release ID** — Use: authoritative release ID if available; otherwise synthetic deterministic hash of normalized release descriptor.

Important: a DLC unit does not create its own canonical game; but it may still have its own installation unit identity and release-local identity.

---

### Phase L. External authoritative catalog binding

Use strongest release-oriented sources first when applicable: Redump, No-Intro, MAME / software-list matching. These help at the release/media level.

Use them when the package type supports it: ROM archives, cue/bin, iso, known emulator/media formats.

Absence of a match does not invalidate local classification.

---

### Phase M. External general game database binding

Implemented as **metadata plugin** category. After local release and game candidate are stable, query public game metadata sources: IGDB, TheGamesDB, LaunchBox metadata index, Games Database, optional UI/media databases later.

**Search inputs:** base title candidate, normalized title, platform, year if available, edition markers, addon markers.

**Binding rules:** store multiple bindings; require platform agreement unless platform is unknown; prefer exact or alias title agreement; do not let one fuzzy match override strong local evidence; addon/DLC units may bind at release level or as child content, but still roll into the base game.

---

### Phase N. Canonical game merge

Merge releases into one internal canonical game only when enough evidence agrees.

**Merge-safe cases:** same base title; same platform family; same authoritative identity; same strong external agreement.

**Do not auto-merge:** sequel with previous game; remaster with original; compilation with single title; soundtrack/manual with game; addon/DLC as standalone game.

**Rule:** addon/DLC/patch/expansion are attached to the canonical base game; they are not separate canonical games.

---

### Phase O. Relationships graph

Store explicit edges: `contains`, `requires`, `payload_of`, `disc_of_set`, `addon_for`, `dlc_for`, `patch_for`, `expansion_for`, `manual_for`, `extra_for`, `depends_on`.

**Examples:** installer `.exe` → `requires` `.bin`; `.cue` → `contains` track `.bin`; season pass installer → `addon_for` base game; patch installer → `patch_for` base game; manual PDF → `manual_for` game.

---

### Phase P. Confidence model

Keep separate confidence values.

**Local confidence** — Confidence in: package grouping, installation-unit role, addon/base relationship, platform, title normalization.

**External confidence** — Confidence in: external binding, canonical merge.

Possible values: `certain`, `strong`, `moderate`, `weak`, `unknown`.

---

### Phase Q. Evidence precedence

Use this order of truth:

1. explicit package relationships
2. addon/base title relationship
3. sibling files
4. directory structure
5. path/library hints
6. archive member listing
7. authoritative external catalog match
8. general external metadata
9. manual review

Important: filesystem/path is strong evidence, not mandatory truth.

---

### Phase R. Output schema

Each resolved release should contain: `release_id`, `canonical_game_id`, `preferred_title`, `base_title_candidate`, `platform`, `emulator_family`, `release_type`, `installation_sequence`, `package_ids`, `installation_unit_ids`, `relationships`, `external_bindings`, `confidence_local`, `confidence_external`, `evidence`

Each installation unit should contain: `installation_unit_id`, `package_id`, `install_role`, `base_game_key_candidate`, `addon_kind`, `install_order_hint`, `depends_on_installation_unit_ids`, `evidence`, `confidence`

---

### Phase S. Recommended implementation order (Go)

- [x] Define enums and Go structs (not Pydantic)
- [x] Implement inventory scanner (Phase A)
- [x] Implement artifact kind detection by extension (Phase B)
- [x] Implement package assembly rules (Phase C)
- [x] Implement installation-unit detection (Phase D)
- [x] Implement addon/DLC/patch detection and base-game binding (Phases D, E)
- [x] Implement installation sequence model (Phase F)
- [x] Implement role classifier (Phase G)
- [x] Implement platform detector (Phase H)
- [x] Implement title normalization (Phase I)
- [x] Implement relationships graph (Phase O)
- [x] Implement archive member listing (Phase J)
- [x] Implement local release descriptor and synthetic release ID (Phase K)
- [x] Implement authoritative catalog binding (Phase L)
- [x] Implement general external DB binding as metadata plugins (Phase M)
- [x] Implement canonical merge layer (Phase N)
- [x] Implement confidence and evidence precedence (Phases P, Q)
- [ ] Implement explanation/evidence traces
- [ ] Implement manual review queue

---

## Non-goals (first version)

- No file-header parsing in normal flow
- No full hashing of huge remote files in first pass
- No deep archive extraction unless role cannot be resolved otherwise
- No automatic trust in one public metadata source
- No promotion of addon/DLC into standalone canonical games
