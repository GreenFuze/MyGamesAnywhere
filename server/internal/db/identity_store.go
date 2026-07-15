package db

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/GreenFuze/MyGamesAnywhere/server/internal/core"
	"github.com/GreenFuze/MyGamesAnywhere/server/pkg/titlematch"
	"github.com/google/uuid"
)

type identityReconciler struct {
	logger core.Logger
}

type editionIdentitySeed struct {
	id              string
	profileID       string
	createdAt       int64
	providerTitles  map[string]int
	fallbackTitles  map[string]int
	platforms       map[string]bool
	kinds           map[string]bool
	evidence        map[string]core.IdentityEvidence
	manual          bool
	existingTitle   string
	existingRegion  string
	existingLabel   string
	conflict        bool
	profileConflict bool
}

type persistedTitleIdentity struct {
	id        string
	createdAt int64
}

type identityUnionFind struct {
	parent map[string]string
}

func newIdentityUnionFind(ids []string) *identityUnionFind {
	parent := make(map[string]string, len(ids))
	for _, id := range ids {
		parent[id] = id
	}
	return &identityUnionFind{parent: parent}
}

func (u *identityUnionFind) find(id string) string {
	parent, ok := u.parent[id]
	if !ok {
		return ""
	}
	if parent != id {
		u.parent[id] = u.find(parent)
	}
	return u.parent[id]
}

func (u *identityUnionFind) union(left, right string) {
	leftRoot, rightRoot := u.find(left), u.find(right)
	if leftRoot == "" || rightRoot == "" || leftRoot == rightRoot {
		return
	}
	if leftRoot < rightRoot {
		u.parent[rightRoot] = leftRoot
	} else {
		u.parent[leftRoot] = rightRoot
	}
}

func (s *sqliteDatabase) rebuildGameIdentity(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin game identity rebuild: %w", err)
	}
	defer tx.Rollback()
	if err := (&identityReconciler{logger: s.logger}).reconcile(ctx, tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit game identity rebuild: %w", err)
	}
	return nil
}

func (r *identityReconciler) reconcile(ctx context.Context, tx *sql.Tx) error {
	seeds, existingTitles, err := r.loadSeeds(ctx, tx)
	if err != nil {
		return err
	}
	validProfiles, err := loadIdentityProfileIDs(ctx, tx)
	if err != nil {
		return err
	}
	byProfile := make(map[string][]*editionIdentitySeed)
	skipped := 0
	for _, seed := range seeds {
		if seed.profileConflict || strings.TrimSpace(seed.profileID) == "" || !validProfiles[seed.profileID] {
			skipped++
			if _, err := tx.ExecContext(ctx, `DELETE FROM game_editions WHERE id=?`, seed.id); err != nil {
				return fmt.Errorf("remove invalid legacy game identity %s: %w", seed.id, err)
			}
			continue
		}
		byProfile[seed.profileID] = append(byProfile[seed.profileID], seed)
	}

	profileIDs := make([]string, 0, len(byProfile))
	for profileID := range byProfile {
		profileIDs = append(profileIDs, profileID)
	}
	sort.Strings(profileIDs)
	for _, profileID := range profileIDs {
		if err := r.reconcileProfile(ctx, tx, profileID, byProfile[profileID], existingTitles); err != nil {
			return err
		}
	}
	if skipped > 0 && len(validProfiles) > 0 {
		r.logger.Warn("skipped legacy game identities without one valid owning profile", "editions", skipped)
	}
	r.logger.Info("reconciled version-aware game identity", "profiles", len(profileIDs), "editions", len(seeds)-skipped)
	return nil
}

