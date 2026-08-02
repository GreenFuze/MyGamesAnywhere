const DATABASE_NAME = 'mga-emulatorjs-saves'
const DATABASE_VERSION = 1
const SNAPSHOT_STORE = 'snapshots'
const SAVE_ROOT = '/data/saves'
const MAX_FILES = 128
const MAX_PATH_LENGTH = 512
const MAX_FILE_BYTES = 64 * 1024 * 1024
const MAX_TOTAL_BYTES = 128 * 1024 * 1024

function assertBoundedIdentity(value, label) {
  const normalized = String(value || '').trim()
  if (!normalized || normalized.length > 256 || /[\u0000-\u001f\u007f]/.test(normalized)) {
    throw new Error(`${label} is invalid.`)
  }
  return normalized
}

function bytesToBase64(bytes) {
  let binary = ''
  for (let index = 0; index < bytes.length; index += 1) {
    binary += String.fromCharCode(bytes[index])
  }
  return btoa(binary)
}

function base64ToBytes(base64) {
  const normalized = String(base64 || '').trim()
  if (!normalized || normalized.length > Math.ceil(MAX_FILE_BYTES / 3) * 4 + 8) {
    throw new Error('Browser save file data is empty or too large.')
  }
  let binary
  try {
    binary = atob(normalized)
  } catch {
    throw new Error('Browser save file data is not valid base64.')
  }
  if (binary.length === 0 || binary.length > MAX_FILE_BYTES) {
    throw new Error('Browser save file data is empty or too large.')
  }
  const bytes = new Uint8Array(binary.length)
  for (let index = 0; index < binary.length; index += 1) {
    bytes[index] = binary.charCodeAt(index)
  }
  return bytes
}

function normalizedRelativePath(path) {
  const original = String(path || '').trim().replaceAll('\\', '/')
  if (!original || original.length > MAX_PATH_LENGTH || /[\u0000-\u001f\u007f]/.test(original)) {
    throw new Error('Browser save path is invalid.')
  }

  let relative = original
  if (relative.startsWith('/data/saves/')) {
    relative = relative.slice('/data/saves/'.length)
  } else if (relative === '/data/saves' || relative === 'data/saves') {
    throw new Error('Browser save path must identify a file.')
  } else if (relative.startsWith('data/saves/')) {
    relative = relative.slice('data/saves/'.length)
  } else if (relative.startsWith('/')) {
    throw new Error('Browser save path escapes the managed save directory.')
  }

  const parts = relative.split('/')
  if (
    parts.length === 0 ||
    parts.some((part) => !part || part === '.' || part === '..' || part.length > 255)
  ) {
    throw new Error('Browser save path is invalid.')
  }
  return parts.join('/')
}

function validateSlotID(slotID) {
  const normalized = String(slotID || '').trim()
  if (normalized !== 'save-ram' && !/^state-[1-9]$/.test(normalized)) {
    throw new Error('Browser save slot is invalid.')
  }
  return normalized
}

function cloneFiles(files) {
  return files.map((file) => ({ path: file.path, base64: file.base64 }))
}

