import assert from 'node:assert/strict'
import test from 'node:test'

import { LibraryLoadProgressModel } from './libraryLoadProgress.ts'

test('shows indeterminate progress while the first games load', () => {
  const model = new LibraryLoadProgressModel({
    target: 'Library',
    mode: 'browse',
    loadedCount: 0,
    totalCount: 0,
    isInitialLoading: true,
    hasMore: false,
  })

  assert.equal(model.isVisible, true)
  assert.equal(model.isLoading, true)
  assert.equal(model.title, 'Loading Library')
  assert.equal(model.detail, 'Getting the first games ready…')
  assert.equal(model.percentage, undefined)
})

test('reports meaningful progress while games continue streaming', () => {
  const model = new LibraryLoadProgressModel({
    target: 'Play',
    mode: 'browse',
    loadedCount: 125,
    totalCount: 500,
    isInitialLoading: false,
    hasMore: true,
  })

  assert.equal(model.isVisible, true)
  assert.equal(model.title, 'Loading Play')
  assert.equal(model.detail, 'Checked 125 of 500 library games')
  assert.equal(model.percentage, 25)
  assert.equal(model.toolbarLabel, 'Checking 125 of 500')
})

test('makes unfinished search, filter, and grouping work explicit', () => {
  const base = {
    target: 'Library',
    loadedCount: 100,
    totalCount: 250,
    isInitialLoading: false,
    hasMore: true,
  }

  assert.equal(new LibraryLoadProgressModel({ ...base, mode: 'search' }).title, 'Searching your games')
  assert.equal(new LibraryLoadProgressModel({ ...base, mode: 'filter' }).title, 'Applying your filters')
  assert.equal(new LibraryLoadProgressModel({ ...base, mode: 'group' }).title, 'Building your groups')
})

test('disappears deterministically when the final page is ready', () => {
  const model = new LibraryLoadProgressModel({
    target: 'Library',
    mode: 'group',
    loadedCount: 250,
    totalCount: 250,
    isInitialLoading: false,
    hasMore: false,
  })

  assert.equal(model.isVisible, false)
  assert.equal(model.isLoading, false)
  assert.equal(model.toolbarLabel, '250 games')
})

test('keeps partial-load errors distinct and actionable', () => {
  const model = new LibraryLoadProgressModel({
    target: 'Play',
    mode: 'browse',
    loadedCount: 100,
    totalCount: 300,
    isInitialLoading: false,
    hasMore: true,
    errorMessage: 'connection reset',
  })

  assert.equal(model.isError, true)
  assert.equal(model.isLoading, false)
  assert.equal(model.title, "Couldn't finish loading Play")
  assert.equal(model.detail, 'Checked 100 of 300 library games. Try again to check the rest.')
})