func loadIdentityProfileIDs(ctx context.Context, tx *sql.Tx) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id FROM profiles`)
	if err != nil {
		return nil, fmt.Errorf("load game identity profiles: %w", err)
	}
	defer rows.Close()
	profiles := map[string]bool{}
	for rows.Next() {
		var profileID string
		if err := rows.Scan(&profileID); err != nil {
			return nil, fmt.Errorf("scan game identity profile: %w", err)
		}
		profiles[profileID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate game identity profiles: %w", err)
	}
	return profiles, nil
}

func (r *identityReconciler) loadSeeds(ctx context.Context, tx *sql.Tx) (map[string]*editionIdentitySeed, map[string]persistedTitleIdentity, error) {
	rows, err := tx.QueryContext(ctx, `SELECT l.canonical_id, cg.created_at, COALESCE(sg.profile_id, ''), sg.raw_title,
		COALESCE(sg.platform, 'unknown'), COALESCE(sg.kind, 'unknown'),
		COALESCE(m.plugin_id, ''), COALESCE(m.external_id, ''), COALESCE(m.title, ''),
		COALESCE(m.platform, ''), COALESCE(m.metadata_json, ''), COALESCE(p.mode, '')
		FROM canonical_source_games_link l
		JOIN canonical_games cg ON cg.id = l.canonical_id
		JOIN source_games sg ON sg.id = l.source_game_id
		LEFT JOIN metadata_resolver_matches m ON m.source_game_id = sg.id AND m.outvoted = 0
		LEFT JOIN canonical_source_pins p ON p.source_game_id = sg.id AND p.profile_id = sg.profile_id
		WHERE 1=1`+profileFilterSQL(ctx, "sg")+`
		ORDER BY l.canonical_id, sg.id, m.plugin_id, m.external_id`)
	if err != nil {
		return nil, nil, fmt.Errorf("load game identity evidence: %w", err)
	}
	defer rows.Close()
	seeds := map[string]*editionIdentitySeed{}
	for rows.Next() {
		var canonicalID, profileID, rawTitle, sourcePlatform, sourceKind string
		var provider, externalID, providerTitle, matchPlatform, metadataJSON, pinMode string
		var createdAt int64
		if err := rows.Scan(&canonicalID, &createdAt, &profileID, &rawTitle, &sourcePlatform, &sourceKind,
			&provider, &externalID, &providerTitle, &matchPlatform, &metadataJSON, &pinMode); err != nil {
			return nil, nil, fmt.Errorf("scan game identity evidence: %w", err)
		}
		seed := seeds[canonicalID]
		if seed == nil {
			seed = &editionIdentitySeed{
				id:             canonicalID,
				profileID:      profileID,
				createdAt:      createdAt,
				providerTitles: map[string]int{},
				fallbackTitles: map[string]int{},
				platforms:      map[string]bool{},
				kinds:          map[string]bool{},
				evidence:       map[string]core.IdentityEvidence{},
			}
			seeds[canonicalID] = seed
		}
		if seed.profileID != profileID {
			seed.profileConflict = true
		}
		addIdentityCandidate(seed.fallbackTitles, rawTitle)
		addIdentityValue(seed.platforms, firstKnownIdentityValue(sourcePlatform, matchPlatform, string(core.PlatformUnknown)))
		matchKind := ""
		if strings.TrimSpace(metadataJSON) != "" {
			match := core.ResolverMatch{}
			parseMetadataJSON(metadataJSON, &match)
			matchKind = match.Kind
		}
		addIdentityValue(seed.kinds, firstKnownIdentityValue(sourceKind, matchKind, string(core.GameKindUnknown)))
		if provider != "" && externalID != "" {
			key := provider + "\x00" + externalID
			seed.evidence[key] = core.IdentityEvidence{Provider: provider, ExternalID: externalID}
			addIdentityCandidate(seed.providerTitles, providerTitle)
		}
		if pinMode != "" {
			seed.manual = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate game identity evidence: %w", err)
	}

	existing := map[string]persistedTitleIdentity{}
	existingRows, err := tx.QueryContext(ctx, `SELECT e.id, e.title_id, t.created_at,
		COALESCE(e.region, ''), COALESCE(e.edition_label, '')
		FROM game_editions e JOIN game_titles t ON t.id = e.title_id`)
	if err != nil {
		return nil, nil, fmt.Errorf("load existing game identities: %w", err)
	}
	defer existingRows.Close()
	for existingRows.Next() {
		var editionID, titleID, region, label string
		var createdAt int64
		if err := existingRows.Scan(&editionID, &titleID, &createdAt, &region, &label); err != nil {
			return nil, nil, fmt.Errorf("scan existing game identity: %w", err)
		}
		existing[titleID] = persistedTitleIdentity{id: titleID, createdAt: createdAt}
		if seed := seeds[editionID]; seed != nil {
			seed.existingTitle = titleID
			seed.existingRegion = region
			seed.existingLabel = label
		}
	}
	if err := existingRows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate existing game identities: %w", err)
	}
	return seeds, existing, nil
}

func (r *identityReconciler) reconcileProfile(ctx context.Context, tx *sql.Tx, profileID string, seeds []*editionIdentitySeed, existing map[string]persistedTitleIdentity) error {
	sort.Slice(seeds, func(i, j int) bool { return seeds[i].id < seeds[j].id })
	ids := make([]string, 0, len(seeds))
	byID := make(map[string]*editionIdentitySeed, len(seeds))
	for _, seed := range seeds {
		ids = append(ids, seed.id)
		byID[seed.id] = seed
	}
	groups := newIdentityUnionFind(ids)

	byEvidence := map[string][]string{}
	byExistingTitle := map[string][]string{}
	for _, seed := range seeds {
		for key := range seed.evidence {
			byEvidence[key] = append(byEvidence[key], seed.id)
		}
		if seed.existingTitle != "" {
			byExistingTitle[seed.existingTitle] = append(byExistingTitle[seed.existingTitle], seed.id)
		}
	}
	for _, members := range byEvidence {
		unionIdentityMembers(groups, members)
	}
	// Once a provider-confirmed Title has been established, temporary provider
	// outages must not split its identity on the next scan.
	for _, members := range byExistingTitle {
		unionExistingIdentityMembers(groups, members, byID)
	}

	memberGroups := map[string][]*editionIdentitySeed{}
	for _, seed := range seeds {
		memberGroups[groups.find(seed.id)] = append(memberGroups[groups.find(seed.id)], seed)
	}
	groupKeys := make([]string, 0, len(memberGroups))
	for key := range memberGroups {
		groupKeys = append(groupKeys, key)
	}
	sort.Strings(groupKeys)

	usedTitleIDs := map[string]bool{}
	assignedTitle := map[string]string{}
	for _, groupKey := range groupKeys {
		candidates := map[string]persistedTitleIdentity{}
		for _, seed := range memberGroups[groupKey] {
			if meta, ok := existing[seed.existingTitle]; ok {
				candidates[meta.id] = meta
			}
		}
		ordered := make([]persistedTitleIdentity, 0, len(candidates))
		for _, candidate := range candidates {
			ordered = append(ordered, candidate)
		}
		sort.Slice(ordered, func(i, j int) bool {
			if ordered[i].createdAt != ordered[j].createdAt {
				return ordered[i].createdAt < ordered[j].createdAt
			}
			return ordered[i].id < ordered[j].id
		})
		for _, candidate := range ordered {
			if !usedTitleIDs[candidate.id] {
				assignedTitle[groupKey] = candidate.id
				usedTitleIDs[candidate.id] = true
				break
			}
		}
		if assignedTitle[groupKey] == "" {
			assignedTitle[groupKey] = uuid.NewString()
			usedTitleIDs[assignedTitle[groupKey]] = true
		}
	}

	now := time.Now().Unix()
	for _, groupKey := range groupKeys {
		members := memberGroups[groupKey]
		titleID := assignedTitle[groupKey]
		displayTitle := selectIdentityTitle(members)
		normalizedTitle := titlematch.NormalizeLookupTitle(displayTitle)
		createdAt := now
		if meta, ok := existing[titleID]; ok {
			createdAt = meta.createdAt
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO game_titles
			(id, profile_id, display_title, normalized_title, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET profile_id=excluded.profile_id,
				display_title=excluded.display_title, normalized_title=excluded.normalized_title,
				updated_at=excluded.updated_at`, titleID, profileID, displayTitle, normalizedTitle, createdAt, now); err != nil {
			return fmt.Errorf("upsert game title %s: %w", titleID, err)
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM game_title_external_ids WHERE title_id=?`, titleID); err != nil {
			return fmt.Errorf("clear game title evidence %s: %w", titleID, err)
		}
		evidence := collectIdentityEvidence(members)
		for _, item := range evidence {
			if _, err := tx.ExecContext(ctx, `INSERT INTO game_title_external_ids (title_id, provider, external_id) VALUES (?, ?, ?)`, titleID, item.Provider, item.ExternalID); err != nil {
				return fmt.Errorf("insert game title evidence %s: %w", titleID, err)
			}
		}
		for _, seed := range members {
			platform, platformConflict := selectIdentityValue(seed.platforms, string(core.PlatformUnknown))
			kind, kindConflict := selectIdentityValue(seed.kinds, string(core.GameKindUnknown))
			seed.conflict = platformConflict || kindConflict
			state := identityState(seed, platform)
			editionCreatedAt := seed.createdAt
			if editionCreatedAt == 0 {
				editionCreatedAt = now
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO game_editions
				(id, title_id, platform, region, edition_label, kind, identity_state, created_at, updated_at)
				VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?)
				ON CONFLICT(id) DO UPDATE SET title_id=excluded.title_id, platform=excluded.platform,
					kind=excluded.kind, identity_state=excluded.identity_state, updated_at=excluded.updated_at`,
				seed.id, titleID, platform, seed.existingRegion, seed.existingLabel, kind, state, editionCreatedAt, now); err != nil {
				return fmt.Errorf("upsert game edition %s: %w", seed.id, err)
			}
		}
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM game_editions
		WHERE title_id IN (SELECT id FROM game_titles WHERE profile_id = ?)
		AND id NOT IN (
			SELECT DISTINCT l.canonical_id FROM canonical_source_games_link l
			JOIN source_games sg ON sg.id = l.source_game_id WHERE sg.profile_id = ?
		)`, profileID, profileID); err != nil {
		return fmt.Errorf("remove stale game editions for profile %s: %w", profileID, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM game_titles WHERE profile_id=?
		AND id NOT IN (SELECT DISTINCT title_id FROM game_editions)`, profileID); err != nil {
		return fmt.Errorf("remove stale game titles for profile %s: %w", profileID, err)
	}
	return nil
}

