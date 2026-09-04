import assert from 'node:assert/strict'
import test from 'node:test'
import { describeMissingFields, fieldLabel, initialConfigValues, missingRequiredFields } from './connectionValidation.ts'

const localSchema = [
  { key: 'base_path', field: { type: 'string', required: true } },
  { key: 'include_paths', field: { type: 'array', required: true, items: { type: 'object' } } },
]

const smbSchema = [
  { key: 'host', field: { type: 'string', required: true } },
  { key: 'share', field: { type: 'string', required: true } },
  { key: 'password', field: { type: 'string', required: true, 'x-secret': true } },
  { key: 'domain', field: { type: 'string' } },
]

test('a filesystem connection cannot be created without a folder', () => {
  const missing = missingRequiredFields(localSchema, { include_paths: [{ path: '', recursive: true }] })
  assert.deepEqual(missing.map((entry) => entry.key), ['base_path'])
  assert.match(describeMissingFields(missing), /base path/i)
})

test('whitespace is not a folder', () => {
  const missing = missingRequiredFields(localSchema, { base_path: '   ', include_paths: [{}] })
  assert.deepEqual(missing.map((entry) => entry.key), ['base_path'])
})

test('an empty include list counts as missing', () => {
  const missing = missingRequiredFields(localSchema, { base_path: 'D:/Games', include_paths: [] })
  assert.deepEqual(missing.map((entry) => entry.key), ['include_paths'])
})

test('a fully configured connection has nothing missing', () => {
  const missing = missingRequiredFields(localSchema, {
    base_path: 'D:/Games',
    include_paths: [{ path: '', recursive: true }],
  })
  assert.deepEqual(missing, [])
  assert.equal(describeMissingFields(missing), null)
})

test('optional fields are never demanded', () => {
  const missing = missingRequiredFields(smbSchema, { host: 'nas', share: 'games', password: 'x' })
  assert.deepEqual(missing, [])
})

test('a stored secret is not missing while editing', () => {
  // The form masks it and the server preserves it, so an untouched password
  // must not block saving a label change.
  assert.deepEqual(
    missingRequiredFields(smbSchema, { host: 'nas', share: 'games' }, { editing: true }).map((e) => e.key),
    [],
  )
  // On a new connection it is genuinely required.
  assert.deepEqual(
    missingRequiredFields(smbSchema, { host: 'nas', share: 'games' }).map((e) => e.key),
    ['password'],
  )
})

test('several gaps are listed in one readable sentence', () => {
  const missing = missingRequiredFields(smbSchema, {})
  assert.equal(describeMissingFields(missing), 'Fill in Host, Share and Password before creating this connection.')
})

test('field names read the way the form labels them', () => {
  assert.equal(fieldLabel('base_path'), 'Base Path')
  assert.equal(fieldLabel('include_paths'), 'Include Paths')
  assert.equal(fieldLabel('host'), 'Host')
})

test('a new provider starts with the include row the form already displays', () => {
  // Otherwise the form shows "Path 1" filled in and then refuses to submit
  // because include_paths is "missing".
  const values = initialConfigValues(localSchema)
  assert.deepEqual(values.include_paths, [{ path: '', recursive: true }])
  assert.deepEqual(missingRequiredFields(localSchema, values).map((e) => e.key), ['base_path'])
})

test('declared defaults are seeded', () => {
  const values = initialConfigValues([{ key: 'port', field: { type: 'number', default: 445 } }])
  assert.equal(values.port, 445)
})
