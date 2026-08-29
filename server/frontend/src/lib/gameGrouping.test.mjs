import assert from 'node:assert/strict'
import test from 'node:test'

import { GameGroupingResolver } from './gameGrouping.ts'

function game(overrides = {}) {
  return {
    id: 'game-1',
    title: 'Double Dragon',
    platform: 'nes',
    source_games: [],
    ...overrides,
  }
}

test('one canonical title remains in every concrete integration group even when labels match', () => {
  const item = game({
    source_games: [
      { id: 'source-a', integration_id: 'connection-a', integration_label: 'Family games', plugin_id: 'game-source-smb' },
      { id: 'source-b', integration_id: 'connection-b', integration_label: 'Family games', plugin_id: 'game-source-google-drive' },
    ],
  })

  const groups = new GameGroupingResolver('integration').build([item])
  assert.equal(groups.length, 2)
  assert.deepEqual(groups.map((group) => group.key), [
    'integration:connection-a',
    'integration:connection-b',
  ])
  assert.ok(groups.every((group) => group.games[0] === item))
})

// Installed and emulator groups came from the retired device agent. Retired
// device rows must not resurrect them.
test('play-method grouping ignores retired device evidence', () => {
  const item = game({
    xcloud_available: true,
    play: { available: true, platform_supported: true },
    source_games: [{
      id: 'source-a',
      integration_id: 'connection-a',
      plugin_id: 'game-source-google-drive',
      platform: 'nes',
      delivery: { profiles: [{ profile: 'emulatorjs', mode: 'direct', ready: true, root_file_id: 'rom' }] },
      files: [{ id: 'rom', path: 'Double Dragon.nes', role: 'root', size: 1 }],
    }],
    devices: [{
      device_id: 'pc',
      display_name: 'Gaming PC',
      installed: true,
      emulator_routes: [{ emulator_id: 'retroarch', source_game_id: 'source-a', state: 'ready' }],
    }],
  })

  const labels = new GameGroupingResolver('play_method').build([item]).map((group) => group.label)
  assert.deepEqual(labels, ['Cloud play', 'Play in browser'])
})
