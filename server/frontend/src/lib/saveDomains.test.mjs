import assert from 'node:assert/strict'
import test from 'node:test'
import { collectSaveDomains, saveDomainStatusLabel, saveDomainSummary } from './saveDomains.ts'

const capability = (domain_id, access, status, extra = {}) => ({
  domain_id, access, status, manager: 'unknown', label: domain_id, detail: 'detail',
  mga_read: false, mga_write: false, transfer: 'unknown', ...extra,
})

test('collectSaveDomains keeps distinct route domains and removes duplicate projections', () => {
  const browser = capability('browser', 'mga_managed', 'available', { manager: 'mga', mga_read: true, mga_write: true })
  const provider = capability('steam', 'provider_opaque', 'provider_managed', { manager: 'provider' })
  const emulator = capability('emulator', 'local_files', 'needs_adapter', { manager: 'device' })
  const domains = collectSaveDomains({
    source_games: [{ id: 'source', raw_title: 'Game', save: provider }],
    play: { available: true, platform_supported: true, options: [{ kind: 'browser', source_game_id: 'source', launchable: true, save: browser }, { kind: 'duplicate', source_game_id: 'source', launchable: true, save: browser }] },
    devices: [{ device_id: 'pc', display_name: 'PC', emulator_routes: [{ emulator_id: 'retroarch', emulator_name: 'RetroArch', source_game_id: 'source', source_title: 'Game', state: 'ready', default: true, save: emulator }] }],
  })
  assert.deepEqual(domains.map((domain) => domain.domain_id), ['steam', 'browser', 'emulator'])
  assert.equal(saveDomainSummary(domains), 'MGA save backup available')
})

test('labels provider and local boundaries without promising access', () => {
  assert.equal(saveDomainSummary([capability('steam', 'provider_opaque', 'provider_managed')]), 'Provider-managed saves')
  assert.equal(saveDomainSummary([capability('local', 'local_files', 'needs_adapter')]), 'Local saves need setup')
  assert.equal(saveDomainStatusLabel(capability('steam', 'provider_opaque', 'provider_managed')), 'Managed by provider')
})
