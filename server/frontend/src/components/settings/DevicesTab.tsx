import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Activity,
  Check,
  Clipboard,
  Download,
  KeyRound,
  Laptop,
  Plus,
  RefreshCw,
  Save,
  Send,
  ShieldAlert,
  Trash2,
  UserRound,
} from 'lucide-react'
import {
  changeOwnCredential,
  createDevicePairingChallenge,
  dispatchDeviceCommand,
  getAuthSession,
  getCredentialStatus,
  initializeCredential,
  deleteDeviceGrant,
  getDeviceClientDownload,
  listDeviceCommands,
  listDeviceGrants,
  listDevices,
  removeOwnCredential,
  renameDevice,
  revokeDevice,
  setDeviceGrant,
  type CredentialKind,
  type DeviceAccessLevel,
  type DeviceEndpoint,
  type DevicePairingChallenge,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { useProfiles } from '@/hooks/useProfiles'
import { cn } from '@/lib/utils'

const statePresentation = {
  ready: { label: 'Ready', dot: 'bg-emerald-400', text: 'text-emerald-300', border: 'border-emerald-500/30' },
  busy: { label: 'Busy', dot: 'bg-amber-400', text: 'text-amber-300', border: 'border-amber-500/30' },
  offline: { label: 'Offline', dot: 'bg-slate-500', text: 'text-slate-300', border: 'border-slate-500/30' },
  update_required: { label: 'Update required', dot: 'bg-purple-400', text: 'text-purple-300', border: 'border-purple-500/40' },
  error: { label: 'Error', dot: 'bg-red-400', text: 'text-red-300', border: 'border-red-500/40' },
} as const

export function DevicesTab() {
  const { currentProfile } = useProfiles()
  const [pairing, setPairing] = useState<DevicePairingChallenge | null>(null)
  const [changingCredential, setChangingCredential] = useState(false)
  const sessionQuery = useQuery({ queryKey: ['auth-session'], queryFn: getAuthSession, retry: false })
  const credentialQuery = useQuery({
    queryKey: ['credential-status', currentProfile?.id],
    queryFn: getCredentialStatus,
    enabled: Boolean(currentProfile),
  })
  const authorized = Boolean(
    currentProfile
      && sessionQuery.data?.authenticated
      && sessionQuery.data.profile?.id === currentProfile.id
      && !sessionQuery.data.must_change,
  )
  const devicesQuery = useQuery({
    queryKey: ['devices', currentProfile?.id],
    queryFn: listDevices,
    enabled: authorized,
    refetchInterval: 3000,
  })
  const downloadQuery = useQuery({ queryKey: ['device-client-download'], queryFn: getDeviceClientDownload })

  const createPairing = useMutation({
    mutationFn: createDevicePairingChallenge,
    onSuccess: setPairing,
  })

  if (!currentProfile) return null
  if (sessionQuery.isLoading || credentialQuery.isLoading) return <PanelMessage text="Checking device-management session…" />
  if (credentialQuery.data && !credentialQuery.data.configured) {
    return <InitializeCredentialPanel profileId={currentProfile.id} />
  }
  if (!authorized) return <PanelMessage text="Sign out and sign in to this profile again to manage devices." />

  return (
    <div className="space-y-5">
      {changingCredential ? <ChangeCredentialPanel optional onClose={() => setChangingCredential(false)} /> : null}
      <section className="rounded-mga border border-mga-border bg-mga-surface p-5 shadow-lg">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <div className="flex items-center gap-2 text-xs font-bold uppercase tracking-[0.2em] text-mga-accent">
              <Activity className="h-4 w-4" /> Device control plane
            </div>
            <h2 className="mt-2 text-xl font-black text-mga-text">MGA Clients</h2>
            <p className="mt-1 max-w-2xl text-sm leading-6 text-mga-muted">
              Each entry is one physical-device / OS-user / MGA Client installation. A second user on the same PC appears separately.
            </p>
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              onClick={() => downloadQuery.data?.download_url && window.open(downloadQuery.data.download_url, '_blank', 'noopener,noreferrer')}
              disabled={!downloadQuery.data?.download_url}
            >
              <Download className="h-4 w-4" /> Download MGA Client
            </Button>
            <Button onClick={() => createPairing.mutate()} disabled={createPairing.isPending}>
              <Plus className="h-4 w-4" /> Add Device
            </Button>
            <Button variant="outline" onClick={() => setChangingCredential((value) => !value)}>
              <KeyRound className="h-4 w-4" /> Credential
            </Button>
          </div>
        </div>
        {createPairing.error ? <ErrorText error={createPairing.error} /> : null}
      </section>

      {pairing ? <PairingPanel pairing={pairing} onClose={() => setPairing(null)} /> : null}

      {devicesQuery.isLoading ? <PanelMessage text="Loading devices…" /> : null}
      {devicesQuery.error ? <ErrorText error={devicesQuery.error} /> : null}
      {devicesQuery.data?.length === 0 ? (
        <section className="rounded-mga border border-dashed border-mga-border bg-mga-surface/70 p-10 text-center">
          <Laptop className="mx-auto h-10 w-10 text-mga-muted" />
          <h3 className="mt-3 text-lg font-bold text-mga-text">No MGA Client paired</h3>
          <p className="mt-1 text-sm text-mga-muted">Choose Add Device, then run the shown command from the target OS user.</p>
        </section>
      ) : null}
      <div className="grid gap-4 xl:grid-cols-2">
        {devicesQuery.data?.map((device) => (
          <DeviceCard key={device.id} device={device} />
        ))}
      </div>
    </div>
  )
}

function InitializeCredentialPanel({ profileId }: { profileId: string }) {
  const queryClient = useQueryClient()
  const [kind, setKind] = useState<CredentialKind>('password')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const initialize = useMutation({
    mutationFn: () => initializeCredential(next, kind),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['credential-status', profileId] })
      await queryClient.invalidateQueries({ queryKey: ['auth-session'] })
    },
  })
  const valid = next === confirm && (kind === 'password' ? next.length >= 8 : /^\d{6,12}$/.test(next))
  return (
    <section className="mx-auto max-w-xl rounded-mga border border-mga-border bg-mga-surface p-6 shadow-xl">
      <KeyRound className="h-9 w-9 text-mga-accent" />
      <h2 className="mt-3 text-xl font-black text-mga-text">Set up a password or PIN</h2>
      <p className="mt-2 text-sm leading-6 text-mga-muted">This profile can use MGA normally without one. Device pairing and control require a real credential.</p>
      <div className="mt-5 space-y-3">
        <Select label="Credential type" value={kind} onChange={(event) => setKind(event.target.value as CredentialKind)} options={[{ value: 'password', label: 'Password' }, { value: 'pin', label: 'PIN' }]} />
        <Input label={kind === 'pin' ? 'New PIN (6–12 digits)' : 'New password (8+ characters)'} type="password" value={next} onChange={(event) => setNext(event.target.value)} />
        <Input label="Confirm credential" type="password" value={confirm} onChange={(event) => setConfirm(event.target.value)} />
        <Button onClick={() => initialize.mutate()} disabled={!valid || initialize.isPending} className="w-full"><KeyRound className="h-4 w-4" /> Enable Device Management</Button>
        {next !== confirm && confirm ? <p className="text-sm text-red-400">Credentials do not match.</p> : null}
        {initialize.error ? <ErrorText error={initialize.error} /> : null}
        <p className="text-xs text-mga-muted">Initial setup is intentionally limited to the computer running MGA Server.</p>
      </div>
    </section>
  )
}

