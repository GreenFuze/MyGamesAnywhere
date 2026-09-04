import assert from 'node:assert/strict'
import test from 'node:test'
import { describeConnection, describeRefreshProgress, describeScanProgress } from './scanProgress.ts'

const runningJob = {
  job_id: 'job-1',
  status: 'running',
  trigger: 'manual',
  metadata_only: false,
  integration_ids: [],
  integration_count: 2,
  integrations_completed: 1,
  current_phase: 'listing source content',
  current_integration_label: 'Xbox',
  integrations: [
    { integration_id: 'a', label: 'Local Folder', plugin_id: 'game-source-local', status: 'completed', games_found: 3 },
    { integration_id: 'b', label: 'Xbox', plugin_id: 'game-source-xbox', status: 'running', phase: 'listing source content' },
  ],
}

test('an operator can see how far along the scan is', () => {
  const view = describeScanProgress(runningJob)
  assert.equal(view.headline, 'Scanning Xbox')
  assert.equal(view.overall.value, 50)
  assert.equal(view.overall.label, '1 of 2 connections')
  assert.equal(view.gamesFound, 3)
  assert.equal(view.finished, false)
})

test('a storefront that reports nothing still shows as working, named', () => {
  // Xbox makes one blocking call that can take minutes and publishes no
  // progress inside it. A dead panel there is indistinguishable from a hang,
  // so the row must still say which connection is busy.
  const xbox = describeConnection(runningJob.integrations[1])
  assert.equal(xbox.tone, 'running')
  assert.ok(xbox.bar, 'a running connection must show a bar')
  assert.equal(xbox.bar.value, undefined, 'unknown totals must read as indeterminate, not 0%')
  assert.match(xbox.bar.label, /Xbox/)
})

test('a finished connection reports its game count and stops spinning', () => {
  const local = describeConnection(runningJob.integrations[0])
  assert.equal(local.tone, 'done')
  assert.equal(local.gamesFound, 3)
  assert.equal(local.bar, null)
  // The tick and the count already say it finished.
  assert.equal(local.detail, null)
})

test('a determinate phase reports real numbers', () => {
  const view = describeConnection({
    integration_id: 'c',
    label: 'SMB File Share',
    status: 'running',
    phase: 'scanning files',
    source_progress: { current: 25, total: 100, unit: 'files' },
  })
  assert.equal(view.bar.value, 25)
  assert.equal(view.bar.label, '25 of 100 files')
})

test('metadata progress wins over source progress once matching starts', () => {
  // Listing is finished by then, so the stale source numbers would only
  // mislead.
  const view = describeConnection({
    integration_id: 'd',
    label: 'Steam',
    status: 'running',
    source_progress: { current: 100, total: 100, unit: 'files' },
    metadata_phase: 'identify',
    metadata_progress: { current: 10, total: 40, unit: 'games' },
  })
  assert.equal(view.bar.label, '10 of 40 games')
})

test('the game being worked on is surfaced', () => {
  const view = describeScanProgress({
    ...runningJob,
    recent_events: [
      { type: 'scan_metadata_game_progress', data: { game_title: 'Halo' } },
      { type: 'scan_metadata_game_progress', data: { game_title: 'Forza Horizon 5' } },
      { type: 'scan_scanner_progress', data: {} },
    ],
  })
  assert.equal(view.currentItem, 'Forza Horizon 5')
})

test('a skipped connection explains itself instead of showing a phase', () => {
  const view = describeConnection({
    integration_id: 'e',
    label: 'Xbox',
    status: 'skipped',
    reason: 'auth_required',
  })
  assert.equal(view.tone, 'skipped')
  assert.match(view.detail, /sign in/i)
})

test('a failed connection shows the error, not a phase', () => {
  const view = describeConnection({
    integration_id: 'f',
    label: 'SMB File Share',
    status: 'failed',
    phase: 'listing source content',
    error: 'dial tcp: connection refused',
  })
  assert.equal(view.tone, 'failed')
  assert.equal(view.detail, 'dial tcp: connection refused')
})

test('a completed scan reports what it found', () => {
  const view = describeScanProgress({ ...runningJob, status: 'completed', integrations_completed: 2 })
  assert.equal(view.finished, true)
  assert.equal(view.overall.value, 100)
  assert.match(view.headline, /3 games found/)
  // The bar must not restate the headline sitting directly above it.
  assert.notEqual(view.overall.label, view.headline)
  assert.equal(view.overall.label, '2 connections')
  assert.equal(view.currentItem, null, 'a finished scan is not working on anything')
})

test('a cancelled or failed scan says so rather than claiming success', () => {
  assert.match(describeScanProgress({ ...runningJob, status: 'cancelled' }).headline, /cancelled/i)
  assert.match(
    describeScanProgress({ ...runningJob, status: 'failed', error: 'provider unreachable' }).headline,
    /provider unreachable/,
  )
})

test('a job with no connections yet does not claim a percentage', () => {
  const view = describeScanProgress({
    ...runningJob,
    status: 'queued',
    integration_count: 0,
    integrations_completed: 0,
    integrations: [],
    current_phase: undefined,
    current_integration_label: undefined,
  })
  assert.equal(view.overall.value, undefined)
  assert.equal(view.headline, 'Starting the scan')
})

test('phase wording never doubles up when no connection is named yet', () => {
  const view = describeScanProgress({ ...runningJob, current_integration_label: undefined })
  assert.equal(view.headline, 'Reading your library…')
})

test('a connection refresh reports the item it is on', () => {
  const view = describeRefreshProgress({
    job_id: 'r1',
    integration_id: 'a',
    plugin_id: 'metadata-hltb',
    label: 'HowLongToBeat',
    status: 'running',
    phase: 'refreshing_metadata',
    items_total: 40,
    items_completed: 12,
    warning_count: 0,
    error_count: 0,
    current_item: 'Hollow Knight',
  })
  assert.equal(view.headline, 'Refreshing metadata…')
  assert.equal(view.currentItem, 'Hollow Knight')
  assert.equal(view.bar.value, 30)
  assert.equal(view.bar.label, '12 of 40 games')
  assert.equal(view.finished, false)
})

test('a finished refresh says how much it covered', () => {
  const view = describeRefreshProgress({
    job_id: 'r1', integration_id: 'a', plugin_id: 'p', label: 'L',
    status: 'completed', items_total: 40, items_completed: 40, warning_count: 0, error_count: 0,
    current_item: 'Hollow Knight',
  })
  assert.match(view.headline, /Refreshed 40 games/)
  assert.equal(view.currentItem, null)
  assert.equal(view.finished, true)
})

test('a failed refresh shows the error rather than a count', () => {
  const view = describeRefreshProgress({
    job_id: 'r1', integration_id: 'a', plugin_id: 'p', label: 'L',
    status: 'failed', items_total: 0, items_completed: 0, warning_count: 0, error_count: 1,
    error: 'provider rejected the request',
  })
  assert.equal(view.failed, true)
  assert.equal(view.headline, 'provider rejected the request')
})

test('an unbounded walk still shows how much it has covered', () => {
  // The count is the clearest evidence a walk is moving; dropping it because
  // there is no total leaves the operator with a bar that never changes.
  const view = describeConnection({
    integration_id: 'g',
    label: 'Local Folder',
    status: 'running',
    phase: 'Reading Roms/Bulk…',
    source_progress: { current: 1457, unit: 'items', indeterminate: true },
  })
  assert.equal(view.bar.value, undefined)
  assert.match(view.bar.label, /1457 items so far/)
  assert.match(view.bar.label, /Reading Roms\/Bulk/)
})
