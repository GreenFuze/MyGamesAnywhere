import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Blocks, ChevronDown, ChevronRight, CloudDownload, CloudUpload, HardDrive, ImageDown, KeyRound, RefreshCw,
} from 'lucide-react'
import {
  clearKey, clearMediaCache, getMediaQueueStatus, getSyncStatus, listCacheEntries, listPlugins,
  retryFailedMediaDownloads, storeKey, syncPull, syncPush, type PluginInfo,
} from '@/api/client'
import { Input } from '@/components/ui/input'
import { pluginLabel, pluginSettingLabel } from '@/lib/displayText'
import { ActionError, ConfirmDialog, FormDialog } from '@/components/management/ManagementActions'
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
          {list.map((plugin) => <PluginRow key={plugin.plugin_id} plugin={plugin} />)}
        </ul>
      )}
    </SectionCard>
  )
}

/**
 * One plugin, and what it wants to be told before it can connect.
 *
 * The schema comes with the list, so opening a row costs nothing: the detail
 * endpoint returns the same fields the list already carried, and asking for it
 * again would only add a request.
 */
function PluginRow({ plugin }: { plugin: PluginInfo }) {
  const [open, setOpen] = useState(false)
  const settings = Object.entries(plugin.config ?? {})

  return (
    <li className="rounded-lg border border-mga-border bg-mga-elevated/40">
      <button
        type="button"
        onClick={() => setOpen((current) => !current)}
        aria-expanded={open}
        className="flex w-full items-center justify-between gap-3 px-3 py-2 text-left focus:outline-none focus:ring-2 focus:ring-mga-accent/50"
      >
        <span className="flex min-w-0 items-center gap-2">
          {open ? <ChevronDown className="h-3.5 w-3.5 shrink-0 text-mga-muted" /> : <ChevronRight className="h-3.5 w-3.5 shrink-0 text-mga-muted" />}
          <span className="min-w-0">
            <span className="block truncate text-xs font-medium text-mga-text">{pluginLabel(plugin.plugin_id)}</span>
            <span className="mt-0.5 block truncate text-[0.68rem] text-mga-muted">
              {(plugin.capabilities ?? []).map(describeCapability).filter(Boolean).join(' · ') || 'Provider'}
            </span>
          </span>
        </span>
        <span className="shrink-0 font-mono text-[0.68rem] text-mga-muted">{plugin.plugin_version}</span>
      </button>

      {open && (
        <div className="border-t border-mga-border px-3 py-2.5">
          <p className="break-all font-mono text-[0.66rem] text-mga-muted">{plugin.plugin_id}</p>
          {settings.length === 0 ? (
            <p className="mt-2 text-[0.68rem] text-mga-muted">
              Nothing to fill in. It either signs you in with the provider or needs no settings.
            </p>
          ) : (
            <dl className="mt-2 space-y-1.5">
              {settings.map(([key, schema]) => (
                <div key={key}>
                  <dt className="text-[0.68rem] text-mga-text">
                    {pluginSettingLabel(key)}
                    {isRequired(schema) && <span className="ml-1.5 text-mga-muted">needed</span>}
                  </dt>
                  {describeSetting(schema) && (
                    <dd className="text-[0.66rem] leading-4 text-mga-muted">{describeSetting(schema)}</dd>
                  )}
                </div>
              ))}
            </dl>
          )}
          <p className="mt-2.5 text-[0.66rem] text-mga-muted">These are filled in on the Sources page when you connect it.</p>
        </div>
      )}
    </li>
  )
}

function isRequired(schema: unknown): boolean {
  return typeof schema === 'object' && schema !== null && (schema as { required?: unknown }).required === true
}

