import assert from 'node:assert/strict'
import test from 'node:test'
import { ProviderCatalog, describeProvider } from './providerCatalog.ts'

const steam = {
  plugin_id: 'game-source-steam',
  plugin_version: '1',
  provides: ['source.games.list', 'achievements.game.get', 'auth.oauth.callback'],
  capabilities: ['source'],
  config: {},
}
const smb = {
  plugin_id: 'game-source-smb',
  plugin_version: '1',
  provides: ['source.filesystem.list'],
  capabilities: ['source'],
  config: {
    host: { type: 'string', required: true },
    share: { type: 'string', required: true },
    password: { type: 'string', required: true, 'x-secret': true },
  },
}
const localDisk = {
  plugin_id: 'save-sync-local-disk',
  plugin_version: '1',
  provides: ['sync.push'],
  capabilities: ['save_sync'],
  config: { root_path: { type: 'string', required: true } },
}
const hltb = {
  plugin_id: 'metadata-hltb',
  plugin_version: '1',
  provides: ['metadata.game.lookup'],
  capabilities: ['metadata'],
  config: {},
}

test('a connection is filed under the plugin capability, never assumed to be a source', () => {
  // Regression: every connection was created as integration_type "source",
  // which mis-filed metadata and save-sync providers.
  assert.equal(describeProvider(hltb).integrationType, 'metadata')
  assert.equal(describeProvider(localDisk).integrationType, 'save_sync')
  assert.equal(describeProvider(steam).integrationType, 'source')
})

test('providers are described by how they are actually set up', () => {
  // A provider that redirects for consent is a sign-in provider even though it
  // also lists other capabilities.
  assert.equal(describeProvider(steam).setup.kind, 'sign_in')
  assert.equal(describeProvider(smb).setup.kind, 'credentials')
  assert.equal(describeProvider(localDisk).setup.kind, 'location')
  assert.equal(describeProvider(hltb).setup.kind, 'none')

  for (const plugin of [steam, smb, localDisk, hltb]) {
    const described = describeProvider(plugin)
    assert.ok(described.setup.summary.length > 0)
    assert.ok(described.setup.detail.length > 0)
  }
})

test('providers are named, never shown as a raw plugin id', () => {
  for (const plugin of [steam, smb, localDisk, hltb]) {
    assert.notEqual(describeProvider(plugin).name, plugin.plugin_id)
  }
})

test('the catalog groups providers by kind in the product order', () => {
  const catalog = new ProviderCatalog([hltb, localDisk, steam, smb])
  const categories = catalog.categories()
  const capabilities = categories.map((entry) => entry.capability)

  // "source" precedes "metadata", which precedes "save_sync".
  assert.deepEqual(capabilities, ['source', 'metadata', 'save_sync'])
  assert.equal(categories[0].label, 'Game Connections')
  assert.equal(catalog.providersFor('source').length, 2)
  assert.equal(catalog.providersFor('metadata').length, 1)
  assert.equal(catalog.providersFor('nonexistent').length, 0)
})

test('a provider can be found again by id for editing', () => {
  const catalog = new ProviderCatalog([hltb, steam])
  assert.equal(catalog.find('game-source-steam')?.capability, 'source')
  assert.equal(catalog.find('not-installed'), undefined)
})

test('an unknown capability still appears rather than vanishing', () => {
  const odd = { plugin_id: 'weird-thing', plugin_version: '1', provides: [], capabilities: ['experimental'], config: {} }
  const capabilities = new ProviderCatalog([odd, hltb]).categories().map((entry) => entry.capability)
  assert.deepEqual(capabilities, ['metadata', 'experimental'])
})