function ChangeCredentialPanel({ optional = false, onClose }: { optional?: boolean; onClose?: () => void }) {
  const queryClient = useQueryClient()
  const [current, setCurrent] = useState('')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [kind, setKind] = useState<CredentialKind>('password')
  const change = useMutation({
    mutationFn: () => changeOwnCredential(current, next, kind),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['auth-session'] })
      onClose?.()
    },
  })
  const disable = useMutation({
    mutationFn: removeOwnCredential,
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['auth-session'] })
      await queryClient.invalidateQueries({ queryKey: ['credential-status'] })
      onClose?.()
    },
  })
  const valid = current && next === confirm && (kind === 'password' ? next.length >= 8 : /^\d{6,12}$/.test(next))
  return (
    <section className="mx-auto max-w-xl rounded-mga border border-amber-500/30 bg-mga-surface p-6 shadow-xl">
      <ShieldAlert className="h-9 w-9 text-amber-300" />
      <h2 className="mt-3 text-xl font-black text-mga-text">{optional ? 'Change profile credential' : 'Replace the bootstrap password'}</h2>
      <p className="mt-2 text-sm leading-6 text-mga-muted">{optional ? 'Changing it invalidates other signed-in sessions for this profile.' : 'Device control stays disabled until the public default is replaced.'}</p>
      <div className="mt-5 space-y-3">
        <Select
          label="Credential type"
          value={kind}
          onChange={(event) => setKind(event.target.value as CredentialKind)}
          options={[{ value: 'password', label: 'Password' }, { value: 'pin', label: 'PIN' }]}
        />
        <Input label="Current password" type="password" value={current} onChange={(event) => setCurrent(event.target.value)} />
        <Input label={kind === 'pin' ? 'New PIN (6–12 digits)' : 'New password (8+ characters)'} type="password" value={next} onChange={(event) => setNext(event.target.value)} />
        <Input label="Confirm new credential" type="password" value={confirm} onChange={(event) => setConfirm(event.target.value)} />
        <Button onClick={() => change.mutate()} disabled={!valid || change.isPending} className="w-full">
          <Save className="h-4 w-4" /> {optional ? 'Save Credential' : 'Save And Unlock'}
        </Button>
        {optional && onClose ? <Button variant="outline" onClick={onClose} className="w-full">Cancel</Button> : null}
        {optional ? (
          <Button
            variant="outline"
            onClick={() => window.confirm('Disable this profile credential? Device management will remain locked until a new password or PIN is configured.') && disable.mutate()}
            disabled={disable.isPending}
            className="w-full text-red-300"
          >
            <Trash2 className="h-4 w-4" /> Disable Credential
          </Button>
        ) : null}
        {next !== confirm && confirm ? <p className="text-sm text-red-400">Credentials do not match.</p> : null}
        {change.error || disable.error ? <ErrorText error={change.error || disable.error} /> : null}
      </div>
    </section>
  )
}

