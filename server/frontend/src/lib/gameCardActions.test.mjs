import assert from 'node:assert/strict'
import test from 'node:test'
import { GameCardActionResolver } from './gameCardActions.ts'

const noop = () => undefined
const details = { id: 'details', label: 'Details', kind: 'details', onSelect: noop }
const browser = { id: 'browser', label: 'Play in browser', kind: 'play', route: 'browser', onSelect: noop }
const cloud = { id: 'cloud', label: 'Play in xCloud', kind: 'play', route: 'cloud', onSelect: noop }

test('route-specific shelf selects its route and preserves the other route', () => {
  const result = new GameCardActionResolver({
    alternateActions: [],
    derivedActions: [browser, cloud],
    preferredPlayRoute: 'cloud',
    fallbackAction: details,
  }).resolve()

  assert.equal(result.primaryAction, cloud)
  assert.deepEqual(result.alternateActions, [browser])
})

test('installed device action remains primary while browser and cloud remain selectable', () => {
  const local = { id: 'local', label: 'Device offline', disabled: true, route: 'local', onSelect: noop }
  const result = new GameCardActionResolver({
    primaryAction: local,
    alternateActions: [],
    derivedActions: [browser, cloud],
    fallbackAction: details,
  }).resolve()

  assert.equal(result.primaryAction, local)
  assert.deepEqual(result.alternateActions, [browser, cloud])
})

test('explicit browser action replaces rather than duplicates the derived browser route', () => {
  const configuredBrowser = { ...browser, id: 'configured-browser', label: 'Play with EmulatorJS' }
  const result = new GameCardActionResolver({
    primaryAction: configuredBrowser,
    alternateActions: [],
    derivedActions: [browser, cloud],
    fallbackAction: details,
  }).resolve()

  assert.equal(result.primaryAction, configuredBrowser)
  assert.deepEqual(result.alternateActions, [cloud])
})

test('details is used only when no route or explicit action exists', () => {
  const result = new GameCardActionResolver({
    alternateActions: [],
    derivedActions: [],
    fallbackAction: details,
  }).resolve()

  assert.equal(result.primaryAction, details)
  assert.deepEqual(result.alternateActions, [])
})
