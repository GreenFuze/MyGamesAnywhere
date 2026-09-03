import assert from 'node:assert/strict'
import test from 'node:test'
import { DESTRUCTIVE_ACTIONS, ManagementPolicy } from './managementPolicy.ts'

const admin = { id: 'admin-1', display_name: 'Admin', role: 'admin_player', avatar_key: 'player-1', created_at: '', updated_at: '' }
const player = { id: 'player-1', display_name: 'Player', role: 'player', avatar_key: 'player-2', created_at: '', updated_at: '' }

const ALL_CAPABILITIES = [
  'profile.create', 'profile.edit', 'profile.delete', 'profile.credentialTicket',
  'source.create', 'source.edit', 'source.delete', 'source.authorize', 'source.maintain',
  'scan.control', 'apiClient.issue', 'apiClient.rotate', 'apiClient.revoke',
  'legacyExport.download',
]

test('management writes fail closed without a selected profile', () => {
  const policy = new ManagementPolicy(null)
  assert.equal(policy.isAdministrator, false)
  assert.equal(policy.profileId, null)
  for (const capability of ALL_CAPABILITIES) {
    assert.equal(policy.can(capability), false, `${capability} must be denied without a profile`)
  }
  assert.equal(policy.reasonFor('source.create'), 'Select a profile to manage this server.')
})

test('an administrator may perform every management write', () => {
  const policy = new ManagementPolicy(admin)
  assert.equal(policy.isAdministrator, true)
  for (const capability of ALL_CAPABILITIES) {
    assert.equal(policy.can(capability), true, `${capability} must be allowed for an administrator`)
  }
  assert.equal(policy.reasonFor('profile.delete'), null)
})

test('a player is denied administrator writes and told why', () => {
  const policy = new ManagementPolicy(player)
  assert.equal(policy.isAdministrator, false)
  for (const capability of ALL_CAPABILITIES) {
    assert.equal(policy.can(capability), false, `${capability} must require an administrator`)
    assert.equal(policy.reasonFor(capability), 'This action requires an administrator profile.')
  }
})

test('record ownership never crosses a profile boundary', () => {
  const policy = new ManagementPolicy(player)
  assert.equal(policy.owns({ profile_id: 'player-1' }), true)
  assert.equal(policy.owns({ profile_id: 'admin-1' }), false)
  assert.equal(policy.owns({}), false)
  assert.equal(policy.owns(null), false)
  assert.equal(new ManagementPolicy(null).owns({ profile_id: 'player-1' }), false)
})

test('every destructive action states consequences and protects user content', () => {
  for (const [action, copy] of Object.entries(DESTRUCTIVE_ACTIONS)) {
    assert.ok(copy.consequences.length > 0, `${action} must state what it changes`)
    assert.ok(copy.preserves.length > 0, `${action} must state what it leaves alone`)

    // No destructive management action may imply it removes user content.
    const preserved = copy.preserves.join(' ').toLowerCase()
    assert.ok(
      /file|content|save/.test(preserved),
      `${action} must promise that files, content, or saves survive`,
    )
  }
})
