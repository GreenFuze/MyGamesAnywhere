import assert from 'node:assert/strict'
import test from 'node:test'
import { SaveHistoryPresenter } from './saveHistory.ts'

test('conflict evidence names both safe save origins and routes', () => {
  const sides = SaveHistoryPresenter.conflictSides({
    slot_id: 'autosave',
    message: 'Both copies changed',
    remote_manifest_hash: 'hash',
    remote_updated_at: '2026-07-24T12:00:00Z',
    remote_file_count: 2,
    remote_total_size: 4096,
    current_origin: 'Living room PC',
    current_route: 'RetroArch',
    incoming_origin: 'This browser',
    incoming_route: 'EmulatorJS',
  })
  assert.deepEqual(sides, [
    { title: 'Current backup', origin: 'Living room PC', route: 'RetroArch' },
    { title: 'This browser', origin: 'This browser', route: 'EmulatorJS' },
  ])
})

test('history summary uses server acceptance evidence and bounded player-facing size', () => {
  const version = {
    id: 'version-1',
    domain_id: 'domain-1',
    manifest_hash: 'hash',
    origin_label: 'TV2',
    route_label: 'ScummVM',
    accepted_at: '2026-07-24T12:00:00Z',
    reported_at: '2036-07-24T12:00:00Z',
    file_count: 2,
    total_size: 4096,
  }
  assert.match(SaveHistoryPresenter.acceptedAt(version), /^Saved by MGA /)
  assert.match(SaveHistoryPresenter.reportedAt(version), /^Device reported /)
  assert.equal(SaveHistoryPresenter.fileSummary(version), '2 files · 4 KB')
})
