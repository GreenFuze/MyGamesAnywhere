import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, ArrowLeft, ArrowRight, CloudDownload, Gamepad2, Gem, Joystick, Rocket, Swords, Trophy } from 'lucide-react'
import {
  browseRestoreSyncSetup,
  checkRestoreSyncSetup,
  getSetupStatus,
  importOAuthCallback,
  isRestoreSyncOAuthRequired,
  listRestoreSyncPoints,
  listProfiles,
  restoreSyncSetup,
  SELECTED_PROFILE_STORAGE_KEY,
  startFreshSetup,
  type Profile,
  type RestoreSyncSetupOAuthRequired,
  type RestoreSyncPoint,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { FolderBrowser } from '@/components/settings/FolderBrowser'
import { OAuthCallbackPanel } from '@/components/settings/OAuthCallbackPanel'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { useSSE } from '@/hooks/useSSE'
import { pluginLabel } from '@/lib/gameUtils'

type ProfileContextValue = {
  profiles: Profile[]
  currentProfile: Profile | null
  setupRequired: boolean
  selectProfile: (id: string) => void
  clearProfile: () => void
  refreshProfiles: () => void
}

const ProfileContext = createContext<ProfileContextValue | null>(null)
const GOOGLE_DRIVE_SYNC_PLUGIN_ID = 'sync-settings-google-drive'
const DEFAULT_GOOGLE_DRIVE_SYNC_PATH = 'Games/mga_sync'

export const PROFILE_AVATARS = [
  { key: 'player-1', label: 'Arcade', Icon: Gamepad2, tone: 'from-sky-400 to-cyan-300', ring: 'ring-sky-300/35' },
  { key: 'player-2', label: 'Quest', Icon: Swords, tone: 'from-rose-400 to-orange-300', ring: 'ring-rose-300/35' },
  { key: 'player-3', label: 'Launch', Icon: Rocket, tone: 'from-violet-400 to-fuchsia-300', ring: 'ring-violet-300/35' },
  { key: 'player-4', label: 'Legend', Icon: Trophy, tone: 'from-amber-300 to-yellow-200', ring: 'ring-amber-200/40' },
  { key: 'player-5', label: 'Retro', Icon: Joystick, tone: 'from-emerald-300 to-lime-200', ring: 'ring-emerald-200/35' },
  { key: 'player-6', label: 'Vault', Icon: Gem, tone: 'from-indigo-300 to-blue-200', ring: 'ring-indigo-200/35' },
] as const

export type ProfileAvatarKey = (typeof PROFILE_AVATARS)[number]['key']

export function profileAvatarFor(key?: string) {
  return PROFILE_AVATARS.find((avatar) => avatar.key === key) ?? PROFILE_AVATARS[0]
}

export function useProfiles() {
  const ctx = useContext(ProfileContext)
  if (!ctx) {
    throw new Error('useProfiles must be used within ProfileProvider')
  }
  return ctx
}

export function ProfileProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient()
  const [selectedProfileId, setSelectedProfileId] = useState(() => readSelectedProfileId())

  const setupQuery = useQuery({
    queryKey: ['setup-status'],
    queryFn: getSetupStatus,
    retry: 0,
  })
  const profilesQuery = useQuery({
    queryKey: ['profiles'],
    queryFn: listProfiles,
    enabled: setupQuery.data?.setup_required === false,
    retry: 0,
  })

  const profiles = setupQuery.data?.setup_required ? [] : (profilesQuery.data ?? setupQuery.data?.profiles ?? [])
  const currentProfile = profiles.find((profile) => profile.id === selectedProfileId) ?? null
  const setupRequired = setupQuery.data?.setup_required === true

  useEffect(() => {
    if (currentProfile || profiles.length === 0) return
    if (selectedProfileId) {
      writeSelectedProfileId('')
      setSelectedProfileId('')
    }
  }, [currentProfile, profiles.length, selectedProfileId])

  const selectProfile = useCallback((id: string) => {
    writeSelectedProfileId(id)
    setSelectedProfileId(id)
    queryClient.invalidateQueries()
  }, [queryClient])

  const clearProfile = useCallback(() => {
    writeSelectedProfileId('')
    setSelectedProfileId('')
    queryClient.invalidateQueries()
  }, [queryClient])

  const refreshProfiles = useCallback(() => {
    queryClient.invalidateQueries({ queryKey: ['profiles'] })
    queryClient.invalidateQueries({ queryKey: ['setup-status'] })
  }, [queryClient])

  const value = useMemo<ProfileContextValue>(() => ({
    profiles,
    currentProfile,
    setupRequired,
    selectProfile,
    clearProfile,
    refreshProfiles,
  }), [clearProfile, currentProfile, profiles, refreshProfiles, selectProfile, setupRequired])

  if (setupQuery.isLoading) {
    return <ProfileGateShell title="Loading MGA" />
  }
  if (setupRequired) {
    return <FirstRunWizard onCreated={selectProfile} />
  }
  if (!currentProfile) {
    return (
      <ProfileContext.Provider value={value}>
        <ProfilePicker profiles={profiles} onSelect={selectProfile} />
      </ProfileContext.Provider>
    )
  }
  return <ProfileContext.Provider value={value}>{children}</ProfileContext.Provider>
}