func unionIdentityMembers(groups *identityUnionFind, members []string) {
	if len(members) < 2 {
		return
	}
	sort.Strings(members)
	for i := 1; i < len(members); i++ {
		groups.union(members[0], members[i])
	}
}

func unionExistingIdentityMembers(groups *identityUnionFind, members []string, seeds map[string]*editionIdentitySeed) {
	if len(members) < 2 {
		return
	}
	evidenceRoots := map[string]bool{}
	for _, member := range members {
		seed := seeds[member]
		if seed != nil && len(seed.evidence) > 0 {
			evidenceRoots[groups.find(member)] = true
		}
	}
	// Preserve an established title through a provider outage only when the
	// remaining accepted evidence still points to at most one identity. If
	// provider-backed roots now disagree, separating them is safer than carrying
	// a stale automatic merge forward.
	if len(evidenceRoots) <= 1 {
		unionIdentityMembers(groups, members)
	}
}

func addIdentityCandidate(candidates map[string]int, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		candidates[value]++
	}
}

func addIdentityValue(values map[string]bool, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		values[value] = true
	}
}

func firstKnownIdentityValue(primary, secondary, unknown string) string {
	primary = strings.TrimSpace(primary)
	if primary != "" && primary != unknown {
		return primary
	}
	secondary = strings.TrimSpace(secondary)
	if secondary != "" && secondary != unknown {
		return secondary
	}
	return unknown
}

