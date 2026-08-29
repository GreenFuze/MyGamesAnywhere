import assert from 'node:assert/strict'
import test from 'node:test'

import { GamePresentation } from './gamePresentation.ts'

function game(overrides = {}) {
  return {
    id: 'game-1',
    title: 'Double Dragon',
    favorite: false,
    platform: 'nes',
    kind: 'base_game',
    source_games: [],
    ...overrides,
  }
}

test('compact presentation exposes at most three deterministic facts', () => {
  const presentation = new GamePresentation(game({
    kind: 'dlc',
    play: { available: true },
  }))

  assert.equal(presentation.content.badgeLabel, 'DLC')
  assert.equal(presentation.availability, 'playable')
  assert.equal(presentation.platform, 'NES')
  assert.equal(presentation.compactBadgeCount, 3)
})

// A retired device agent can no longer contribute an installed or emulator
// availability state; MGA never reports device-local placement again.
test('retired device evidence never produces an installed or emulator state', () => {
  const presentation = new GamePresentation(game({
    play: { available: true, options: [{ kind: 'xcloud', launchable: true }] },
    xcloud_available: true,
    devices: [{
      installed: true,
      connected: true,
      can_play: true,
      emulator_routes: [{ state: 'ready' }],
    }],
  }))

  assert.notEqual(presentation.availability, 'installed')
  assert.notEqual(presentation.availability, 'emulator')
  assert.equal(presentation.availability, 'playable')
  assert.equal(presentation.game.xcloud_available, true)
})

test('connections are deduplicated and sorted for stable display', () => {
  const presentation = new GamePresentation(game({
    source_games: [
      { integration_id: 'z', integration_label: 'Xbox', plugin_id: 'xbox' },
      { integration_id: 'a', integration_label: 'Arcade shelf', plugin_id: 'smb' },
      { integration_id: 'z', integration_label: 'Xbox', plugin_id: 'xbox' },
    ],
  }))

  assert.deepEqual(
    presentation.sources.map((source) => source.label),
    ['Arcade shelf', 'Xbox'],
  )
  assert.equal(presentation.foundInLabel, 'Found in Arcade shelf and Xbox')
  assert.equal(presentation.copyCountLabel, '3 copies')
})
