import type { Profile } from '@/api/client'

/** One write the management console can offer. */
export type ManagementCapability =
  | 'profile.create'
  | 'profile.edit'
  | 'profile.delete'
  | 'profile.credentialTicket'
  | 'source.create'
  | 'source.edit'
  | 'source.delete'
  | 'source.authorize'
  | 'source.maintain'
  | 'scan.control'
  | 'game.regroup'
  | 'apiClient.issue'
  | 'apiClient.rotate'
  | 'apiClient.revoke'
  | 'legacyExport.download'

const ADMINISTRATOR_ONLY: ReadonlySet<ManagementCapability> = new Set<ManagementCapability>([
  'profile.create',
  'profile.edit',
  'profile.delete',
  'profile.credentialTicket',
  'source.create',
  'source.edit',
  'source.delete',
  'source.authorize',
  'source.maintain',
  'scan.control',
  'game.regroup',
  'apiClient.issue',
  'apiClient.rotate',
  'apiClient.revoke',
  'legacyExport.download',
])

/**
 * Decides which management writes the selected profile may perform.
 *
 * The server re-checks every one of these, so this exists to explain the
 * boundary in the UI rather than to enforce it. It fails closed: an absent or
 * unrecognized profile is granted nothing.
 */
export class ManagementPolicy {
  readonly #profile: Profile | null

  constructor(profile: Profile | null | undefined) {
    this.#profile = profile ?? null
  }

  get isAdministrator(): boolean {
    return this.#profile?.role === 'admin_player'
  }

  get profileId(): string | null {
    return this.#profile?.id ?? null
  }

  can(capability: ManagementCapability): boolean {
    if (!this.#profile) return false
    if (ADMINISTRATOR_ONLY.has(capability)) return this.isAdministrator
    return true
  }

  /** Explains a denial so the console never hides a capability silently. */
  reasonFor(capability: ManagementCapability): string | null {
    if (this.can(capability)) return null
    if (!this.#profile) return 'Select a profile to manage this server.'
    return 'This action requires an administrator profile.'
  }

  /** True when the selected profile owns the record, which is the boundary for
   * anything a player may manage without an administrator ceremony. */
  owns(record: { profile_id?: string } | null | undefined): boolean {
    if (!this.#profile || !record?.profile_id) return false
    return record.profile_id === this.#profile.id
  }
}

/** What a destructive management action changes and what it deliberately leaves
 * alone. Kept as data so the wording is reviewable and testable. */
export interface DestructiveActionCopy {
  consequences: string[]
  preserves: string[]
}

export const DESTRUCTIVE_ACTIONS: Record<
  'profile.delete' | 'source.delete' | 'apiClient.revoke' | 'source.removeMissing',
  DestructiveActionCopy
> = {
  'profile.delete': {
    consequences: [
      'Remove this profile, its credential, and its sessions.',
      'Revoke the frontend API clients issued to this profile.',
      'Disconnect the source connections owned by this profile.',
    ],
    preserves: [
      'Delete any game files, source content, or media on disk.',
      'Delete saves or save history held by the server.',
      'Touch other profiles or their connections.',
    ],
  },
  'source.delete': {
    consequences: [
      'Disconnect this provider and stop future scans and refreshes.',
      'Remove the stored credentials for this connection.',
      'Remove the library records contributed by this connection.',
    ],
    preserves: [
      'Delete any game files or content on the provider or on disk.',
      'Delete saves, save history, or cached media for other connections.',
      'Affect other profiles or their connections.',
    ],
  },
  'apiClient.revoke': {
    consequences: [
      'Reject every future request that uses this client token.',
      'Disconnect the frontend integration until a new client is issued.',
    ],
    preserves: [
      'Delete any game content, metadata, media, or saves.',
      'Change this profile’s own credentials or its source connections.',
    ],
  },
  'source.removeMissing': {
    consequences: [
      'Remove the library records whose files are no longer present.',
    ],
    preserves: [
      'Delete any file on disk or at the provider.',
      'Remove records whose files are still present.',
    ],
  },
}