function FirstRunWizard({ onCreated }: { onCreated: (id: string) => void }) {
  const queryClient = useQueryClient()
  const { subscribe } = useSSE()
  const [mode, setMode] = useState<'choose' | 'fresh' | 'restore'>('choose')
  const [displayName, setDisplayName] = useState('Admin Player')
  const [avatarKey, setAvatarKey] = useState<ProfileAvatarKey>('player-1')
  const [syncPath, setSyncPath] = useState(DEFAULT_GOOGLE_DRIVE_SYNC_PATH)
  const [passphrase, setPassphrase] = useState('')
  const [oauthState, setOauthState] = useState('')
  const [oauthResponse, setOauthResponse] = useState<RestoreSyncSetupOAuthRequired | null>(null)
  const [driveConnected, setDriveConnected] = useState(false)
  const [showFolderBrowser, setShowFolderBrowser] = useState(false)
  const [restorePoints, setRestorePoints] = useState<RestoreSyncPoint[]>([])
  const [selectedPayloadId, setSelectedPayloadId] = useState('')
  const [selectedIntegrationKeys, setSelectedIntegrationKeys] = useState<Set<string>>(new Set())
  const [message, setMessage] = useState('')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  const selectedRestorePoint = restorePoints.find((point) => point.id === selectedPayloadId) ?? restorePoints[0] ?? null
  const restoreConfig = useCallback((pathOverride?: string) => ({
    sync_path: (pathOverride ?? syncPath).trim() || DEFAULT_GOOGLE_DRIVE_SYNC_PATH,
    max_versions: 10,
  }), [syncPath])

  const loadRestorePoints = useCallback(async (pathOverride?: string) => {
    setSaving(true)
    setError('')
    try {
      const result = await listRestoreSyncPoints({
        plugin_id: GOOGLE_DRIVE_SYNC_PLUGIN_ID,
        label: 'Google Drive Sync',
        integration_type: 'sync',
        config: restoreConfig(pathOverride),
        passphrase: '',
      })
      if (result.status === 'oauth_required') {
        setOauthState(result.state)
        setOauthResponse(result)
        setDriveConnected(false)
        setMessage('Finish Google Drive sign-in in the browser, then choose the sync folder.')
        return
      }
      const payloads = [...(result.payloads ?? [])].sort((a, b) => Number(b.is_latest) - Number(a.is_latest))
      setRestorePoints(payloads)
      const latest = payloads.find((point) => point.is_latest) ?? payloads[0]
      setSelectedPayloadId(latest?.id ?? '')
      setSelectedIntegrationKeys(new Set(latest?.integrations.map((integration) => integration.key) ?? []))
      setMessage(payloads.length > 0 ? 'Choose the restore point and integrations to import.' : 'No sync payloads were found in this folder.')
    } catch (err) {
      setError(apiErrorText(err, 'Failed to list restore points'))
    } finally {
      setSaving(false)
    }
  }, [restoreConfig])

  async function submit() {
    setSaving(true)
    setError('')
    try {
      const profile = await startFreshSetup({ display_name: displayName, avatar_key: avatarKey })
      await queryClient.invalidateQueries({ queryKey: ['setup-status'] })
      await queryClient.invalidateQueries({ queryKey: ['profiles'] })
      onCreated(profile.id)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Setup failed')
    } finally {
      setSaving(false)
    }
  }

  const submitRestore = useCallback(async () => {
    setSaving(true)
    setError('')
    setMessage('')
    try {
      const result = await restoreSyncSetup({
        plugin_id: GOOGLE_DRIVE_SYNC_PLUGIN_ID,
        label: 'Google Drive Sync',
        integration_type: 'sync',
        config: restoreConfig(),
        passphrase,
        store_key: true,
        payload_id: selectedRestorePoint?.id,
        payload_name: selectedRestorePoint?.name,
        selected_integrations: Array.from(selectedIntegrationKeys),
      })
      if (isRestoreSyncOAuthRequired(result)) {
        setOauthState(result.state)
        setOauthResponse(result)
        setDriveConnected(false)
        setMessage('Finish Google Drive sign-in in the browser, then choose the sync folder.')
        return
      }
      await queryClient.invalidateQueries({ queryKey: ['setup-status'] })
      await queryClient.invalidateQueries({ queryKey: ['profiles'] })
      onCreated(result.profile_id)
    } catch (err) {
      setError(apiErrorText(err, 'Restore failed'))
    } finally {
      setSaving(false)
    }
  }, [onCreated, passphrase, queryClient, restoreConfig, selectedIntegrationKeys, selectedRestorePoint])

  const connectGoogleDrive = useCallback(async () => {
    setSaving(true)
    setError('')
    setMessage('')
    const authWindow = window.open('', '_blank')
    try {
      const result = await checkRestoreSyncSetup({
        plugin_id: GOOGLE_DRIVE_SYNC_PLUGIN_ID,
        label: 'Google Drive Sync',
        integration_type: 'sync',
        config: restoreConfig(),
        passphrase: '',
      })
      if (isRestoreSyncOAuthRequired(result)) {
        setOauthState(result.state)
        setOauthResponse(result)
        setMessage('Finish Google Drive sign-in in the browser, then choose the sync folder.')
        if (authWindow) {
          authWindow.location.href = result.authorize_url
        } else {
          setError('Browser blocked the Google sign-in popup. Allow popups for MGA and try again.')
        }
        return
      }
      authWindow?.close()
      setDriveConnected(true)
      if (!syncPath.trim()) setSyncPath(DEFAULT_GOOGLE_DRIVE_SYNC_PATH)
      setMessage('Google Drive connected. Choose the sync folder that contains latest.json, then restore.')
      await loadRestorePoints()
    } catch (err) {
      authWindow?.close()
      setError(apiErrorText(err, 'Google Drive sign-in failed'))
    } finally {
      setSaving(false)
    }
  }, [loadRestorePoints, restoreConfig, syncPath])

  useEffect(() => {
    if (!oauthState) return
    const unsubComplete = subscribe('oauth_complete', (data: unknown) => {
      const d = data as { state?: string }
      if (d.state === oauthState) {
        setOauthState('')
        setOauthResponse(null)
        setDriveConnected(true)
        if (!syncPath.trim()) setSyncPath(DEFAULT_GOOGLE_DRIVE_SYNC_PATH)
        setMessage('Google Drive connected. Choose the sync folder that contains latest.json, then restore.')
        void loadRestorePoints()
      }
    })
    const unsubError = subscribe('oauth_error', (data: unknown) => {
      const d = data as { state?: string; error?: string }
      if (d.state === oauthState) {
        setError(d.error ?? 'Authentication failed')
        setOauthState('')
        setOauthResponse(null)
      }
    })
    return () => { unsubComplete(); unsubError() }
  }, [loadRestorePoints, oauthState, subscribe, syncPath])

  const reopenRestoreOAuthWindow = useCallback(() => {
    if (!oauthResponse) return
    const authWindow = window.open('', '_blank')
    if (authWindow) {
      authWindow.location.href = oauthResponse.authorize_url
      setError('')
    } else {
      setError('Browser blocked the Google sign-in popup. Allow popups for MGA and try again.')
    }
  }, [oauthResponse])

  const submitRestoreOAuthCallback = useCallback(async (callbackUrl: string) => {
    if (!oauthResponse) return
    setSaving(true)
    setError('')
    try {
      await importOAuthCallback(GOOGLE_DRIVE_SYNC_PLUGIN_ID, callbackUrl)
      setOauthState('')
      setOauthResponse(null)
      setDriveConnected(true)
      if (!syncPath.trim()) setSyncPath(DEFAULT_GOOGLE_DRIVE_SYNC_PATH)
      setMessage('Google Drive connected. Choose the sync folder that contains latest.json, then restore.')
      await loadRestorePoints()
    } catch (err) {
      setError(apiErrorText(err, 'Failed to import callback URL'))
    } finally {
      setSaving(false)
    }
  }, [loadRestorePoints, oauthResponse, syncPath])

  if (mode === 'choose') {
    return (
      <ProfileGateShell eyebrow="First Run" title="Set Up MGA">
        <div className="grid gap-3 sm:grid-cols-2">
          <button
            type="button"
            onClick={() => setMode('fresh')}
            className="group min-h-44 rounded-mga border border-mga-border bg-mga-bg p-4 text-left transition hover:-translate-y-0.5 hover:border-mga-accent/70 hover:bg-mga-elevated focus:outline-none focus-visible:ring-2 focus-visible:ring-mga-accent"
          >
            <span className="grid h-11 w-11 place-items-center rounded-full bg-mga-accent text-mga-bg">
              <ArrowRight className="h-5 w-5" />
            </span>
            <div className="mt-5 text-xl font-black text-mga-text">Start Fresh</div>
            <p className="mt-2 text-sm leading-6 text-mga-muted">Create the first Admin Player profile and configure integrations later.</p>
          </button>
          <button
            type="button"
            onClick={() => setMode('restore')}
            className="group min-h-44 rounded-mga border border-mga-border bg-mga-bg p-4 text-left transition hover:-translate-y-0.5 hover:border-mga-accent/70 hover:bg-mga-elevated focus:outline-none focus-visible:ring-2 focus-visible:ring-mga-accent"
          >
            <span className="grid h-11 w-11 place-items-center rounded-full bg-gradient-to-br from-cyan-300 to-emerald-200 text-mga-bg">
              <CloudDownload className="h-5 w-5" />
            </span>
            <div className="mt-5 text-xl font-black text-mga-text">Restore From Sync</div>
            <p className="mt-2 text-sm leading-6 text-mga-muted">Connect a settings sync integration and restore profiles, integrations, and settings from it.</p>
          </button>
        </div>
      </ProfileGateShell>
    )
  }

  if (mode === 'restore') {
    return (
      <ProfileGateShell eyebrow="First Run" title="Restore From Sync">
        <div className="space-y-5">
          <div className="rounded-mga border border-amber-300/40 bg-amber-300/10 p-4 text-sm leading-6 text-amber-100">
            <div className="flex items-start gap-3">
              <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0" />
              <p>This restore initializes this new MGA instance from the remote settings sync payload. Existing remote integrations and settings will be imported; old sync payloads create one Admin Player profile. Games are rebuilt later by scanning the restored integrations.</p>
            </div>
          </div>
          <div className="rounded-mga border border-mga-border/80 bg-mga-bg/65 p-4 shadow-inner">
            <div className="mb-4 rounded-mga border border-mga-border/70 bg-mga-surface/70 p-3">
              <div className="flex items-center gap-3">
                <span className="grid h-10 w-10 place-items-center rounded-full bg-gradient-to-br from-sky-300 to-emerald-200 text-mga-bg">
                  <CloudDownload className="h-5 w-5" />
                </span>
                <div>
                  <div className="text-sm font-bold text-mga-text">Google Drive Settings Sync</div>
                  <div className="text-xs leading-5 text-mga-muted">First sign in to Google Drive, then choose the folder that contains <span className="font-mono text-mga-text">latest.json</span>. MGA will save this as your Settings Sync integration after restore.</div>
                </div>
              </div>
            </div>
            <div className="space-y-4">
              {!driveConnected ? (
                <Button type="button" onClick={connectGoogleDrive} disabled={saving || Boolean(oauthState)} className="h-12 w-full justify-between px-4">
                  <span>{oauthState ? 'Waiting For Google Sign-In' : 'Sign In To Google Drive'}</span>
                  <CloudDownload className="h-4 w-4" />
                </Button>
              ) : (
                <div className="space-y-3">
                  <div className="grid gap-2 sm:grid-cols-[1fr_auto] sm:items-end">
                    <Input
                      label="Google Drive folder path"
                      value={syncPath}
                      onChange={(event) => setSyncPath(event.target.value)}
                      className="h-11"
                      placeholder={DEFAULT_GOOGLE_DRIVE_SYNC_PATH}
                    />
                    <Button type="button" variant="outline" onClick={() => setShowFolderBrowser((value) => !value)} className="h-11">
                      {showFolderBrowser ? 'Hide Browse' : 'Browse'}
                    </Button>
                  </div>
                  {showFolderBrowser ? (
                    <div className="rounded-mga border border-mga-border bg-mga-surface/70 p-3">
                      <FolderBrowser
                        pluginId={GOOGLE_DRIVE_SYNC_PLUGIN_ID}
                        initialPath={syncPath.trim()}
                        onSelect={(path) => {
                          const nextPath = path || DEFAULT_GOOGLE_DRIVE_SYNC_PATH
                          setSyncPath(nextPath)
                          setShowFolderBrowser(false)
                          void loadRestorePoints(nextPath)
                        }}
                        browse={(path) => browseRestoreSyncSetup(GOOGLE_DRIVE_SYNC_PLUGIN_ID, path)}
                      />
                    </div>
                  ) : null}
                </div>
              )}
              {oauthResponse ? (
                <OAuthCallbackPanel
                  providerLabel="Google Drive"
                  authorizeUrl={oauthResponse.authorize_url}
                  remoteBrowserHint={oauthResponse.remote_browser_hint}
                  pasteCallbackSupported={oauthResponse.paste_callback_supported}
                  busy={saving}
                  error={error || null}
                  onOpenSignIn={reopenRestoreOAuthWindow}
                  onSubmitCallback={submitRestoreOAuthCallback}
                  onCancel={() => {
                    setOauthState('')
                    setOauthResponse(null)
                    setError('')
                  }}
                />
              ) : null}
              <Input
                label="Sync encryption passphrase"
                type="password"
                value={passphrase}
                onChange={(event) => setPassphrase(event.target.value)}
                className="h-11"
                placeholder="Leave blank to use a stored key on this PC"
              />
              {driveConnected ? (
                <div className="space-y-3 rounded-mga border border-mga-border/70 bg-mga-surface/70 p-3">
                  <div className="flex items-center justify-between gap-3">
                    <div>
                      <div className="text-sm font-bold text-mga-text">Restore point</div>
                      <div className="text-xs text-mga-muted">Choose which sync JSON to restore. latest.json is selected by default.</div>
                    </div>
                    <Button type="button" variant="outline" size="sm" onClick={() => { void loadRestorePoints() }} disabled={saving}>
                      Refresh
                    </Button>
                  </div>
                  {restorePoints.length > 0 ? (
                    <select
                      value={selectedPayloadId}
                      onChange={(event) => {
                        const nextID = event.target.value
                        const point = restorePoints.find((item) => item.id === nextID)
                        setSelectedPayloadId(nextID)
                        setSelectedIntegrationKeys(new Set(point?.integrations.map((integration) => integration.key) ?? []))
                      }}
                      className="h-11 w-full rounded-mga border border-mga-border bg-mga-bg px-3 text-sm text-mga-text"
                    >
                      {restorePoints.map((point) => (
                        <option key={point.id || point.name} value={point.id}>
                          {point.name}{point.is_latest ? ' (latest)' : ''} - {point.integration_count} integrations
                        </option>
                      ))}
                    </select>
                  ) : (
                    <p className="text-sm text-mga-muted">No restore points loaded yet.</p>
                  )}
                  {selectedRestorePoint ? (
                    <div className="space-y-2">
                      <div className="text-xs leading-5 text-mga-muted">
                        Version {selectedRestorePoint.version || 'unknown'} · {selectedRestorePoint.profile_count || 1} profile(s) · {selectedRestorePoint.exported_at ? new Date(selectedRestorePoint.exported_at).toLocaleString() : 'unknown export time'}
                      </div>
                      <div className="max-h-44 space-y-2 overflow-y-auto rounded-mga border border-mga-border bg-mga-bg p-2">
                        {selectedRestorePoint.integrations.map((integration) => (
                          <label key={integration.key} className="flex items-center gap-2 rounded-mga px-2 py-1.5 text-sm text-mga-text hover:bg-mga-elevated">
                            <input
                              type="checkbox"
                              checked={selectedIntegrationKeys.has(integration.key)}
                              onChange={(event) => {
                                setSelectedIntegrationKeys((prev) => {
                                  const next = new Set(prev)
                                  if (event.target.checked) next.add(integration.key)
                                  else next.delete(integration.key)
                                  return next
                                })
                              }}
                            />
                            <span className="min-w-0 flex-1 truncate">{integration.label || pluginLabel(integration.plugin_id)}</span>
                            <span className="text-xs text-mga-muted">{pluginLabel(integration.plugin_id)}</span>
                          </label>
                        ))}
                      </div>
                    </div>
                  ) : null}
                </div>
              ) : null}
              <p className="text-xs leading-5 text-mga-muted">Integration configs inside the sync file are encrypted. Enter the passphrase used when this sync was pushed, or leave it blank if this Windows user already has the stored MGA sync key. The default folder path comes from the Google Drive settings-sync plugin and can be changed before restore.</p>
            </div>
          </div>
          {message ? <p className="text-sm text-mga-muted">{message}</p> : null}
          {error ? <p className="text-sm text-red-400">{error}</p> : null}
          <div className="flex flex-col gap-3 sm:flex-row">
            <Button type="button" variant="outline" onClick={() => setMode('choose')} className="h-12 px-4">
              <ArrowLeft className="h-4 w-4" />
              <span>Back</span>
            </Button>
            <Button onClick={submitRestore} disabled={saving || Boolean(oauthState) || !driveConnected || !selectedRestorePoint} className="h-12 flex-1 justify-between px-4">
              <span>{driveConnected ? 'Restore From Selected Folder' : 'Sign In First'}</span>
              <CloudDownload className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </ProfileGateShell>
    )
  }

  return (
    <ProfileGateShell eyebrow="First Run" title="Build Your First Profile">
      <div className="space-y-5">
        <div className="rounded-mga border border-mga-border/80 bg-mga-bg/65 p-4 shadow-inner">
          <Input
            label="Profile name"
            value={displayName}
            onChange={(event) => setDisplayName(event.target.value)}
            aria-label="Profile name"
            className="h-11 text-base"
          />
          <AvatarChooser value={avatarKey} onChange={setAvatarKey} className="mt-4" />
        </div>
        {error ? <p className="text-sm text-red-400">{error}</p> : null}
        <div className="flex flex-col gap-3 sm:flex-row">
          <Button type="button" variant="outline" onClick={() => setMode('choose')} className="h-12 px-4">
            <ArrowLeft className="h-4 w-4" />
            <span>Back</span>
          </Button>
          <Button onClick={submit} disabled={saving || displayName.trim() === ''} className="h-12 flex-1 justify-between px-4">
            <span>Start Fresh</span>
            <ArrowRight className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </ProfileGateShell>
  )
}