function PairingPanel({ pairing, onClose }: { pairing: DevicePairingChallenge; onClose: () => void }) {
  const [copied, setCopied] = useState(false)
  const copy = async () => {
    await navigator.clipboard.writeText(pairing.pair_command)
    setCopied(true)
  }
  return (
    <section className="rounded-mga border border-mga-accent/40 bg-mga-accent/5 p-5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h3 className="font-bold text-mga-text">Pair a per-user MGA Client</h3>
          <p className="mt-1 text-sm text-mga-muted">Run this once as the OS user who should own the endpoint. Expires {new Date(pairing.expires_at).toLocaleTimeString()}.</p>
        </div>
        <Button variant="outline" size="sm" onClick={onClose}>Close</Button>
      </div>
      <div className="mt-4 flex items-center gap-3 rounded-mga border border-mga-border bg-black/30 p-3">
        <code className="min-w-0 flex-1 overflow-x-auto whitespace-nowrap text-sm text-mga-text">{pairing.pair_command}</code>
        <Button size="sm" onClick={copy}>{copied ? <Check className="h-4 w-4" /> : <Clipboard className="h-4 w-4" />}{copied ? 'Copied' : 'Copy'}</Button>
      </div>
      <Button className="mt-3" onClick={() => { window.location.href = pairing.pair_uri }}>
        <Laptop className="h-4 w-4" /> Open MGA Client
      </Button>
    </section>
  )
}

