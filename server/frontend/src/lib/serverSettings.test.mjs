import assert from 'node:assert/strict'
import test from 'node:test'
import {
  describeReach, formatUptime, orderStorage, otherSettings, settingLabel,
} from './serverSettings.ts'

// The keys the server reports, written out independently of the implementation.
// Reading them from LABELS would make the coverage check below prove nothing.
const REPORTED_KEYS = [
  'LISTEN_IP', 'PORT', 'APP_INSTALL_TYPE', 'DB_PATH', 'MEDIA_ROOT', 'SOURCE_CACHE_ROOT',
  'PLUGINS_DIR', 'FRONTEND_DIST', 'UPDATES_DIR', 'LOG_FILE', 'LOG_MAX_SIZE_MB',
  'LOG_MAX_BACKUPS', 'MEDIA_DOWNLOAD_CONCURRENCY', 'UPDATE_MANIFEST_URL',
]

test('every setting the server reports has words of its own', () => {
  const raw = REPORTED_KEYS.filter((key) => settingLabel(key) === key)
  assert.deepEqual(raw, [], `these keys would reach the screen as raw keys: ${raw.join(', ')}`)
})

test('a key nobody has named is shown as itself rather than invented', () => {
  assert.equal(settingLabel('SOMETHING_NEW'), 'SOMETHING_NEW')
})

test('the loopback address is described as unreachable from other devices', () => {
  for (const address of ['127.0.0.1', 'localhost', '::1', ' 127.0.0.1 ']) {
    const described = describeReach(address)
    assert.equal(described.reach, 'this-computer', `${address} was not treated as loopback`)
    assert.match(described.detail, /cannot connect|cannot reach/)
  }
})

test('listening on every address is described as reachable from the network', () => {
  for (const address of ['0.0.0.0', '::']) {
    assert.equal(describeReach(address).reach, 'whole-network')
  }
})

test('a single routable address is neither of the two special cases', () => {
  const described = describeReach('192.168.1.20')
  assert.equal(described.reach, 'one-address')
  assert.match(described.headline, /192\.168\.1\.20/)
})

test('an empty address still produces something safe to render', () => {
  const described = describeReach('')
  assert.equal(described.reach, 'one-address')
  assert.ok(described.headline.length > 0)
})

test('uptime is reported in the roughest useful unit', () => {
  assert.equal(formatUptime(30), 'Less than a minute')
  assert.equal(formatUptime(60), '1 minute')
  assert.equal(formatUptime(3600), '1 hour')
  assert.equal(formatUptime(60 * 60 * 49), '2 days')
  assert.equal(formatUptime(Number.NaN), 'Unknown')
})

test('storage is ordered by what a person looks for first', () => {
  const ordered = orderStorage([
    { key: 'UPDATES_DIR' }, { key: 'MEDIA_ROOT' }, { key: 'DB_PATH' }, { key: 'MYSTERY' },
  ])
  assert.deepEqual(ordered.map((entry) => entry.key), ['DB_PATH', 'MEDIA_ROOT', 'UPDATES_DIR', 'MYSTERY'])
})

test('the remaining settings exclude everything already shown elsewhere', () => {
  const rest = otherSettings(REPORTED_KEYS.map((key) => ({ key })))
  assert.deepEqual(rest.map((entry) => entry.key), [
    'APP_INSTALL_TYPE', 'LOG_MAX_SIZE_MB', 'LOG_MAX_BACKUPS',
    'MEDIA_DOWNLOAD_CONCURRENCY', 'UPDATE_MANIFEST_URL',
  ])
})