function apiErrorText(err: unknown, fallback: string) {
  if (err && typeof err === 'object' && 'responseText' in err) {
    const text = String((err as { responseText?: string }).responseText ?? '').trim()
    if (text) return text
  }
  return err instanceof Error ? err.message : fallback
}

function ProfilePicker({ profiles, onSelect }: { profiles: Profile[]; onSelect: (id: string) => void }) {
  return (
    <ProfileGateShell eyebrow="Profile Select" title="Choose Your Player">
      <div className="grid gap-3 sm:grid-cols-2">
        {profiles.map((profile) => (
          <button
            key={profile.id}
            type="button"
            onClick={() => onSelect(profile.id)}
            className="group relative min-h-32 overflow-hidden rounded-mga border border-mga-border bg-mga-bg p-4 text-left shadow-lg transition hover:-translate-y-0.5 hover:border-mga-accent/70 hover:bg-mga-elevated focus:outline-none focus-visible:ring-2 focus-visible:ring-mga-accent"
          >
            <div className="absolute inset-x-0 top-0 h-1 bg-mga-accent opacity-75" />
            <div className="flex items-start justify-between gap-3">
              <ProfileAvatar profile={profile} className="h-14 w-14" />
              <span className="rounded-full border border-mga-border bg-mga-surface px-2 py-1 text-[0.68rem] font-semibold uppercase tracking-[0.16em] text-mga-muted">
                {profile.role === 'admin_player' ? 'Admin' : 'Player'}
              </span>
            </div>
            <div className="mt-5 min-w-0">
              <div className="truncate text-xl font-bold text-mga-text">{profile.display_name}</div>
              <div className="mt-2 flex items-center gap-2 text-xs font-medium uppercase tracking-[0.18em] text-mga-accent">
                <span>Enter</span>
                <ArrowRight className="h-3.5 w-3.5 transition group-hover:translate-x-1" />
              </div>
            </div>
          </button>
        ))}
      </div>
    </ProfileGateShell>
  )
}

