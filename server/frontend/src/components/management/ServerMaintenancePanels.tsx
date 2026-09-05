import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Blocks, HardDrive, ImageDown, RefreshCw } from 'lucide-react'
import {
  clearMediaCache, getMediaQueueStatus, getSyncStatus, listCacheEntries, listPlugins,
  retryFailedMediaDownloads,
} from '@/api/client'
import { pluginLabel } from '@/lib/displayText'
import { ConfirmDialog } from '@/components/management/ManagementActions'
import { QueryFeedback, SectionCard, StatusPill, formatCount, formatDate } from '@/components/management/ManagementPrimitives'

/**
 * The parts of the System page that were endpoints with nobody calling them.
 *
 * Each panel reports something the server already knew and could not say, and
 * offers only the actions that are safe to offer: retrying a failed download
 * adds work, clearing a cache removes files and therefore states its scope
 * first.
 */

export function InstalledPluginsPanel({ admin }: { admin: boolean }) {
  const plugins = useQuery({ queryKey: ['management', 'plugins'], queryFn: listPlugins, enabled: admin })

  if (!admin) {
    return (
      <SectionCard title="What this server can connect to" description="The kinds of source and metadata provider MGA knows about.">
        <p className="text-xs text-mga-muted">Only an administrator can see this.</p>
      </SectionCard>
    )
  }

  const list = plugins.data ?? []
  return (
    <SectionCard
      title="What this server can connect to"
      description={list.length > 0 ? `${formatCount(list.length)} available. Connecting one is done from Sources.` : undefined}
    >
      <QueryFeedback
        pending={plugins.isPending}
        error={plugins.error}
        empty={!plugins.isPending && list.length === 0}
        emptyTitle="None found"
        emptyDescription="This server has no provider plugins installed."
      />
      {list.length > 0 && (
        <ul className="grid gap-2 sm:grid-cols-2">
          {list.map((plugin) => (
            <li key={plugin.plugin_id} className="flex items-center justify-between gap-3 rounded-lg border border-mga-border bg-mga-elevated/40 px-3 py-2">
              <div className="min-w-0">
                <p className="truncate text-xs font-medium text-mga-text">{pluginLabel(plugin.plugin_id)}</p>
                <p className="mt-0.5 truncate text-[0.68rem] text-mga-muted">
                  {(plugin.capabilities ?? []).map(describeCapability).filter(Boolean).join(' · ') || 'Provider'}
                </p>
              </div>
              <span className="shrink-0 font-mono text-[0.68rem] text-mga-muted">{plugin.plugin_version}</span>
            </li>
          ))}
        </ul>
      )}
    </SectionCard>
  )
}

/** Capability ids are ours; these are the words for them. */
function describeCapability(capability: string): string {
  switch (capability) {
    case 'source': return 'Finds games'
    case 'metadata': return 'Adds details and artwork'
    case 'save_sync': return 'Syncs saved games'
    case 'sync': return 'Backs up settings'
    case 'achievements': return 'Tracks achievements'
    default: return ''
  }
}

