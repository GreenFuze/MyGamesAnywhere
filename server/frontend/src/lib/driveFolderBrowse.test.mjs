import assert from 'node:assert/strict'
import test from 'node:test'
import { buildInitialFolderHistory, filterBrowsableFolders } from './driveFolderBrowse.ts'

test('My Drive paths retain navigable breadcrumb history', () => {
  const history = buildInitialFolderHistory('Games/Arcade')
  assert.deepEqual(history.map(({ name, browsePath }) => ({ name, browsePath })), [
    { name: 'My Drive', browsePath: '' },
    { name: 'Games', browsePath: 'Games' },
    { name: 'Arcade', browsePath: 'Games/Arcade' },
  ])
})

test('Shared with me is offered only to stable-ID capable source pickers', () => {
  const folders = [
    { name: 'Shared with me', location_kind: 'shared_with_me' },
    { name: 'Games' },
  ]
  assert.deepEqual(filterBrowsableFolders(folders, false), [{ name: 'Games' }])
  assert.deepEqual(filterBrowsableFolders(folders, true), folders)
})

test('stored shared folder opens by stable object ID instead of its friendly path', () => {
  const history = buildInitialFolderHistory('Shared with me/Arcade', 'folder-id')
  assert.equal(history[1].browsePath, 'mga-drive://shared-with-me')
  assert.equal(history[2].browsePath, 'mga-drive://folder/folder-id?path=Shared%20with%20me%2FArcade')
  assert.equal(history[2].objectId, 'folder-id')
})