function ProfileGateShell({ eyebrow, title, children }: { eyebrow?: string; title: string; children?: ReactNode }) {
  return (
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden bg-mga-bg p-4 font-mga text-mga-text">
      <div className="absolute inset-0 bg-[linear-gradient(90deg,rgba(255,255,255,0.035)_1px,transparent_1px),linear-gradient(0deg,rgba(255,255,255,0.028)_1px,transparent_1px)] bg-[size:44px_44px]" />
      <div className="absolute inset-0 bg-[linear-gradient(135deg,transparent_0%,transparent_46%,rgba(61,184,255,0.13)_46%,rgba(61,184,255,0.13)_50%,transparent_50%,transparent_100%)]" />
      <div className="relative w-full max-w-3xl overflow-hidden rounded-mga border border-mga-border bg-mga-surface/92 shadow-2xl">
        <div className="absolute left-0 top-0 h-full w-1 bg-mga-accent" />
        <div className="grid gap-0 md:grid-cols-[16rem_1fr]">
          <div className="relative border-b border-mga-border bg-mga-bg/70 p-6 md:border-b-0 md:border-r">
            <div className="absolute inset-0 bg-[linear-gradient(160deg,rgba(255,255,255,0.07),transparent_42%)]" />
            <div className="relative space-y-5">
              <img src="/logo.png" alt="" width={56} height={56} className="h-14 w-14 object-contain drop-shadow" />
              <div>
                {eyebrow ? <div className="text-xs font-bold uppercase tracking-[0.28em] text-mga-accent">{eyebrow}</div> : null}
                <h1 className="mt-2 text-3xl font-black leading-tight text-mga-text">{title}</h1>
              </div>
              <div className="grid grid-cols-3 gap-2 opacity-80">
                {PROFILE_AVATARS.slice(0, 6).map(({ key, Icon, tone }) => (
                  <div key={key} className={cn('grid h-11 place-items-center rounded-mga bg-gradient-to-br text-mga-bg', tone)}>
                    <Icon className="h-5 w-5" />
                  </div>
                ))}
              </div>
            </div>
          </div>
          <div className="p-5 sm:p-7">
            {children}
          </div>
        </div>
      </div>
    </div>
  )
}

