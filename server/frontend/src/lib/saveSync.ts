import type { SaveSyncSnapshot, SaveSyncSnapshotFile } from '@/api/client'

export const SAVE_SYNC_SLOT_IDS = ['autosave', 'slot-1', 'slot-2', 'slot-3', 'slot-4', 'slot-5'] as const
export const EMULATORJS_SAVE_RAM_SLOT_ID = 'save-ram'

export type SaveSyncSlotId = (typeof SAVE_SYNC_SLOT_IDS)[number]

export type RuntimeSaveFile = {
  path: string
  base64: string
}

export type RuntimeSaveSnapshot = {
  files: RuntimeSaveFile[]
}

export type RuntimeBridgeCommand =
  | { type: 'export-save-snapshot'; requestId: string }
  | { type: 'import-save-snapshot'; requestId: string; files: RuntimeSaveFile[] }
  | { type: 'native-save-result'; requestId: string; ok: boolean; error?: string }
  | { type: 'native-load-state-result'; requestId: string; ok: boolean; stateBase64?: string; error?: string }
  | { type: 'native-load-ram-result'; requestId: string; ok: boolean; files?: RuntimeSaveFile[]; error?: string }

export type RuntimeBridgeEvent =
  | { type: 'ready'; saveAdapter: boolean; nativeSaveSync?: boolean }
  | { type: 'export-result'; requestId: string; snapshot?: RuntimeSaveSnapshot; error?: string }
  | { type: 'import-result'; requestId: string; ok: boolean; error?: string }
  | { type: 'native-save-state'; requestId: string; slot: string; stateBase64: string }
  | { type: 'native-load-state'; requestId: string; slot: string }
  | { type: 'native-save-ram'; requestId: string; files?: RuntimeSaveFile[]; saveBase64?: string; savePath?: string }
  | { type: 'native-load-ram'; requestId: string }
  | { type: 'runtime-error'; error: string }

export function emulatorJsStateSlotId(slot: string | number | null | undefined): string {
  const numericSlot = Number.parseInt(String(slot ?? '1'), 10)
  if (!Number.isFinite(numericSlot) || numericSlot < 1 || numericSlot > 9) {
    return 'state-1'
  }
  return `state-${numericSlot}`
}

export async function buildSaveSyncSnapshot(params: {
  canonicalGameId: string
  sourceGameId: string
  runtime: string
  slotId: string
  files: RuntimeSaveFile[]
}): Promise<SaveSyncSnapshot> {
  const normalized = normalizeRuntimeFiles(params.files)
  const manifestFiles = await Promise.all(
    normalized.map(async (file) => {
      const bytes = base64ToBytes(file.base64)
      return {
        path: file.path,
        size: bytes.byteLength,
        hash: await hashHex(bytes),
      } satisfies SaveSyncSnapshotFile
    }),
  )

  const archiveBytes = writeStoredZip(normalized)
  return {
    canonical_game_id: params.canonicalGameId,
    source_game_id: params.sourceGameId,
    runtime: params.runtime,
    slot_id: params.slotId,
    files: manifestFiles,
    archive_base64: bytesToBase64(archiveBytes),
  }
}

export function extractRuntimeFilesFromSnapshot(snapshot: SaveSyncSnapshot): RuntimeSaveFile[] {
  const archiveBase64 = snapshot.archive_base64 ?? ''
  if (!archiveBase64) {
    return []
  }
  return readStoredZip(base64ToBytes(archiveBase64))
}

export async function computeLocalSnapshotHash(files: RuntimeSaveFile[]): Promise<string> {
  const normalized = normalizeRuntimeFiles(files)
  const manifest = await Promise.all(
    normalized.map(async (file) => {
      const bytes = base64ToBytes(file.base64)
      return {
        path: file.path,
        size: bytes.byteLength,
        hash: await hashHex(bytes),
      }
    }),
  )
  return hashHex(new TextEncoder().encode(JSON.stringify(manifest)))
}

export function normalizeRuntimeFiles(files: RuntimeSaveFile[]): RuntimeSaveFile[] {
  return [...files]
    .map((file) => ({
      path: file.path.replaceAll('\\', '/').replace(/^\/+/, ''),
      base64: file.base64,
    }))
    .sort((a, b) => a.path.localeCompare(b.path))
}

export function bytesToBase64(bytes: Uint8Array): string {
  let binary = ''
  for (let i = 0; i < bytes.length; i += 1) {
    binary += String.fromCharCode(bytes[i])
  }
  return btoa(binary)
}

export function base64ToBytes(base64: string): Uint8Array {
  const binary = atob(base64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i += 1) {
    bytes[i] = binary.charCodeAt(i)
  }
  return bytes
}

async function hashHex(bytes: Uint8Array): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', bytes)
  return Array.from(new Uint8Array(digest))
    .map((byte) => byte.toString(16).padStart(2, '0'))
    .join('')
}

const crc32Table = (() => {
  const table = new Uint32Array(256)
  for (let i = 0; i < 256; i += 1) {
    let c = i
    for (let j = 0; j < 8; j += 1) {
      c = (c & 1) !== 0 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
    }
    table[i] = c >>> 0
  }
  return table
})()