function describeSetting(schema: unknown): string {
  if (typeof schema !== 'object' || schema === null) return ''
  const description = (schema as { description?: unknown }).description
  return typeof description === 'string' ? description : ''
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

/**
 * Backing up this profile, restoring it, and the passphrase that encrypts the
 * copy.
 *
 * Every action here needs that passphrase. MGA can remember it, and when it
 * does the passphrase is never sent back — so this panel can say whether one is
 * stored, and can replace or forget it, but can never show it.
 *
 * Restoring is the one action that takes something away: it replaces this
 * profile's connections with the ones in the backup. It states that first.
 */
export function BackupPanel({ admin }: { admin: boolean }) {
  const queryClient = useQueryClient()
  const sync = useQuery({ queryKey: ['management', 'sync-status'], queryFn: getSyncStatus, enabled: admin })

  const [asking, setAsking] = useState<'back-up' | 'restore' | 'remember' | null>(null)
  const [passphrase, setPassphrase] = useState('')
  const [confirmingForget, setConfirmingForget] = useState(false)
  const [done, setDone] = useState<string | null>(null)

  const refresh = () => queryClient.invalidateQueries({ queryKey: ['management', 'sync-status'] })
  const finish = (message: string) => {
    setPassphrase('')
    setAsking(null)
    setDone(message)
    void refresh()
  }

  const backUp = useMutation({
    mutationFn: (secret?: string) => syncPush(secret),
    onSuccess: (result) => finish(`Backed up ${result.integrations} connections and ${result.settings} settings.`),
  })
  const restore = useMutation({
    mutationFn: (secret?: string) => syncPull(secret),
    onSuccess: () => finish('Restored from the backup. Check your connections on the Sources page.'),
  })
  const remember = useMutation({
    mutationFn: (secret: string) => storeKey(secret),
    onSuccess: () => finish('MGA will use that passphrase from now on.'),
  })
  const forget = useMutation({
    mutationFn: clearKey,
    onSuccess: () => { setConfirmingForget(false); finish('Forgotten. MGA will ask for it next time.') },
  })

  if (!admin) {
    return (
      <SectionCard title="Backup" description="A copy of this profile and its connections, kept somewhere you choose.">
        <p className="text-xs text-mga-muted">Only an administrator can see this.</p>
      </SectionCard>
    )
  }

  const status = sync.data
  const stored = status?.has_stored_key ?? false
  const busy = backUp.isPending || restore.isPending || remember.isPending || forget.isPending

  // With no remembered passphrase every action has to ask for one first.
  const start = (action: 'back-up' | 'restore') => {
    setDone(null)
    if (stored) {
      if (action === 'back-up') backUp.mutate(undefined)
      else setAsking('restore')
      return
    }
    setPassphrase('')
    setAsking(action)
  }

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
                label={stored ? 'Passphrase remembered' : 'Passphrase needed each time'}
                tone="neutral"
              />
            )}
          </div>

          <dl className="grid grid-cols-2 gap-x-4 gap-y-2 text-xs">
            <Fact label="Last backed up" value={status.last_push ? formatDate(status.last_push) : 'Never'} />
            <Fact label="Last restored" value={status.last_pull ? formatDate(status.last_pull) : 'Never'} />
          </dl>

          {!status.configured ? (
            <p className="text-xs leading-5 text-mga-muted">
              Connect a sync source under Sources to keep an encrypted copy of this profile and its connections.
            </p>
          ) : (
            <>
              <div className="flex flex-wrap gap-2">
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => start('back-up')}
                  className="inline-flex items-center gap-1.5 rounded-md border border-mga-border px-3 py-1.5 text-xs text-mga-muted transition hover:text-mga-text disabled:opacity-50"
                >
                  <CloudUpload className="h-3.5 w-3.5" />
                  {backUp.isPending ? 'Backing up…' : 'Back up now'}
                </button>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => start('restore')}
                  className="inline-flex items-center gap-1.5 rounded-md border border-mga-border px-3 py-1.5 text-xs text-mga-muted transition hover:text-mga-text disabled:opacity-50"
                >
                  <CloudDownload className="h-3.5 w-3.5" />
                  {restore.isPending ? 'Restoring…' : 'Restore from backup'}
                </button>
                <button
                  type="button"
                  disabled={busy}
                  onClick={() => { setDone(null); setPassphrase(''); stored ? setConfirmingForget(true) : setAsking('remember') }}
                  className="inline-flex items-center gap-1.5 rounded-md border border-mga-border px-3 py-1.5 text-xs text-mga-muted transition hover:text-mga-text disabled:opacity-50"
                >
                  <KeyRound className="h-3.5 w-3.5" />
                  {stored ? 'Forget my passphrase' : 'Remember my passphrase'}
                </button>
              </div>

              {done && <p className="text-xs text-emerald-300">{done}</p>}
              <ActionError error={backUp.error ?? restore.error ?? remember.error ?? forget.error} />

              <p className="text-[0.68rem] leading-5 text-mga-muted">
                The copy is encrypted with your passphrase. MGA cannot read it without one and cannot recover it if you
                lose it.
              </p>
            </>
          )}
        </div>
      )}

      <FormDialog
        open={asking !== null}
        onClose={() => { setAsking(null); setPassphrase('') }}
        title={asking === 'remember' ? 'Remember your passphrase' : asking === 'restore' ? 'Restore from backup' : 'Back up now'}
        description={asking === 'remember'
          ? 'MGA will use it for backups from now on and will never show it again.'
          : 'The passphrase that encrypts the copy. It is used for this one action.'}
        submitLabel={asking === 'remember' ? 'Remember it' : asking === 'restore' ? 'Restore' : 'Back up'}
        submitting={busy}
        error={backUp.error ?? restore.error ?? remember.error}
        disabled={passphrase.trim() === ''}
        destructive={asking === 'restore'}
        onSubmit={() => {
          if (asking === 'remember') remember.mutate(passphrase)
          else if (asking === 'restore') restore.mutate(stored ? undefined : passphrase)
          else backUp.mutate(passphrase)
        }}
      >
        {asking === 'restore' && (
          <div className="rounded-lg border border-rose-400/25 bg-rose-500/5 p-3 text-xs leading-5 text-mga-text">
            This replaces this profile’s connections with the ones in the backup. Your games, saves and downloaded
            pictures stay where they are.
          </div>
        )}
        {!(asking === 'restore' && stored) && (
          <Input
            label="Passphrase"
            type="password"
            autoComplete="off"
            value={passphrase}
            onChange={(event) => setPassphrase(event.target.value)}
            autoFocus
          />
        )}
      </FormDialog>

      <ConfirmDialog
        open={confirmingForget}
        title="Forget your passphrase?"
        confirmLabel="Forget it"
        submitting={forget.isPending}
        error={forget.error}
        consequences={[
          'Remove the stored passphrase from this server',
          'Ask you for it every time you back up or restore',
        ]}
        preserves={[
          'The backup itself, which stays where it is',
          'Your games, connections and profiles',
        ]}
        onClose={() => setConfirmingForget(false)}
        onConfirm={() => forget.mutate()}
      />
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