export function ProfileAvatar({ profile, className = '' }: { profile: Profile; className?: string }) {
  const avatar = profileAvatarFor(profile.avatar_key)
  const Icon = avatar.Icon
  return (
    <span
      className={cn(
        'grid h-9 w-9 shrink-0 place-items-center rounded-full bg-gradient-to-br text-mga-bg shadow-lg ring-2',
        avatar.tone,
        avatar.ring,
        className,
      )}
      title={avatar.label}
    >
      <Icon className="h-[52%] w-[52%]" />
    </span>
  )
}

export function AvatarChooser({
  value,
  onChange,
  className,
}: {
  value: string
  onChange: (value: ProfileAvatarKey) => void
  className?: string
}) {
  return (
    <div className={cn('space-y-2', className)}>
      <div className="text-sm font-medium text-mga-text">Avatar</div>
      <div className="grid grid-cols-3 gap-2 sm:grid-cols-6">
        {PROFILE_AVATARS.map((avatar) => {
          const Icon = avatar.Icon
          const selected = avatar.key === value
          return (
            <button
              key={avatar.key}
              type="button"
              onClick={() => onChange(avatar.key)}
              className={cn(
                'grid h-12 place-items-center rounded-mga border bg-mga-bg transition hover:border-mga-accent focus:outline-none focus-visible:ring-2 focus-visible:ring-mga-accent',
                selected ? 'border-mga-accent ring-2 ring-mga-accent/30' : 'border-mga-border',
              )}
              title={avatar.label}
              aria-label={avatar.label}
            >
              <span className={cn('grid h-8 w-8 place-items-center rounded-full bg-gradient-to-br text-mga-bg', avatar.tone)}>
                <Icon className="h-4 w-4" />
              </span>
            </button>
          )
        })}
      </div>
    </div>
  )
}

function readSelectedProfileId(): string {
  try {
    return localStorage.getItem(SELECTED_PROFILE_STORAGE_KEY) ?? ''
  } catch {
    return ''
  }
}

function writeSelectedProfileId(id: string) {
  try {
    if (id) localStorage.setItem(SELECTED_PROFILE_STORAGE_KEY, id)
    else localStorage.removeItem(SELECTED_PROFILE_STORAGE_KEY)
  } catch {
    // Browser storage can be unavailable in private or embedded contexts.
  }
}