function DeviceCard({ device }: { device: DeviceEndpoint }) {
  const queryClient = useQueryClient()
  const state = statePresentation[device.status]
  const [name, setName] = useState(device.display_name)
  const commandsQuery = useQuery({
    queryKey: ['device-commands', device.id],
    queryFn: () => listDeviceCommands(device.id),
    refetchInterval: device.status === 'offline' ? false : 2000,
  })
  const action = useMutation({
    mutationFn: (command: 'endpoint.ping' | 'endpoint.refresh') => dispatchDeviceCommand(device.id, command),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['device-commands', device.id] })
      await queryClient.invalidateQueries({ queryKey: ['devices'] })
    },
  })
  const rename = useMutation({
    mutationFn: () => renameDevice(device.id, name),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['devices'] }),
  })
  const revoke = useMutation({
    mutationFn: () => revokeDevice(device.id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['devices'] }),
  })
  const latest = commandsQuery.data?.[0]
  const canManage = accessAllows(device.access_level, 'manage')
  const isOwner = device.access_level === 'owner'
  return (
    <article className={cn('overflow-hidden rounded-mga border bg-mga-surface shadow-lg', state.border)}>
      <div className="p-5">
        <div className="flex items-start justify-between gap-3">
          <div className="flex min-w-0 items-center gap-3">
            <div className="grid h-11 w-11 shrink-0 place-items-center rounded-mga bg-mga-bg"><Laptop className="h-5 w-5 text-mga-accent" /></div>
            <div className="min-w-0">
              <h3 className="truncate text-lg font-black text-mga-text">{device.display_name}</h3>
              <div className="mt-1 flex items-center gap-2 text-sm text-mga-muted"><UserRound className="h-3.5 w-3.5" />{device.os_user} on {device.host_name}</div>
            </div>
          </div>
          <div className={cn('inline-flex items-center gap-2 rounded-full border bg-black/20 px-2.5 py-1 text-xs font-bold', state.border, state.text)}>
            <span className={cn('h-2 w-2 rounded-full', state.dot)} />{state.label}
          </div>
        </div>
        {device.status_reason ? <p className="mt-3 text-sm text-red-300">{device.status_reason}</p> : null}
        <dl className="mt-4 grid grid-cols-2 gap-x-4 gap-y-2 text-sm">
          <DeviceFact label="Platform" value={`${device.platform} / ${device.arch}`} />
          <DeviceFact label="Client" value={device.client_version} />
          <DeviceFact label="Protocol" value={`v${device.protocol_version}`} />
          <DeviceFact label="Access" value={device.access_level} />
          <DeviceFact label="Last seen" value={device.last_seen_at ? new Date(device.last_seen_at).toLocaleString() : 'Never'} />
          <DeviceFact label="Capabilities" value={String(device.capabilities.length)} />
        </dl>
        {isOwner ? (
          <div className="mt-4 flex gap-2">
            <Input aria-label="Device name" value={name} onChange={(event) => setName(event.target.value)} />
            <Button variant="outline" size="sm" onClick={() => rename.mutate()} disabled={!name.trim() || name === device.display_name || rename.isPending}><Save className="h-4 w-4" /> Rename</Button>
          </div>
        ) : null}
        <div className="mt-4 flex flex-wrap gap-2 border-t border-mga-border pt-4">
          <Button size="sm" onClick={() => action.mutate('endpoint.ping')} disabled={device.status === 'offline' || !device.capabilities.includes('endpoint.ping') || action.isPending}><Send className="h-4 w-4" /> Ping</Button>
          <Button variant="outline" size="sm" onClick={() => action.mutate('endpoint.refresh')} disabled={device.status === 'offline' || !canManage || !device.capabilities.includes('endpoint.refresh') || action.isPending}><RefreshCw className="h-4 w-4" /> Refresh</Button>
          {isOwner ? <Button variant="outline" size="sm" onClick={() => window.confirm(`Revoke ${device.display_name}? The client must run "mga-client unpair" before it can pair again.`) && revoke.mutate()} disabled={revoke.isPending} className="ml-auto"><Trash2 className="h-4 w-4" /> Revoke</Button> : null}
        </div>
        {latest ? (
          <div className="mt-3 rounded-mga bg-mga-bg px-3 py-2 text-xs text-mga-muted">
            Latest: <span className="font-mono text-mga-text">{latest.name}</span> · <span className={latest.status === 'succeeded' ? 'text-emerald-300' : latest.status === 'failed' || latest.status === 'rejected' ? 'text-red-300' : 'text-amber-300'}>{latest.status}</span>
            {latest.error_message ? ` · ${latest.error_message}` : ''}
          </div>
        ) : null}
        {action.error || rename.error || revoke.error ? <ErrorText error={action.error || rename.error || revoke.error} /> : null}
        {isOwner ? <DeviceAccessPanel device={device} /> : null}
      </div>
    </article>
  )
}

