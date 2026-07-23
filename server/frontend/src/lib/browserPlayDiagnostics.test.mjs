import assert from 'node:assert/strict'
import test from 'node:test'
import { BrowserPlayIssueResolver } from './browserPlayDiagnostics.ts'

test('missing root explains when only a save file remains and links to the owning connection', () => {
  const issue = new BrowserPlayIssueResolver('Eye of the Beholder', {
    integrationId: 'drive-1',
    integrationLabel: 'Family games',
    rawTitle: 'ad&d - eye of the beholder (u)',
    rootPath: 'Games/SNES',
    files: [{ path: 'Games/SNES/AD&D - Eye of the Beholder (U).srm' }],
  }).missingRootFile('EmulatorJS')

  assert.equal(issue.code, 'missing_root_file')
  assert.equal(issue.title, 'Playable game file is missing')
  assert.match(issue.message, /only save or support files/)
  assert.match(issue.message, /Restore the ROM or game archive/)
  assert.deepEqual(issue.action, {
    label: 'Open connection',
    href: '/settings?tab=connections&integration=drive-1',
  })
})

test('missing declared root is diagnosed as stale connection data', () => {
  const issue = new BrowserPlayIssueResolver('A Game', {
    integrationId: 'local-1',
    integrationLabel: 'Local games',
    files: [{ path: 'Games/A Game/game.zip' }],
  }).missingRootFile('EmulatorJS', 'stale-file-id')

  assert.equal(issue.title, 'Connection data is out of date')
  assert.match(issue.message, /Rescan Local games/)
})

test('unrecognized files produce a general playable-file repair', () => {
  const issue = new BrowserPlayIssueResolver('A Game', {
    files: [{ path: 'Games/A Game/readme.txt' }],
  }).missingRootFile('js-dos')

  assert.equal(issue.title, 'Playable game file not selected')
  assert.match(issue.message, /ROM, archive, disc image, or executable/)
  assert.equal(issue.action?.href, '/settings?tab=connections')
})
