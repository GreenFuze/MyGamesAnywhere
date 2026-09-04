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

test('game source begins at provider root with My Drive and Shared with me as peers', () => {
  const history = buildInitialFolderHistory('', undefined, true)
  assert.deepEqual(history, [{
    name: 'Google Drive',
    browsePath: 'mga-drive://locations',
    displayPath: '',
    selectable: false,
  }])
})

test('existing My Drive game source keeps provider and My Drive breadcrumbs', () => {
  const history = buildInitialFolderHistory('Games/Arcade', undefined, true)
  assert.deepEqual(history.map(({ name, browsePath }) => ({ name, browsePath })), [
    { name: 'Google Drive', browsePath: 'mga-drive://locations' },
    { name: 'My Drive', browsePath: 'mga-drive://my-drive' },
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
  const history = buildInitialFolderHistory('Shared with me/Arcade', 'folder-id', true)
  assert.equal(history[0].name, 'Google Drive')
  assert.equal(history[1].browsePath, 'mga-drive://shared-with-me')
  assert.equal(history[2].browsePath, 'mga-drive://folder/folder-id?path=Shared%20with%20me%2FArcade')
  assert.equal(history[2].objectId, 'folder-id')
})

test('a POSIX absolute path keeps its leading slash', () => {
  // split('/').filter(Boolean) drops the leading empty segment, which turned
  // "/mnt/games" into the relative "mnt/games" and browsed the wrong folder.
  const history = buildInitialFolderHistory('/mnt/games', undefined, false, 'Computer')
  assert.deepEqual(history.map(({ name, browsePath }) => ({ name, browsePath })), [
    { name: 'Computer', browsePath: '' },
    { name: 'mnt', browsePath: '/mnt' },
    { name: 'games', browsePath: '/mnt/games' },
  ])
})

test('a Windows path browses by drive', () => {
  const history = buildInitialFolderHistory('C:/Games/SNES', undefined, false, 'Computer')
  assert.deepEqual(history.map(({ browsePath }) => browsePath), ['', 'C:', 'C:/Games', 'C:/Games/SNES'])
})

test('the root breadcrumb keeps its Drive name by default', () => {
  // Every existing caller relies on this default.
  assert.equal(buildInitialFolderHistory('Games')[0].name, 'My Drive')
})
