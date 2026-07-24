import type { SaveDomainHistoryVersion, SaveSyncConflict } from '@/api/client'

export type SaveConflictSide = {
  title: string
  origin: string
  route: string
}

export class SaveHistoryPresenter {
  static conflictSides(conflict: SaveSyncConflict): SaveConflictSide[] {
    return [
      {
        title: 'Current backup',
        origin: conflict.current_origin?.trim() || 'Earlier MGA save',
        route: conflict.current_route?.trim() || 'Unknown play route',
      },
      {
        title: 'This browser',
        origin: conflict.incoming_origin?.trim() || 'This browser',
        route: conflict.incoming_route?.trim() || 'Current browser route',
      },
    ]
  }

  static acceptedAt(version: SaveDomainHistoryVersion): string {
    const value = new Date(version.accepted_at)
    return Number.isNaN(value.getTime())
      ? 'Saved by MGA at an unknown time'
      : `Saved by MGA ${value.toLocaleString()}`
  }

  static reportedAt(version: SaveDomainHistoryVersion): string | null {
    if (!version.reported_at) return null
    const value = new Date(version.reported_at)
    return Number.isNaN(value.getTime()) ? null : `Device reported ${value.toLocaleString()}`
  }

  static fileSummary(version: SaveDomainHistoryVersion): string {
    const files = `${version.file_count} ${version.file_count === 1 ? 'file' : 'files'}`
    return `${files} · ${this.formatBytes(version.total_size)}`
  }

  static formatBytes(value: number): string {
    if (!Number.isFinite(value) || value <= 0) return '0 B'
    const units = ['B', 'KB', 'MB', 'GB', 'TB']
    const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
    const precision = index >= 3 ? 1 : 0
    return `${(value / 1024 ** index).toFixed(precision)} ${units[index]}`
  }
}