func selectIdentityValue(values map[string]bool, unknown string) (string, bool) {
	known := make([]string, 0, len(values))
	for value := range values {
		if value != "" && value != unknown {
			known = append(known, value)
		}
	}
	sort.Strings(known)
	if len(known) == 0 {
		return unknown, false
	}
	if len(known) > 1 {
		return unknown, true
	}
	return known[0], false
}

func selectIdentityTitle(seeds []*editionIdentitySeed) string {
	provider := map[string]int{}
	fallback := map[string]int{}
	for _, seed := range seeds {
		for title, count := range seed.providerTitles {
			provider[title] += count
		}
		for title, count := range seed.fallbackTitles {
			fallback[title] += count
		}
	}
	if title := highestCountIdentityTitle(provider); title != "" {
		return title
	}
	if title := highestCountIdentityTitle(fallback); title != "" {
		return title
	}
	return "Unknown game"
}

func highestCountIdentityTitle(candidates map[string]int) string {
	best, bestCount := "", -1
	for title, count := range candidates {
		if count > bestCount || (count == bestCount && title < best) {
			best, bestCount = title, count
		}
	}
	return best
}

func collectIdentityEvidence(seeds []*editionIdentitySeed) []core.IdentityEvidence {
	unique := map[string]core.IdentityEvidence{}
	for _, seed := range seeds {
		for key, evidence := range seed.evidence {
			unique[key] = evidence
		}
	}
	out := make([]core.IdentityEvidence, 0, len(unique))
	for _, evidence := range unique {
		out = append(out, evidence)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].ExternalID < out[j].ExternalID
	})
	return out
}