function DeviceAccessPanel({ device }: { device: DeviceEndpoint }) {
  const { profiles } = useProfiles()
  const queryClient = useQueryClient()
  const [profileId, setProfileId] = useState('')
  const [accessLevel, setAccessLevel] = useState<DeviceAccessLevel>('view')
  const grants = useQuery({ queryKey: ['device-grants', device.id], queryFn: () => listDeviceGrants(device.id) })
  const save = useMutation({
    mutationFn: ({ targetProfileId, level }: { targetProfileId: string; level: DeviceAccessLevel }) => setDeviceGrant(device.id, targetProfileId, level),
    onSuccess: async () => {
      setProfileId('')
      await queryClient.invalidateQueries({ queryKey: ['device-grants', device.id] })
      await queryClient.invalidateQueries({ queryKey: ['devices'] })
    },
  })
  const remove = useMutation({
    mutationFn: (targetProfileId: string) => deleteDeviceGrant(device.id, targetProfileId),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: ['device-grants', device.id] })
      await queryClient.invalidateQueries({ queryKey: ['devices'] })
    },
  })
  const grantedProfiles = new Set(grants.data?.map((grant) => grant.profile_id) ?? [])
  const availableProfiles = profiles.filter((profile) => !grantedProfiles.has(profile.id))
  const levelOptions = [
    { value: 'view', label: 'View' },
    { value: 'play', label: 'Play' },
    { value: 'manage', label: 'Manage' },
    { value: 'owner', label: 'Owner' },
  ]

  return (
    <div className="mt-4 border-t border-mga-border pt-4">
      <h4 className="text-xs font-bold uppercase tracking-[0.18em] text-mga-muted">Profile access</h4>
      <div className="mt-3 space-y-2">
        {grants.data?.map((grant) => (
          <div key={grant.profile_id} className="flex flex-wrap items-center gap-2 rounded-mga bg-mga-bg px-3 py-2">
            <span className="min-w-0 flex-1 truncate text-sm font-semibold text-mga-text">{grant.profile_display_name}</span>
            <Select
              aria-label={`Access for ${grant.profile_display_name}`}
              value={grant.access_level}
              onChange={(event) => save.mutate({ targetProfileId: grant.profile_id, level: event.target.value as DeviceAccessLevel })}
              options={levelOptions}
            />
            <Button variant="outline" size="sm" onClick={() => remove.mutate(grant.profile_id)} disabled={remove.isPending}><Trash2 className="h-4 w-4" /> Remove</Button>
          </div>
        ))}
      </div>
      {availableProfiles.length ? (
        <div className="mt-3 grid gap-2 sm:grid-cols-[1fr_9rem_auto] sm:items-end">
          <Select
            label="Share with profile"
            value={profileId}
            onChange={(event) => setProfileId(event.target.value)}
            options={[{ value: '', label: 'Choose profile' }, ...availableProfiles.map((profile) => ({ value: profile.id, label: profile.display_name }))]}
          />
          <Select label="Access" value={accessLevel} onChange={(event) => setAccessLevel(event.target.value as DeviceAccessLevel)} options={levelOptions} />
          <Button size="sm" onClick={() => save.mutate({ targetProfileId: profileId, level: accessLevel })} disabled={!profileId || save.isPending}><Plus className="h-4 w-4" /> Share</Button>
        </div>
      ) : null}
      {grants.error || save.error || remove.error ? <ErrorText error={grants.error || save.error || remove.error} /> : null}
    </div>
  )
}

function accessAllows(granted: DeviceAccessLevel, required: DeviceAccessLevel): boolean {
  const rank: Record<DeviceAccessLevel, number> = { view: 1, play: 2, manage: 3, owner: 4 }
  return rank[granted] >= rank[required]
}

function DeviceFact({ label, value }: { label: string; value: string }) {
  return <div><dt className="text-xs uppercase tracking-wider text-mga-muted">{label}</dt><dd className="mt-0.5 truncate font-semibold text-mga-text">{value}</dd></div>
}

function PanelMessage({ text }: { text: string }) {
  return <div className="rounded-mga border border-mga-border bg-mga-surface p-6 text-sm text-mga-muted">{text}</div>
}

function ErrorText({ error }: { error: unknown }) {
  return <p className="mt-3 text-sm text-red-400">{error instanceof Error ? error.message : 'The operation failed.'}</p>
}
