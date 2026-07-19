export type FolderBrowseLocation = {
  name: string
  browsePath: string
  displayPath: string
  objectId?: string
  selectable: boolean
  locationKind?: string
}

export type BrowsableFolder = {
  location_kind?: string
}

export const SHARED_WITH_ME_BROWSE_TOKEN = 'mga-drive://shared-with-me'
const SHARED_FOLDER_BROWSE_TOKEN_PREFIX = 'mga-drive://folder/'

export function buildSharedFolderBrowseToken(objectId: string, displayPath: string): string {
  return `${SHARED_FOLDER_BROWSE_TOKEN_PREFIX}${objectId}?path=${encodeURIComponent(displayPath)}`
}

export function buildInitialFolderHistory(initialPath: string, objectId?: string): FolderBrowseLocation[] {
  const root: FolderBrowseLocation = {
    name: 'My Drive',
    browsePath: '',
    displayPath: '',
    selectable: true,
  }
  if (objectId) {
    return [root, {
      name: 'Shared with me',
      browsePath: SHARED_WITH_ME_BROWSE_TOKEN,
      displayPath: 'Shared with me',
      selectable: false,
      locationKind: 'shared_with_me',
    }, {
      name: initialPath.split('/').filter(Boolean).at(-1) ?? 'Shared folder',
      browsePath: buildSharedFolderBrowseToken(objectId, initialPath),
      displayPath: initialPath,
      objectId,
      selectable: true,
      locationKind: 'shared_folder',
    }]
  }
  const segments = initialPath.split('/').filter(Boolean)
  if (segments.length === 0) return [root]
  return [root, ...segments.map((segment, index) => {
    const displayPath = segments.slice(0, index + 1).join('/')
    return {
      name: segment,
      browsePath: displayPath,
      displayPath,
      selectable: true,
    }
  })]
}

export function filterBrowsableFolders<T extends BrowsableFolder>(folders: T[], allowSharedLocations: boolean): T[] {
  if (allowSharedLocations) return folders
  return folders.filter((folder) => folder.location_kind !== 'shared_with_me')
}