function nextRevision() {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`
}

function requestResult(request, fallbackMessage) {
  return new Promise((resolve, reject) => {
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error || new Error(fallbackMessage))
  })
}

export class EmulatorJsSaveTree {
  static normalizePath(path) {
    return `${SAVE_ROOT}/${normalizedRelativePath(path)}`
  }

  static normalizeFiles(files) {
    if (!Array.isArray(files) || files.length === 0 || files.length > MAX_FILES) {
      throw new Error('Browser save snapshot is empty or contains too many files.')
    }

    const normalized = []
    const seen = new Set()
    let totalBytes = 0
    for (const file of files) {
      const path = this.normalizePath(file?.path)
      if (seen.has(path)) {
        throw new Error(`Browser save snapshot contains duplicate path ${path}.`)
      }
      const bytes = base64ToBytes(file?.base64)
      totalBytes += bytes.byteLength
      if (totalBytes > MAX_TOTAL_BYTES) {
        throw new Error('Browser save snapshot is too large.')
      }
      seen.add(path)
      normalized.push({ path, base64: bytesToBase64(bytes) })
    }
    normalized.sort((left, right) => left.path.localeCompare(right.path))
    return normalized
  }

  static collect(fileSystem, primaryPath = null, primaryBytes = null) {
    if (!fileSystem) throw new Error('EmulatorJS save filesystem is unavailable.')
    const files = []
    const seen = new Set()

    const addFile = (path, bytes) => {
      if (!bytes || bytes.byteLength === 0) return
      const normalizedPath = this.normalizePath(path)
      if (seen.has(normalizedPath)) return
      seen.add(normalizedPath)
      files.push({ path: normalizedPath, base64: bytesToBase64(bytes) })
    }

    if (primaryBytes) {
      addFile(primaryPath || 'save.ram', primaryBytes)
    }

    const pathExists = (path) => Boolean(fileSystem.analyzePath(path).exists)
    const isDirectory = (path) => {
      if (!pathExists(path)) return false
      return fileSystem.isDir(fileSystem.stat(path).mode)
    }
    const visit = (path) => {
      if (!pathExists(path) || !isDirectory(path)) return
      for (const entry of fileSystem.readdir(path)) {
        if (entry === '.' || entry === '..') continue
        const childPath = `${path.replace(/\/$/, '')}/${entry}`
        if (isDirectory(childPath)) {
          visit(childPath)
        } else {
          try {
            addFile(childPath, fileSystem.readFile(childPath))
          } catch {
            // A core may rotate a file while the bounded tree is being captured.
          }
        }
      }
    }
    visit(SAVE_ROOT)

    if (files.length === 0) return []
    return this.normalizeFiles(files)
  }

  static replace(fileSystem, files) {
    if (!fileSystem) throw new Error('EmulatorJS save filesystem is unavailable.')
    const normalized = Array.isArray(files) && files.length > 0 ? this.normalizeFiles(files) : []
    this.clear(fileSystem)
    for (const file of normalized) {
      this.ensureParent(fileSystem, file.path)
      fileSystem.writeFile(file.path, base64ToBytes(file.base64))
    }
    return cloneFiles(normalized)
  }

  static clear(fileSystem) {
    if (!fileSystem.analyzePath(SAVE_ROOT).exists) {
      if (!fileSystem.analyzePath('/data').exists) {
        fileSystem.mkdir('/data')
      }
      fileSystem.mkdir(SAVE_ROOT)
      return
    }

    const clearDirectory = (path) => {
      for (const entry of fileSystem.readdir(path)) {
        if (entry === '.' || entry === '..') continue
        const childPath = `${path.replace(/\/$/, '')}/${entry}`
        if (fileSystem.isDir(fileSystem.stat(childPath).mode)) {
          clearDirectory(childPath)
          fileSystem.rmdir(childPath)
        } else {
          fileSystem.unlink(childPath)
        }
      }
    }
    clearDirectory(SAVE_ROOT)
  }

  static ensureParent(fileSystem, path) {
    const parts = path.split('/')
    let current = ''
    for (let index = 0; index < parts.length - 1; index += 1) {
      const part = parts[index]
      if (!part) continue
      current += `/${part}`
      if (!fileSystem.analyzePath(current).exists) {
        fileSystem.mkdir(current)
      }
    }
  }
}

export class EmulatorJsBrowserSaveStore {
  constructor(database, route) {
    this.database = database
    this.route = {
      ownerProfileId: assertBoundedIdentity(route?.ownerProfileId, 'Browser save profile'),
      sourceGameId: assertBoundedIdentity(route?.sourceGameId, 'Browser save source'),
      core: assertBoundedIdentity(route?.core, 'Browser save core'),
    }
  }

  static async open(route) {
    if (typeof indexedDB === 'undefined') {
      throw new Error('This browser does not provide IndexedDB for local game saves.')
    }
    const request = indexedDB.open(DATABASE_NAME, DATABASE_VERSION)
    request.onupgradeneeded = () => {
      const database = request.result
      if (!database.objectStoreNames.contains(SNAPSHOT_STORE)) {
        database.createObjectStore(SNAPSHOT_STORE, { keyPath: 'key' })
      }
    }
    const database = await requestResult(request, 'Unable to open the browser save database.')
    return new EmulatorJsBrowserSaveStore(database, route)
  }

  key(slotID) {
    return [
      'v1',
      encodeURIComponent(this.route.ownerProfileId),
      encodeURIComponent(this.route.sourceGameId),
      'emulatorjs',
      encodeURIComponent(this.route.core),
      validateSlotID(slotID),
    ].join('|')
  }

  async get(slotID) {
    const key = this.key(slotID)
    const transaction = this.database.transaction(SNAPSHOT_STORE, 'readonly')
    const record = await requestResult(
      transaction.objectStore(SNAPSHOT_STORE).get(key),
      'Unable to read the browser save.',
    )
    if (!record) return null
    return {
      ...record,
      files: cloneFiles(EmulatorJsSaveTree.normalizeFiles(record.files)),
    }
  }

  async put(slotID, files, options = {}) {
    const normalizedSlotID = validateSlotID(slotID)
    const normalizedFiles = EmulatorJsSaveTree.normalizeFiles(files)
    const previous = await this.get(normalizedSlotID)
    const revision = nextRevision()
    const record = {
      key: this.key(normalizedSlotID),
      schemaVersion: 1,
      ownerProfileId: this.route.ownerProfileId,
      sourceGameId: this.route.sourceGameId,
      runtime: 'emulatorjs',
      core: this.route.core,
      slotId: normalizedSlotID,
      revision,
      updatedAt: new Date().toISOString(),
      lastSyncedManifestHash:
        typeof options.syncedManifestHash === 'string'
          ? options.syncedManifestHash
          : previous?.lastSyncedManifestHash || '',
      files: cloneFiles(normalizedFiles),
    }
    const transaction = this.database.transaction(SNAPSHOT_STORE, 'readwrite')
    await requestResult(
      transaction.objectStore(SNAPSHOT_STORE).put(record),
      'Unable to persist the browser save.',
    )
    return { ...record, files: cloneFiles(record.files) }
  }

  async markSynced(slotID, revision, manifestHash) {
    const key = this.key(slotID)
    const boundedManifestHash = assertBoundedIdentity(manifestHash, 'Save backup manifest')
    const transaction = this.database.transaction(SNAPSHOT_STORE, 'readwrite')
    const store = transaction.objectStore(SNAPSHOT_STORE)
    const record = await requestResult(store.get(key), 'Unable to read the browser save.')
    if (!record || record.revision !== revision) return false
    record.lastSyncedManifestHash = boundedManifestHash
    await requestResult(store.put(record), 'Unable to update browser save backup state.')
    return true
  }
}
