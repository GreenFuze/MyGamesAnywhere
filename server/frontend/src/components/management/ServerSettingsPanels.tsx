import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Download, FileText, Globe, Laptop, RefreshCw, Save } from 'lucide-react'
import {
  downloadServerLog, getServerLogTail, getServerSettings, setServerNetwork,
  type ServerSettings,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { ActionError, ConfirmDialog } from '@/components/management/ManagementActions'
import { QueryFeedback, SectionCard, StatusPill, formatDate } from '@/components/management/ManagementPrimitives'
import {
  describeReach, formatUptime, orderStorage, otherSettings, settingLabel,
} from '@/lib/serverSettings'
import { formatBytes } from '@/lib/updateStatus'

/**
 * What this server is running on, where it keeps things, and who can reach it.
 *
 * The server has one configuration file and it is read once, at startup. That
 * shapes everything here: a change is written to the file and takes effect at
 * the next start, so the panel shows the value in use and the value waiting
 * rather than pretending a save took effect immediately.
 */
export function ServerSettingsPanel({ admin }: { admin: boolean }) {
  const settings = useQuery({
    queryKey: ['management', 'server-settings'],
    queryFn: getServerSettings,
    enabled: admin,
  })

  if (!admin) {
    return (
      <SectionCard title="How MGA is reached" description="The address other devices use, and where MGA keeps things.">
        <p className="text-xs text-mga-muted">Only an administrator can see this.</p>
      </SectionCard>
    )
  }

  return (
    <div className="grid gap-5 xl:grid-cols-2">
      <NetworkCard data={settings.data} pending={settings.isPending} error={settings.error} />
      <LocationsCard data={settings.data} pending={settings.isPending} error={settings.error} />
    </div>
  )
}

function NetworkCard({ data, pending, error }: { data?: ServerSettings; pending: boolean; error: unknown }) {
  const queryClient = useQueryClient()
  const [choice, setChoice] = useState<'this-computer' | 'whole-network' | 'one-address'>('this-computer')
  const [address, setAddress] = useState('')
  const [port, setPort] = useState('')
  const [confirming, setConfirming] = useState(false)

  const current = useMemo(() => {
    const map = new Map((data?.settings ?? []).map((setting) => [setting.key, setting]))
    return {
      listenIP: map.get('LISTEN_IP')?.value ?? '',
      port: map.get('PORT')?.value ?? '',
      pendingListenIP: map.get('LISTEN_IP')?.pending_value ?? '',
      pendingPort: map.get('PORT')?.pending_value ?? '',
    }
  }, [data])

  // The form starts from whatever is waiting to be applied, falling back to
  // what is running: editing a value that is already saved but not yet live
  // must not silently revert it.
  //
  // The dependencies are the values, never the response object. This query
  // refetches when the window regains focus, and depending on the object would
  // reseed the form from an identical answer — wiping an address someone was
  // halfway through typing every time they came back to the tab.
  const loaded = Boolean(data)
  useEffect(() => {
    if (!loaded) return
    const effective = current.pendingListenIP || current.listenIP
    setChoice(describeReach(effective).reach)
    setAddress(effective)
    setPort(current.pendingPort || current.port)
  }, [loaded, current.listenIP, current.port, current.pendingListenIP, current.pendingPort])

  const save = useMutation({
    mutationFn: () => setServerNetwork({ listen_ip: listenIPForChoice(choice, address), port }),
    onSuccess: (result) => {
      queryClient.setQueryData(['management', 'server-settings'], result)
      setConfirming(false)
    },
  })

  const reach = describeReach(current.listenIP)
  const chosenIP = listenIPForChoice(choice, address)
  const opensUp = choice === 'whole-network' && reach.reach !== 'whole-network'
  const changed = chosenIP !== current.listenIP || port !== current.port
  const waiting = Boolean(current.pendingListenIP || current.pendingPort)

  return (
    <SectionCard title="Who can reach MGA" description="MGA answers on one address and one port. Changing either takes effect when MGA next starts.">
      <QueryFeedback pending={pending} error={error} empty={false} emptyTitle="" emptyDescription="" />
      {data && (
        <div className="space-y-4">
          <div className="rounded-lg border border-mga-border bg-mga-elevated/40 p-3">
            <div className="flex items-center gap-2">
              {reach.reach === 'whole-network' ? <Globe className="h-4 w-4 text-mga-muted" /> : <Laptop className="h-4 w-4 text-mga-muted" />}
              <p className="text-sm font-medium text-mga-text">{reach.headline}</p>
            </div>
            <p className="mt-1 text-xs leading-5 text-mga-muted">{reach.detail}</p>
            <p className="mt-2 font-mono text-[0.68rem] text-mga-muted">{current.listenIP}:{current.port}</p>
          </div>

          {waiting && (
            <div className="rounded-lg border border-amber-400/25 bg-amber-400/5 p-3">
              <p className="text-xs leading-5 text-amber-200">
                Saved, and waiting for a restart. MGA will answer on{' '}
                <span className="font-mono">{current.pendingListenIP || current.listenIP}:{current.pendingPort || current.port}</span>{' '}
                the next time it starts. Until then it stays where it is.
              </p>
            </div>
          )}

          <fieldset className="space-y-2">
            <legend className="text-xs font-medium text-mga-text">Let MGA be reached from</legend>
            <Choice
              checked={choice === 'this-computer'}
              onSelect={() => { setChoice('this-computer'); setAddress('127.0.0.1') }}
              title="Only this computer"
              detail="Nothing else on the network can connect."
            />
            <Choice
              checked={choice === 'whole-network'}
              onSelect={() => { setChoice('whole-network'); setAddress('0.0.0.0') }}
              title="Any device on my network"
              detail="Needed for a TV, a phone, or an app on another PC."
            />
            <Choice
              checked={choice === 'one-address'}
              onSelect={() => setChoice('one-address')}
              title="One address I choose"
              detail="For a computer with more than one network."
            />
            {choice === 'one-address' && (
              <Input
                label="Address"
                value={address}
                onChange={(event) => setAddress(event.target.value)}
                placeholder="192.168.1.20"
                className="font-mono"
              />
            )}
          </fieldset>

          <Input
            label="Port"
            value={port}
            inputMode="numeric"
            onChange={(event) => setPort(event.target.value)}
            className="font-mono"
          />

          <div className="flex flex-wrap items-center gap-3">
            <Button
              size="sm"
              disabled={!changed || save.isPending}
              onClick={() => (opensUp ? setConfirming(true) : save.mutate())}
            >
              <Save className="h-3.5 w-3.5" /> {save.isPending ? 'Saving…' : 'Save'}
            </Button>
            {!changed && <span className="text-xs text-mga-muted">Nothing to save.</span>}
          </div>

          <ActionError error={save.error} />

          <p className="text-[0.68rem] leading-5 text-mga-muted">
            These come from <span className="font-mono">{data.config_path}</span>, which MGA reads once when it starts.
            A copy of the previous file is kept beside it.
          </p>
        </div>
      )}

      <ConfirmDialog
        open={confirming}
        title="Let other devices reach MGA?"
        confirmLabel="Save"
        submitting={save.isPending}
        error={save.error}
        consequences={[
          'Answer to every device on the same network, not only this computer',
          'Expose the sign-in page and any access key you have issued to that network',
          'Take effect the next time MGA starts',
        ]}
        preserves={[
          'Your games, sources and profiles',
          'The password and access keys you already have',
          'Anything on a different network or the wider internet, which still cannot reach MGA',
        ]}
        onClose={() => setConfirming(false)}
        onConfirm={() => save.mutate()}
      />
    </SectionCard>
  )
}

function listenIPForChoice(choice: string, address: string): string {
  if (choice === 'this-computer') return '127.0.0.1'
  if (choice === 'whole-network') return '0.0.0.0'
  return address.trim()
}

function Choice({ checked, onSelect, title, detail }: {
  checked: boolean
  onSelect: () => void
  title: string
  detail: string
}) {
  return (
    <label className="flex cursor-pointer items-start gap-2 rounded-lg border border-mga-border bg-mga-elevated/30 p-2.5">
      <input
        type="radio"
        checked={checked}
        onChange={onSelect}
        className="mt-0.5 h-3.5 w-3.5 accent-[color:var(--mga-accent,#7c5cff)]"
      />
      <span>
        <span className="block text-xs font-medium text-mga-text">{title}</span>
        <span className="mt-0.5 block text-[0.68rem] text-mga-muted">{detail}</span>
      </span>
    </label>
  )
}

function LocationsCard({ data, pending, error }: { data?: ServerSettings; pending: boolean; error: unknown }) {
  const storage = orderStorage(data?.storage ?? [])
  const rest = otherSettings(data?.settings ?? [])

  return (
    <SectionCard title="Where MGA keeps things" description="Chosen when MGA was installed. Moving any of them means editing the file and restarting.">
      <QueryFeedback pending={pending} error={error} empty={false} emptyTitle="" emptyDescription="" />
      {data && (
        <div className="space-y-4">
          <ul className="space-y-2">
            {storage.map((entry) => (
              <li key={entry.key} className="rounded-lg border border-mga-border bg-mga-elevated/40 px-3 py-2">
                <div className="flex items-center justify-between gap-3">
                  <p className="text-xs font-medium text-mga-text">{settingLabel(entry.key)}</p>
                  {entry.source === 'unset'
                    ? <StatusPill label="MGA decides" tone="neutral" />
                    : entry.exists
                      ? <StatusPill label={entry.size_bytes ? formatBytes(entry.size_bytes) : 'Found'} tone="neutral" />
                      : <StatusPill label="Not there yet" tone="attention" />}
                </div>
                {/* A location nothing pinned down is said to be unpinned rather
                    than guessed at: each part of MGA picks its own folder when
                    the file is silent, and they do not all pick the same one. */}
                <p className="mt-1 break-all font-mono text-[0.66rem] text-mga-muted">
                  {entry.source === 'unset' ? 'Not written in the configuration file.' : entry.path}
                </p>
              </li>
            ))}
          </ul>

          {rest.length > 0 && (
            <dl className="grid grid-cols-[minmax(0,1fr)_auto] gap-x-4 gap-y-2 text-xs">
              {rest.map((setting) => (
                <div key={setting.key} className="contents">
                  <dt className="truncate text-mga-muted">{settingLabel(setting.key)}</dt>
                  <dd className="truncate text-right font-mono text-[0.68rem] text-mga-text">
                    {setting.value || (setting.source === 'unset' ? 'MGA decides' : '—')}
                  </dd>
                </div>
              ))}
            </dl>
          )}

          <div className="text-[0.68rem] leading-5 text-mga-muted">
            <p>
              Running {data.server.version || 'a development build'} on {data.server.os}/{data.server.arch},
              started {formatDate(data.server.started_at)} and up for {formatUptime(data.server.uptime_seconds)}.
            </p>
            {data.other_keys.length > 0 && (
              <p className="mt-1">
                The file also holds settings MGA does not use: {data.other_keys.join(', ')}. Their values are not shown here.
              </p>
            )}
          </div>
        </div>
      )}
    </SectionCard>
  )
}

const LOG_TAIL_LINES = 200

/**
 * The end of the server log, and the whole file as a download.
 *
 * The log is the first thing anyone asks for when MGA misbehaves, and until now
 * the only way to get it was to know where MGA had been installed. It records
 * paths and identifiers, so it is administrator-only and says so before it is
 * sent anywhere.
 */
export function ServerLogPanel({ admin }: { admin: boolean }) {
  const [refreshedAt, setRefreshedAt] = useState<string | null>(null)

  const settings = useQuery({
    queryKey: ['management', 'server-settings'],
    queryFn: getServerSettings,
    enabled: admin,
  })
  const tail = useQuery({
    queryKey: ['management', 'server-log', LOG_TAIL_LINES],
    queryFn: () => getServerLogTail(LOG_TAIL_LINES),
    enabled: admin,
  })

  useEffect(() => {
    if (tail.data !== undefined) setRefreshedAt(new Date().toISOString())
  }, [tail.data])

  const download = useMutation({
    mutationFn: async () => {
      const blob = await downloadServerLog()
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `mga-server-log-${new Date().toISOString().slice(0, 10)}.txt`
      anchor.click()
      URL.revokeObjectURL(url)
    },
  })

  if (!admin) {
    return (
      <SectionCard title="Server log" description="What MGA recorded while it was running.">
        <p className="text-xs text-mga-muted">Only an administrator can see this.</p>
      </SectionCard>
    )
  }

  const file = settings.data?.log
  return (
    <SectionCard
      title="Server log"
      description={file?.available
        ? `${formatBytes(file.size_bytes)}, last written ${formatDate(file.modified_at)}.`
        : 'What MGA recorded while it was running.'}
    >
      <div className="mb-3 flex flex-wrap gap-2">
        <Button variant="outline" size="sm" disabled={tail.isFetching} onClick={() => void tail.refetch()}>
          <RefreshCw className="h-3.5 w-3.5" /> {tail.isFetching ? 'Reading…' : 'Read again'}
        </Button>
        <Button variant="outline" size="sm" disabled={download.isPending} onClick={() => download.mutate()}>
          <Download className="h-3.5 w-3.5" /> {download.isPending ? 'Preparing…' : 'Download the whole log'}
        </Button>
      </div>

      <QueryFeedback
        pending={tail.isPending}
        error={tail.error}
        empty={!tail.isPending && !tail.error && (tail.data ?? '').trim() === ''}
        emptyTitle="Nothing recorded yet"
        emptyDescription="MGA has not written anything to its log since it started."
      />

      {tail.data && tail.data.trim() !== '' && (
        <>
          <pre className="max-h-80 overflow-auto rounded-lg border border-mga-border bg-mga-bg p-3 font-mono text-[0.66rem] leading-5 text-mga-muted">
            {tail.data}
          </pre>
          <p className="mt-2 flex items-center gap-1.5 text-[0.68rem] text-mga-muted">
            <FileText className="h-3 w-3" />
            The last {LOG_TAIL_LINES} lines{refreshedAt ? `, read ${formatDate(refreshedAt)}` : ''}. The file records folder
            paths and account identifiers, so treat a copy of it as private.
          </p>
        </>
      )}

      <ActionError error={download.error} className="mt-3" />
    </SectionCard>
  )
}
