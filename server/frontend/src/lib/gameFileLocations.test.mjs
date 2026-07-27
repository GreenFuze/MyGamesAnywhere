import assert from 'node:assert/strict'
import test from 'node:test'

import { buildGameFileLocations, gameFileLocationSaveLabel } from './gameFileLocations.ts'

const source = {
  id: 'source-orr',
  integration_id: 'integration-orr',
  integration_label: 'Orr Drive',
  plugin_id: 'game-source-google-drive',
  external_id: 'double-dragon',
  raw_title: 'Double Dragon (World)',
  platform: 'nes',
  kind: 'base_game',
  status: 'found',
  created_at: '2026-07-27T00:00:00Z',
  files: [{ id: 'rom', path: 'NES/Double Dragon (World).nes', role: 'content', file_kind: 'rom', size: 42 }],
  resolver_matches: [],
}

test('keeps selected profile, source, prepared, device user, and route evidence separate', () => {
  const rows = buildGameFileLocations({
    id: 'game',
    source_games: [source],
    devices: [{
      device_id: 'tv2-orr',
      display_name: 'TV2',
      os_user: 'orr',
      status: 'installed',
      connected: true,
      can_manage: true,
      can_play: true,
      platform_supported: true,
      installed: true,
      installed_source_id: source.id,
      install_path: 'C:\\Users\\orr\\Games\\Double Dragon',
      install_state: 'installed',
      archive_install_supported: true,
      gog_inno_install_supported: false,
      failed_cleanup_supported: false,
      uninstall_supported: true,
      launch_supported: true,
      use_existing_supported: true,
      emulator_routes: [{
        emulator_id: 'retroarch',
        emulator_name: 'RetroArch',
        source_game_id: source.id,
        source_title: source.raw_title,
        state: 'ready',
        default: true,
      }],
    }],
  }, [{
    id: 'cache',
    cache_key: 'cache',
    canonical_game_id: 'game',
    source_game_id: source.id,
    integration_id: source.integration_id,
    plugin_id: source.plugin_id,
    profile: 'browser',
    mode: 'materialized',
    status: 'ready',
    file_count: 1,
    size: 42,
    created_at: '2026-07-27T00:00:00Z',
    updated_at: '2026-07-27T00:00:00Z',
  }], { id: 'profile-orr', displayName: 'Orr' })

  assert.deepEqual(rows.map((row) => row.kind), ['source', 'prepared', 'installed', 'emulator'])
  assert.ok(rows.every((row) => row.ownerProfileId === 'profile-orr'))
  assert.equal(rows[2].osUser, 'orr')
  assert.equal(rows[3].routeId, 'retroarch')
})

test('rejects prepared entries whose source or owning connection does not match this game', () => {
  const rows = buildGameFileLocations({
    id: 'game',
    source_games: [source],
  }, [
    {
      id: 'wrong-source',
      cache_key: 'wrong-source',
      canonical_game_id: 'game',
      source_game_id: 'source-tc',
      integration_id: 'integration-tc',
      plugin_id: source.plugin_id,
      profile: 'browser',
      mode: 'materialized',
      status: 'ready',
      file_count: 1,
      size: 42,
      created_at: '2026-07-27T00:00:00Z',
      updated_at: '2026-07-27T00:00:00Z',
    },
    {
      id: 'wrong-connection',
      cache_key: 'wrong-connection',
      canonical_game_id: 'game',
      source_game_id: source.id,
      integration_id: 'integration-tc',
      plugin_id: source.plugin_id,
      profile: 'browser',
      mode: 'materialized',
      status: 'ready',
      file_count: 1,
      size: 42,
      created_at: '2026-07-27T00:00:00Z',
      updated_at: '2026-07-27T00:00:00Z',
    },
  ], { id: 'profile-orr', displayName: 'Orr' })

  assert.deepEqual(rows.map((row) => row.kind), ['source'])
})

test('never implies save compatibility without exact route evidence', () => {
  assert.equal(gameFileLocationSaveLabel({}), 'Save compatibility not known')
  assert.equal(gameFileLocationSaveLabel({
    save: {
      domain_id: 'steam:game',
      access: 'provider_opaque',
      status: 'provider_managed',
      manager: 'provider',
      label: 'Steam Cloud',
      detail: 'Provider managed',
      mga_read: false,
      mga_write: false,
      transfer: 'unknown',
    },
  }), 'Saves managed by provider')
})

test('fails fast when no selected profile owns the view', () => {
  assert.throws(
    () => buildGameFileLocations({ id: 'game', source_games: [source] }, [], { id: '', displayName: 'Orr' }),
    /selected profile identity is required/i,
  )
})
