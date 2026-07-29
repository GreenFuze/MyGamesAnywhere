import assert from 'node:assert/strict'
import test from 'node:test'

import { parsePluginConfigSchema } from './pluginConfig.ts'
import { launchOptionVersionContext, sourceVersionContext } from './sourceCapabilities.ts'

test('source evidence names the concrete connection, platform, and release title', () => {
  assert.equal(sourceVersionContext({
    plugin_id: 'game-source-google-drive',
    integration_label: 'Orr games',
    platform: 'nes',
    raw_title: 'Double Dragon (World)',
  }), 'Orr games · NES · Double Dragon (World)')
})

test('launch evidence resolves through the exact source record', () => {
  const source = {
    id: 'source-japan',
    plugin_id: 'game-source-smb',
    integration_label: 'NAS',
    platform: 'nes',
    raw_title: 'Double Dragon (Japan)',
  }
  assert.equal(launchOptionVersionContext({
    kind: 'browser',
    source_game_id: 'source-japan',
    launchable: true,
  }, [source]), 'NAS · NES · Double Dragon (Japan)')
})

test('provider-managed credentials never become editable config fields', () => {
  const schema = parsePluginConfigSchema({
    api_key: { type: 'string', 'x-secret': true },
    refresh_token: { type: 'string', 'x-secret': true, 'x-auth-method': 'qr' },
  })
  assert.deepEqual(schema.map(({ key }) => key), ['api_key'])
})
