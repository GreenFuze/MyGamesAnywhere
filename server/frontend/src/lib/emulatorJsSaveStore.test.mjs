import assert from 'node:assert/strict'
import test from 'node:test'

import {
  EmulatorJsBrowserSaveStore,
  EmulatorJsSaveTree,
} from '../../public/runtimes/emulatorjs/mga-save-store.js'

class FakeEmulatorFileSystem {
  constructor() {
    this.directories = new Set(['/','/data', '/data/saves'])
    this.files = new Map()
  }

  analyzePath(path) {
    return { exists: this.directories.has(path) || this.files.has(path) }
  }

  isDir(mode) {
    return mode === 'directory'
  }

  stat(path) {
    if (this.directories.has(path)) return { mode: 'directory' }
    if (this.files.has(path)) return { mode: 'file' }
    throw new Error(`Missing path ${path}`)
  }

  readdir(path) {
    if (!this.directories.has(path)) throw new Error(`Missing directory ${path}`)
    const prefix = path === '/' ? '/' : `${path}/`
    const entries = new Set(['.', '..'])
    for (const candidate of [...this.directories, ...this.files.keys()]) {
      if (!candidate.startsWith(prefix) || candidate === path) continue
      const remainder = candidate.slice(prefix.length)
      const entry = remainder.split('/')[0]
      if (entry) entries.add(entry)
    }
    return [...entries]
  }

  mkdir(path) {
    if (this.analyzePath(path).exists) throw new Error(`Path already exists ${path}`)
    const parent = path.slice(0, path.lastIndexOf('/')) || '/'
    if (!this.directories.has(parent)) throw new Error(`Missing parent ${parent}`)
    this.directories.add(path)
  }

  writeFile(path, bytes) {
    const parent = path.slice(0, path.lastIndexOf('/')) || '/'
    if (!this.directories.has(parent)) throw new Error(`Missing parent ${parent}`)
    this.files.set(path, new Uint8Array(bytes))
  }

  readFile(path) {
    const bytes = this.files.get(path)
    if (!bytes) throw new Error(`Missing file ${path}`)
    return new Uint8Array(bytes)
  }

  unlink(path) {
    if (!this.files.delete(path)) throw new Error(`Missing file ${path}`)
  }

  rmdir(path) {
    if (this.readdir(path).some((entry) => entry !== '.' && entry !== '..')) {
      throw new Error(`Directory is not empty ${path}`)
    }
    this.directories.delete(path)
  }
}

function writeNestedFile(fileSystem, path, bytes) {
  EmulatorJsSaveTree.ensureParent(fileSystem, path)
  fileSystem.writeFile(path, new Uint8Array(bytes))
}

test('MAME multi-file NVRAM tree round-trips without flattening paths', () => {
  const source = new FakeEmulatorFileSystem()
  writeNestedFile(source, '/data/saves/mame2003-plus/nvram/ddragon.nv', [1, 2, 3])
  writeNestedFile(source, '/data/saves/mame2003-plus/hi/ddragon.hi', [4, 5])

  const snapshot = EmulatorJsSaveTree.collect(source)
  assert.deepEqual(
    snapshot.map((file) => file.path),
    [
      '/data/saves/mame2003-plus/hi/ddragon.hi',
      '/data/saves/mame2003-plus/nvram/ddragon.nv',
    ],
  )

  const restored = new FakeEmulatorFileSystem()
  writeNestedFile(restored, '/data/saves/foreign-profile.srm', [9])
  EmulatorJsSaveTree.replace(restored, snapshot)

  assert.equal(restored.analyzePath('/data/saves/foreign-profile.srm').exists, false)
  assert.deepEqual(
    [...restored.readFile('/data/saves/mame2003-plus/nvram/ddragon.nv')],
    [1, 2, 3],
  )
  assert.deepEqual(
    [...restored.readFile('/data/saves/mame2003-plus/hi/ddragon.hi')],
    [4, 5],
  )
})

test('console save RAM uses the same bounded adapter', () => {
  const source = new FakeEmulatorFileSystem()
  writeNestedFile(source, '/data/saves/proof.srm', [7, 8, 9])
  const snapshot = EmulatorJsSaveTree.collect(source)

  const restored = new FakeEmulatorFileSystem()
  EmulatorJsSaveTree.replace(restored, snapshot)
  assert.deepEqual([...restored.readFile('/data/saves/proof.srm')], [7, 8, 9])
})

test('empty restore creates a missing save root without recreating its existing parent', () => {
  const fileSystem = new FakeEmulatorFileSystem()
  fileSystem.directories.delete('/data/saves')

  EmulatorJsSaveTree.replace(fileSystem, [])

  assert.equal(fileSystem.analyzePath('/data').exists, true)
  assert.equal(fileSystem.analyzePath('/data/saves').exists, true)
})

test('save imports fail closed before clearing data when a path escapes the managed root', () => {
  const fileSystem = new FakeEmulatorFileSystem()
  writeNestedFile(fileSystem, '/data/saves/current.srm', [1])

  assert.throws(
    () =>
      EmulatorJsSaveTree.replace(fileSystem, [
        { path: '/home/web_user/foreign.srm', base64: 'AQ==' },
      ]),
    /escapes the managed save directory/,
  )
  assert.deepEqual([...fileSystem.readFile('/data/saves/current.srm')], [1])
})

test('browser save keys isolate profile, source copy, core, and slot', () => {
  const first = new EmulatorJsBrowserSaveStore(
    {},
    { ownerProfileId: 'profile-a', sourceGameId: 'source-a', core: 'mame2003_plus' },
  )
  const otherProfile = new EmulatorJsBrowserSaveStore(
    {},
    { ownerProfileId: 'profile-b', sourceGameId: 'source-a', core: 'mame2003_plus' },
  )
  const otherSource = new EmulatorJsBrowserSaveStore(
    {},
    { ownerProfileId: 'profile-a', sourceGameId: 'source-b', core: 'mame2003_plus' },
  )
  const otherCore = new EmulatorJsBrowserSaveStore(
    {},
    { ownerProfileId: 'profile-a', sourceGameId: 'source-a', core: 'fceumm' },
  )

  const keys = new Set([
    first.key('save-ram'),
    first.key('state-1'),
    otherProfile.key('save-ram'),
    otherSource.key('save-ram'),
    otherCore.key('save-ram'),
  ])
  assert.equal(keys.size, 5)
})
