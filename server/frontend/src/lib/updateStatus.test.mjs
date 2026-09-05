import assert from 'node:assert/strict'
import test from 'node:test'
import { describeUpdate, formatBytes, shortDigest } from './updateStatus.ts'

// The update service reports more situations than "up to date" and "update
// available", and the two that matter most are the ones easiest to get wrong:
// a check that could not reach the manifest, and a download that finished and
// is waiting for a decision. Reporting either as "up to date" would be a lie
// that stops someone updating.

test('an unreachable update service is not reported as up to date', () => {
  // No latest_version means the check failed, not that this is the newest
  // release. Conflating them is how a server sits unpatched while its console
  // says everything is fine.
  const summary = describeUpdate({ current_version: '0.2.17', update_available: false, install_type: 'portable' })
  assert.equal(summary.pill, 'Cannot check')
  assert.equal(summary.tone, 'neutral')
  assert.match(summary.detail, /could not reach/)
  assert.equal(summary.canDownload, false)
  assert.equal(summary.readyToApply, false)
})

test('up to date is only claimed when a latest version was actually seen', () => {
  const summary = describeUpdate({
    current_version: '0.2.17', latest_version: '0.2.17', update_available: false, install_type: 'portable',
  })
  assert.equal(summary.pill, 'Up to date')
  assert.equal(summary.tone, 'good')
  assert.equal(summary.canDownload, false)
})

test('an available update offers a download and says it changes nothing yet', () => {
  const summary = describeUpdate({
    current_version: '0.2.17', latest_version: '0.3.0', update_available: true, install_type: 'portable',
  })
  assert.equal(summary.canDownload, true)
  assert.equal(summary.readyToApply, false)
  assert.match(summary.headline, /0\.3\.0/)
  assert.match(summary.detail, /does not change anything yet/)
})

test('a finished download offers to install and warns that it restarts', () => {
  const summary = describeUpdate({
    current_version: '0.2.17', latest_version: '0.3.0', update_available: true, install_type: 'portable',
    downloaded_path: 'C:/tmp/mga.zip', downloaded_sha256: 'abc', downloaded_size: 1024,
  })
  assert.equal(summary.readyToApply, true)
  assert.equal(summary.canDownload, false, 'downloading twice is not a useful offer')
  assert.match(summary.detail, /restarts/)
})

test('a download in progress offers nothing to press', () => {
  const summary = describeUpdate({
    current_version: '0.2.17', latest_version: '0.3.0', update_available: true, install_type: 'portable',
    download_in_progress: true, download_percent: 40,
  })
  assert.equal(summary.canDownload, false)
  assert.equal(summary.readyToApply, false)
  assert.match(summary.detail, /Nothing changes on this server until you install it/)
})

test('an install under way outranks every other state', () => {
  // Offering buttons here invites a second install on top of a restarting
  // server, so apply_started has to win even though the other fields still
  // describe an available, downloaded update.
  const summary = describeUpdate({
    current_version: '0.2.17', latest_version: '0.3.0', update_available: true, install_type: 'portable',
    downloaded_path: 'C:/tmp/mga.zip', apply_started: true,
  })
  assert.equal(summary.pill, 'Installing')
  assert.equal(summary.canDownload, false)
  assert.equal(summary.readyToApply, false)
})

test('an unloaded status says it is checking rather than guessing', () => {
  const summary = describeUpdate(undefined)
  assert.equal(summary.pill, 'Checking')
  assert.equal(summary.canDownload, false)
  assert.equal(summary.readyToApply, false)
})

test('sizes and checksums are readable', () => {
  assert.equal(formatBytes(0), '0 B')
  assert.equal(formatBytes(undefined), '0 B')
  assert.equal(formatBytes(512), '512 B')
  assert.equal(formatBytes(1536), '1.5 KB')
  assert.equal(formatBytes(20 * 1024 * 1024), '20 MB')

  // Enough to compare against a release page without a wall of hex.
  assert.equal(shortDigest(undefined), 'not recorded')
  assert.equal(shortDigest('short'), 'short')
  // Spelled out rather than recomputed. An expectation built the same way as
  // the implementation passes however wrong both of them are.
  assert.equal(
    shortDigest('cb425aed0b1c2d3e4f5a6b7c8d9e0f1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e'),
    'cb425aed…4b5c6d7e',
  )
})
