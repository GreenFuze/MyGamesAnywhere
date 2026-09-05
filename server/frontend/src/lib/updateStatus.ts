import type { UpdateStatus } from '@/api/client'

/**
 * Turning the update service's fields into one sentence a person can act on.
 *
 * Kept apart from the component so the states can be tested directly. There are
 * more of them than the happy path suggests: no manifest reachable, an update
 * available, a download half finished, a download verified and waiting, and an
 * install already under way.
 */

export type UpdateSummary = {
  headline: string
  detail: string
  pill: string
  tone: 'neutral' | 'good' | 'attention'
  canDownload: boolean
  readyToApply: boolean
}

export function describeUpdate(status: UpdateStatus | undefined): UpdateSummary {
  if (!status) {
    return {
      headline: 'Checking this server',
      detail: 'Reading the current version.',
      pill: 'Checking',
      tone: 'neutral',
      canDownload: false,
      readyToApply: false,
    }
  }

  const current = status.current_version || 'unknown'

  // An install already under way outranks everything else: nothing else the
  // user could do now is useful, and offering buttons would invite a second one.
  if (status.apply_started) {
    return {
      headline: 'Installing',
      detail: 'The server is restarting on the new version. This page will reconnect on its own.',
      pill: 'Installing',
      tone: 'attention',
      canDownload: false,
      readyToApply: false,
    }
  }

  if (status.download_in_progress) {
    return {
      headline: `Downloading ${status.latest_version ?? 'the update'}`,
      detail: 'Nothing changes on this server until you install it.',
      pill: 'Downloading',
      tone: 'attention',
      canDownload: false,
      readyToApply: false,
    }
  }

  // Downloaded and checksummed, waiting for a decision.
  if (status.downloaded_path && status.update_available) {
    return {
      headline: `${status.latest_version ?? 'An update'} is ready to install`,
      detail: `This server is on ${current}. Installing restarts it.`,
      pill: 'Ready to install',
      tone: 'attention',
      canDownload: false,
      readyToApply: true,
    }
  }

  if (status.update_available) {
    return {
      headline: `${status.latest_version ?? 'A new version'} is available`,
      detail: `This server is on ${current}. Downloading does not change anything yet.`,
      pill: 'Update available',
      tone: 'attention',
      canDownload: true,
      readyToApply: false,
    }
  }

  // No latest version at all means the check could not reach the manifest,
  // which is a different situation from being up to date and must not be
  // reported as one.
  if (!status.latest_version) {
    return {
      headline: `This server is on ${current}`,
      detail: 'MGA could not reach the update service, so whether a newer version exists is unknown.',
      pill: 'Cannot check',
      tone: 'neutral',
      canDownload: false,
      readyToApply: false,
    }
  }

  return {
    headline: `This server is up to date`,
    detail: `Running ${current}, which is the newest release.`,
    pill: 'Up to date',
    tone: 'good',
    canDownload: false,
    readyToApply: false,
  }
}

export function formatBytes(bytes: number | undefined): string {
  if (!bytes || bytes < 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value >= 10 || unit === 0 ? Math.round(value) : value.toFixed(1)} ${units[unit]}`
}

/** Enough of the checksum to compare against a release page, without a wall of
 *  hex nobody reads. */
export function shortDigest(digest: string | undefined): string {
  const trimmed = (digest ?? '').trim()
  if (trimmed.length <= 16) return trimmed || 'not recorded'
  return `${trimmed.slice(0, 8)}…${trimmed.slice(-8)}`
}