export function ArtworkPanel({ admin }: { admin: boolean }) {
  const queryClient = useQueryClient()
  const [confirmingClear, setConfirmingClear] = useState(false)

  const queue = useQuery({ queryKey: ['management', 'media-queue'], queryFn: getMediaQueueStatus, enabled: admin })
  const cache = useQuery({ queryKey: ['management', 'cache-entries'], queryFn: listCacheEntries, enabled: admin })

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ['management', 'media-queue'] })
    void queryClient.invalidateQueries({ queryKey: ['management', 'cache-entries'] })
  }
  const retry = useMutation({ mutationFn: retryFailedMediaDownloads, onSuccess: refresh })
  const clear = useMutation({ mutationFn: clearMediaCache, onSuccess: refresh })

  if (!admin) {
    return (
      <SectionCard title="Artwork and downloads" description="Covers and screenshots MGA has fetched.">
        <p className="text-xs text-mga-muted">Only an administrator can see this.</p>
      </SectionCard>
    )
  }

  const status = queue.data
  // These two are different things and the server treats them differently.
  // "Retry failed" resets rows that are NOT permanently failed — its SQL
  // excludes download_permanent_failure — so offering it for the permanent
  // ones would be a button that can never do anything. Verified against the
  // running server: three permanent failures, retry called, still three.
  const gaveUp = status?.failed_permanent ?? 0
  const working = (status?.downloading ?? 0) + (status?.queued ?? 0) + (status?.retry_waiting ?? 0)
  const pending = status?.items_left ?? 0
  const cached = cache.data?.length ?? 0

  return (
    <SectionCard title="Artwork and downloads" description="Covers and screenshots MGA has fetched for your games.">
      <QueryFeedback pending={queue.isPending} error={queue.error} empty={false} emptyTitle="" emptyDescription="" />

      {status && (
        <div className="space-y-4">
          <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-xs sm:grid-cols-3">
            <Fact label="Downloaded" value={formatCount(status.downloaded)} />
            <Fact label="Still to do" value={formatCount(working)} />
            <Fact
              label="Gave up on"
              value={formatCount(gaveUp)}
              tone={gaveUp > 0 ? 'attention' : undefined}
            />
          </dl>

          {gaveUp > 0 && (
            <div className="rounded-lg border border-amber-400/20 bg-amber-400/5 p-3">
              <p className="text-xs font-medium text-amber-200">
                {formatCount(gaveUp)} {gaveUp === 1 ? 'picture' : 'pictures'} were given up on
              </p>
              <p className="mt-1 text-xs leading-5 text-mga-muted">
                {status.last_error ? `The last attempt said: ${status.last_error}.` : 'The provider did not answer.'}
                {status.last_activity_at ? ` Last tried ${formatDate(status.last_activity_at)}.` : ''}
                {' '}MGA has stopped trying these, and trying pending downloads again will not pick them up.
                Deleting the downloaded artwork below is what makes it attempt them once more.
              </p>
            </div>
          )}

          <div className="flex flex-wrap gap-2">
            <button
              type="button"
              onClick={() => retry.mutate()}
              disabled={retry.isPending || pending === 0}
              title={pending === 0 ? 'Nothing is waiting to download.' : undefined}
              className="inline-flex items-center gap-1.5 rounded-md border border-mga-border px-3 py-1.5 text-xs text-mga-muted transition hover:text-mga-text disabled:opacity-50"
            >
              <RefreshCw className="h-3.5 w-3.5" />
              {retry.isPending ? 'Trying again…' : 'Try pending downloads again'}
            </button>
            <button
              type="button"
              onClick={() => setConfirmingClear(true)}
              disabled={clear.isPending}
              className="inline-flex items-center gap-1.5 rounded-md border border-mga-border px-3 py-1.5 text-xs text-mga-muted transition hover:text-rose-200 disabled:opacity-50"
            >
              <ImageDown className="h-3.5 w-3.5" />
              Delete downloaded artwork
            </button>
          </div>

          {(retry.error || clear.error) && (
            <p className="text-xs text-rose-300" role="alert">
              {(retry.error ?? clear.error) instanceof Error
                ? (retry.error ?? clear.error as Error).message
                : 'The server refused that.'}
            </p>
          )}

          <p className="text-[0.68rem] text-mga-muted">
            {cached > 0
              ? `${formatCount(cached)} game files are also cached for delivery.`
              : 'No game files are cached for delivery right now.'}
          </p>
        </div>
      )}

      <ConfirmDialog
        open={confirmingClear}
        title="Delete downloaded artwork?"
        confirmLabel="Delete artwork"
        submitting={clear.isPending}
        error={clear.error}
        consequences={[
          'Remove every cover and screenshot this server has downloaded',
          'Leave your games without pictures until they are fetched again',
          'Start downloading them all again from your metadata sources, including the ones MGA gave up on',
        ]}
        preserves={[
          'Your games, sources and profiles',
          'Any game files stored for delivery',
          'Achievements and saved games',
        ]}
        onClose={() => setConfirmingClear(false)}
        onConfirm={() => { setConfirmingClear(false); clear.mutate() }}
      />
    </SectionCard>
  )
}

export function BackupPanel({ admin }: { admin: boolean }) {
  const sync = useQuery({ queryKey: ['management', 'sync-status'], queryFn: getSyncStatus, enabled: admin })

  if (!admin) {
    return (
      <SectionCard title="Backup" description="A copy of this profile and its connections, kept somewhere you choose.">
        <p className="text-xs text-mga-muted">Only an administrator can see this.</p>
      </SectionCard>
    )
  }

  const status = sync.data
  return (
    <SectionCard title="Backup" description="A copy of this profile and its connections, kept somewhere you choose.">
      <QueryFeedback pending={sync.isPending} error={sync.error} empty={false} emptyTitle="" emptyDescription="" />
      {status && (
        <div className="space-y-3">
          <div className="flex flex-wrap items-center gap-2">
            <StatusPill
              label={status.configured ? 'Set up' : 'Not set up'}
              tone={status.configured ? 'good' : 'neutral'}
            />
            {status.configured && (
              <StatusPill
                label={status.has_stored_key ? 'Passphrase remembered' : 'Passphrase needed each time'}
                tone="neutral"
              />
            )}
          </div>

          <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-xs">
            <Fact label="Last backed up" value={status.last_push ? formatDate(status.last_push) : 'Never'} />
            <Fact label="Last restored" value={status.last_pull ? formatDate(status.last_pull) : 'Never'} />
          </dl>

          {/* Backing up and restoring both need the passphrase that encrypts
              the copy, and restoring replaces this profile's connections.
              Neither belongs behind a button that could be pressed by accident
              while its behaviour is still unverified here. */}
          <p className="text-xs leading-5 text-mga-muted">
            {status.configured
              ? 'Backing up and restoring are not available from this page yet. Both need the passphrase that encrypts the copy, and restoring replaces this profile’s connections.'
              : 'Connect a sync source under Sources to keep an encrypted copy of this profile and its connections.'}
          </p>
        </div>
      )}
    </SectionCard>
  )
}

function Fact({ label, value, tone }: { label: string; value: string; tone?: 'attention' }) {
  return (
    <div>
      <dt className="text-mga-muted">{label}</dt>
      <dd className={`mt-0.5 ${tone === 'attention' ? 'text-amber-200' : 'text-mga-text'}`}>{value}</dd>
    </div>
  )
}

export { Blocks as PluginsIcon, HardDrive as StorageIcon }
