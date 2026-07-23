export type UpdateActionPresentation = {
  primaryLabel: 'Download and apply' | 'Apply'
  secondaryLabel: 'Download only' | 'Redownload'
}

export function resolveUpdateActionPresentation(downloaded: boolean): UpdateActionPresentation {
  if (downloaded) {
    return {
      primaryLabel: 'Apply',
      secondaryLabel: 'Redownload',
    }
  }
  return {
    primaryLabel: 'Download and apply',
    secondaryLabel: 'Download only',
  }
}
