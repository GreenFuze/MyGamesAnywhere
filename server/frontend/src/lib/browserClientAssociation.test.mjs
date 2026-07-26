import assert from 'node:assert/strict'
import test from 'node:test'
import {
  findNewConnectedEndpointIDs,
  resolveBrowserClientEndpointID,
  resolveNewlyPairedEndpointID,
} from './browserClientAssociation.ts'
import { resolveClientStatusPresentation } from './clientStatusPresentation.ts'

test('browser-local client association never guesses from authorized endpoints', () => {
  assert.equal(resolveBrowserClientEndpointID('', ['only-remote-device']), '')
  assert.equal(resolveBrowserClientEndpointID('local-device', ['local-device', 'remote-device']), 'local-device')
  assert.equal(resolveBrowserClientEndpointID('not-granted-to-this-profile', ['remote-device']), '')
})

test('an explicit pairing attempt accepts exactly one new connected endpoint', () => {
  const baseline = ['remote-device']
  const endpoints = [
    { id: 'remote-device', status: 'ready' },
    { id: 'local-device', status: 'ready' },
  ]

  assert.deepEqual(findNewConnectedEndpointIDs(baseline, endpoints), ['local-device'])
  assert.equal(resolveNewlyPairedEndpointID(baseline, endpoints), 'local-device')
})

test('pairing association fails closed for offline, pre-existing, or ambiguous endpoints', () => {
  assert.equal(resolveNewlyPairedEndpointID([], [{ id: 'offline-device', status: 'offline' }]), '')
  assert.equal(resolveNewlyPairedEndpointID(['existing-device'], [{ id: 'existing-device', status: 'ready' }]), '')
  assert.equal(resolveNewlyPairedEndpointID([], [
    { id: 'device-a', status: 'ready' },
    { id: 'device-b', status: 'busy' },
  ]), '')
})

test('top-bar presentation requires a proven browser-local endpoint', () => {
  assert.equal(resolveClientStatusPresentation(true, undefined, false).label, 'Connect client')
  assert.equal(resolveClientStatusPresentation(false, undefined, false).label, 'Client unavailable')
})
