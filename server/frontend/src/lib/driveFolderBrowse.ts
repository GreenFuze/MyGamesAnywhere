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
export const DRIVE_LOCATIONS_BROWSE_TOKEN = 'mga-drive://locations'
export const MY_DRIVE_BROWSE_TOKEN = 'mga-drive://my-drive'
const SHARED_FOLDER_BROWSE_TOKEN_PREFIX = 'mga-drive://folder/'

export function buildSharedFolderBrowseToken(objectId: string, displayPath: string): string {
  return `${SHARED_FOLDER_BROWSE_TOKEN_PREFIX}${objectId}?path=${encodeURIComponent(displayPath)}`
}

export function buildInitialFolderHistory(
  initialPath: string,
  objectId?: string,
  showDriveLocations = false,
): FolderBrowseLocation[] {
  const myDriveRoot: FolderBrowseLocation = {
    name: 'My Drive',
    browsePath: '',
    displayPath: '',
    selectable: true,
  }
  if (!showDriveLocations) {
    const segments = initialPath.split('/').filter(Boolean)
    if (segments.length === 0) return [myDriveRoot]
    return [myDriveRoot, ...segments.map((segment, index) => {
      const displayPath = segments.slice(0, index + 1).join('/')
      return {
        name: segment,
        browsePath: displayPath,
        displayPath,
        selectable: true,
      }
    })]
  }

  const providerRoot: FolderBrowseLocation = {
    name: 'Google Drive',
    browsePath: DRIVE_LOCATIONS_BROWSE_TOKEN,
    displayPath: '',
    selectable: false,
  }
  if (objectId) {
    return [providerRoot, {
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
  if (segments.length === 0) return [providerRoot]
  const sourceMyDriveRoot: FolderBrowseLocation = {
    ...myDriveRoot,
    browsePath: MY_DRIVE_BROWSE_TOKEN,
    locationKind: 'my_drive',
  }
  return [providerRoot, sourceMyDriveRoot, ...segments.map((segment, index) => {
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
