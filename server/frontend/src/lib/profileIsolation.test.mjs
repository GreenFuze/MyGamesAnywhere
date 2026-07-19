import assert from 'node:assert/strict'
import test from 'node:test'
import { discardAmbiguousLegacyProfileStorage, profileStorageKey } from './profileStorage.ts'

class MemoryStorage {
  #values = new Map()
  get length() { return this.#values.size }
  getItem(key) { return this.#values.get(key) ?? null }
  setItem(key, value) { this.#values.set(String(key), String(value)) }
  removeItem(key) { this.#values.delete(key) }
  key(index) { return [...this.#values.keys()][index] ?? null }
}

function withWindow(fn) {
  const prior = globalThis.window
  const localStorage = new MemoryStorage()
  const sessionStorage = new MemoryStorage()
  globalThis.window = { localStorage, sessionStorage }
  try { fn({ localStorage, sessionStorage }) } finally { globalThis.window = prior }
}

test('profile storage keys cannot collide across profiles', () => withWindow(({ localStorage }) => {
  localStorage.setItem('mga.selectedProfileId', 'profile-a')
  const a = profileStorageKey('libraryPrefs')
  localStorage.setItem('mga.selectedProfileId', 'profile-b')
  const b = profileStorageKey('libraryPrefs')
  assert.notEqual(a, b)
  assert.match(a, /^mga\.profile\.v2\.profile-a\./)
  assert.match(b, /^mga\.profile\.v2\.profile-b\./)
}))

test('upgrade discards ambiguous player state but preserves browser-global appearance', () => withWindow(({ localStorage, sessionStorage }) => {
  localStorage.setItem('mga.libraryPrefs', 'foreign library')
  localStorage.setItem('mga.browserPlaySource.game-a', 'foreign source')
  sessionStorage.setItem('mga.browserPlaySession.token', 'foreign play session')
  localStorage.setItem('mga.themeId', 'global theme')
  localStorage.setItem('mga.dateTimeFormat', 'global locale')

  discardAmbiguousLegacyProfileStorage()

  assert.equal(localStorage.getItem('mga.libraryPrefs'), null)
  assert.equal(localStorage.getItem('mga.browserPlaySource.game-a'), null)
  assert.equal(sessionStorage.getItem('mga.browserPlaySession.token'), null)
  assert.equal(localStorage.getItem('mga.themeId'), 'global theme')
  assert.equal(localStorage.getItem('mga.dateTimeFormat'), 'global locale')
}))