func identityState(seed *editionIdentitySeed, platform string) string {
	if seed.manual {
		return "manual"
	}
	if seed.conflict {
		return "legacy_review"
	}
	if len(seed.evidence) > 0 && platform != "" && platform != string(core.PlatformUnknown) {
		return "provider_confirmed"
	}
	return "unresolved"
}

func loadGameIdentity(ctx context.Context, db *sql.DB, canonicalID string) (*core.GameIdentity, error) {
	var identity core.GameIdentity
	var titleCreatedAt, titleUpdatedAt, editionCreatedAt, editionUpdatedAt int64
	err := db.QueryRowContext(ctx, `SELECT t.id, t.display_title, t.normalized_title, t.created_at, t.updated_at,
		e.id, e.title_id, e.platform, COALESCE(e.region, ''), COALESCE(e.edition_label, ''),
		e.kind, e.identity_state, e.created_at, e.updated_at
		FROM game_editions e JOIN game_titles t ON t.id=e.title_id WHERE e.id=?`, canonicalID).Scan(
		&identity.Title.ID, &identity.Title.DisplayTitle, &identity.Title.NormalizedTitle, &titleCreatedAt, &titleUpdatedAt,
		&identity.Edition.ID, &identity.Edition.TitleID, (*string)(&identity.Edition.Platform), &identity.Edition.Region,
		&identity.Edition.EditionLabel, (*string)(&identity.Edition.Kind), &identity.Edition.State, &editionCreatedAt, &editionUpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("load game identity %s: %w", canonicalID, err)
	}
	identity.Title.CreatedAt = time.Unix(titleCreatedAt, 0).UTC()
	identity.Title.UpdatedAt = time.Unix(titleUpdatedAt, 0).UTC()
	identity.Edition.CreatedAt = time.Unix(editionCreatedAt, 0).UTC()
	identity.Edition.UpdatedAt = time.Unix(editionUpdatedAt, 0).UTC()
	rows, err := db.QueryContext(ctx, `SELECT provider, external_id FROM game_title_external_ids
		WHERE title_id=? ORDER BY provider, external_id`, identity.Title.ID)
	if err != nil {
		return nil, fmt.Errorf("load game identity evidence %s: %w", canonicalID, err)
	}
	defer rows.Close()
	for rows.Next() {
		var evidence core.IdentityEvidence
		if err := rows.Scan(&evidence.Provider, &evidence.ExternalID); err != nil {
			return nil, fmt.Errorf("scan game identity evidence %s: %w", canonicalID, err)
		}
		identity.Evidence = append(identity.Evidence, evidence)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate game identity evidence %s: %w", canonicalID, err)
	}
	return &identity, nil
}
