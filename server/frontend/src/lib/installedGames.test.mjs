import assert from 'node:assert/strict'
import test from 'node:test'
import { resolveAssociatedEndpointID } from './clientEndpointAssociation.ts'
import { resolveInstalledGameAction } from './installedGameAction.ts'
import { installationReasonLabel, validationStatusLabel } from './installationValidation.ts'

test('device association keeps valid preference and ignores stale ids', () => {
  assert.equal(resolveAssociatedEndpointID('device-b', ['device-a', 'device-b']), 'device-b')
  assert.equal(resolveAssociatedEndpointID('deleted-device', ['device-a', 'device-b']), '')
})

test('device association uses a non-persisted single-device fallback', () => {
  assert.equal(resolveAssociatedEndpointID('', ['only-device']), 'only-device')
  assert.equal(resolveAssociatedEndpointID('', ['device-a', 'device-b']), '')
})

test('installed game action reflects exact device and launch state', () => {
  const ready = {
    launching: false,
    deviceStatus: 'ready',
    connected: true,
    accessLevel: 'play',
    launchSupported: true,
    launchTarget: 'Game.exe',
    canPlay: true,
  }
  assert.deepEqual(resolveInstalledGameAction(ready), {
    label: 'Play', intent: 'launch', disabled: false, kind: 'play',
  })
  assert.equal(resolveInstalledGameAction({ ...ready, connected: false }).label, 'Offline')
  assert.equal(resolveInstalledGameAction({ ...ready, deviceStatus: 'update_required', canPlay: false }).label, 'Needs update')
  assert.equal(resolveInstalledGameAction({ ...ready, accessLevel: 'view', canPlay: false }).label, 'View only')
  assert.equal(resolveInstalledGameAction({ ...ready, launchTarget: '', canPlay: false }).label, 'Choose executable')
  assert.equal(resolveInstalledGameAction({ ...ready, launching: true }).label, 'Starting…')
})

test('installation validation uses player-facing reasons and visible schedule states', () => {
  assert.equal(installationReasonLabel('launch_target_missing'), 'The executable used to start this game is missing.')
  assert.equal(installationReasonLabel('registered_program_missing'), 'Windows no longer lists this game as installed.')
  assert.equal(validationStatusLabel({ state: 'running', eligible_count: 1 }), 'Checking now…')
  assert.equal(validationStatusLabel({ state: 'disabled', eligible_count: 1 }), 'Automatic checks paused')
  assert.equal(validationStatusLabel({ state: 'scheduled', eligible_count: 1, last_finished_at: '2026-07-16T12:00:00Z' }, () => 'formatted'), 'Last checked formatted')
})