function crc32(bytes: Uint8Array): number {
  let crc = 0xffffffff
  for (let i = 0; i < bytes.length; i += 1) {
    crc = crc32Table[(crc ^ bytes[i]) & 0xff] ^ (crc >>> 8)
  }
  return (crc ^ 0xffffffff) >>> 0
}

function writeUint16(view: DataView, offset: number, value: number) {
  view.setUint16(offset, value, true)
}

function writeUint32(view: DataView, offset: number, value: number) {
  view.setUint32(offset, value >>> 0, true)
}

function concatBytes(chunks: Uint8Array[]): Uint8Array {
  const total = chunks.reduce((sum, chunk) => sum + chunk.length, 0)
  const out = new Uint8Array(total)
  let offset = 0
  for (const chunk of chunks) {
    out.set(chunk, offset)
    offset += chunk.length
  }
  return out
}

function writeStoredZip(files: RuntimeSaveFile[]): Uint8Array {
  const encoder = new TextEncoder()
  const localChunks: Uint8Array[] = []
  const centralChunks: Uint8Array[] = []
  let offset = 0

  for (const file of files) {
    const nameBytes = encoder.encode(file.path)
    const fileBytes = base64ToBytes(file.base64)
    const crc = crc32(fileBytes)

    const localHeader = new Uint8Array(30 + nameBytes.length)
    const localView = new DataView(localHeader.buffer)
    writeUint32(localView, 0, 0x04034b50)
    writeUint16(localView, 4, 20)
    writeUint16(localView, 6, 0)
    writeUint16(localView, 8, 0)
    writeUint16(localView, 10, 0)
    writeUint16(localView, 12, 0)
    writeUint32(localView, 14, crc)
    writeUint32(localView, 18, fileBytes.length)
    writeUint32(localView, 22, fileBytes.length)
    writeUint16(localView, 26, nameBytes.length)
    writeUint16(localView, 28, 0)
    localHeader.set(nameBytes, 30)
    localChunks.push(localHeader, fileBytes)

    const centralHeader = new Uint8Array(46 + nameBytes.length)
    const centralView = new DataView(centralHeader.buffer)
    writeUint32(centralView, 0, 0x02014b50)
    writeUint16(centralView, 4, 20)
    writeUint16(centralView, 6, 20)
    writeUint16(centralView, 8, 0)
    writeUint16(centralView, 10, 0)
    writeUint16(centralView, 12, 0)
    writeUint16(centralView, 14, 0)
    writeUint32(centralView, 16, crc)
    writeUint32(centralView, 20, fileBytes.length)
    writeUint32(centralView, 24, fileBytes.length)
    writeUint16(centralView, 28, nameBytes.length)
    writeUint16(centralView, 30, 0)
    writeUint16(centralView, 32, 0)
    writeUint16(centralView, 34, 0)
    writeUint16(centralView, 36, 0)
    writeUint32(centralView, 38, 0)
    writeUint32(centralView, 42, offset)
    centralHeader.set(nameBytes, 46)
    centralChunks.push(centralHeader)

    offset += localHeader.length + fileBytes.length
  }

  const centralDirectory = concatBytes(centralChunks)
  const end = new Uint8Array(22)
  const endView = new DataView(end.buffer)
  writeUint32(endView, 0, 0x06054b50)
  writeUint16(endView, 4, 0)
  writeUint16(endView, 6, 0)
  writeUint16(endView, 8, files.length)
  writeUint16(endView, 10, files.length)
  writeUint32(endView, 12, centralDirectory.length)
  writeUint32(endView, 16, offset)
  writeUint16(endView, 20, 0)

  return concatBytes([...localChunks, centralDirectory, end])
}

function readStoredZip(bytes: Uint8Array): RuntimeSaveFile[] {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
  const decoder = new TextDecoder()
  const files: RuntimeSaveFile[] = []
  let offset = 0

  while (offset + 30 <= bytes.length) {
    const signature = view.getUint32(offset, true)
    if (signature !== 0x04034b50) {
      break
    }
    const compression = view.getUint16(offset + 8, true)
    if (compression !== 0) {
      throw new Error('Unsupported save archive compression method.')
    }
    const compressedSize = view.getUint32(offset + 18, true)
    const uncompressedSize = view.getUint32(offset + 22, true)
    const nameLength = view.getUint16(offset + 26, true)
    const extraLength = view.getUint16(offset + 28, true)
    const nameStart = offset + 30
    const dataStart = nameStart + nameLength + extraLength
    const name = decoder.decode(bytes.subarray(nameStart, nameStart + nameLength))
    const fileBytes = bytes.subarray(dataStart, dataStart + compressedSize)
    if (compressedSize !== uncompressedSize) {
      throw new Error('Compressed save archives are not supported.')
    }
    files.push({
      path: name,
      base64: bytesToBase64(fileBytes),
    })
    offset = dataStart + compressedSize
  }

  return normalizeRuntimeFiles(files)
}
