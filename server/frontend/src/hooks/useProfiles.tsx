import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowRight, CloudDownload, Gamepad2, Gem, Joystick, Rocket, Swords, Trophy } from 'lucide-react'
import {
  getSetupStatus,
  listProfiles,
  SELECTED_PROFILE_STORAGE_KEY,
  startFreshSetup,
  type Profile,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

type ProfileContextValue = {
  profiles: Profile[]
  currentProfile: Profile | null
  setupRequired: boolean
  selectProfile: (id: string) => void
  clearProfile: () => void
  refreshProfiles: () => void
}

const ProfileContext = createContext<ProfileContextValue | null>(null)

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
  const [displayName, setDisplayName] = useState('Admin Player')
  const [avatarKey, setAvatarKey] = useState<ProfileAvatarKey>('player-1')
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

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
        <div className="grid gap-3 sm:grid-cols-2">
          <Button onClick={submit} disabled={saving || displayName.trim() === ''} className="h-12 justify-between px-4">
            <span>Start Fresh</span>
            <ArrowRight className="h-4 w-4" />
          </Button>
          <Button disabled variant="outline" title="Sync restore will land with sync payload v2 import" className="h-12 justify-between px-4">
            <span>Restore From Sync</span>
            <CloudDownload className="h-4 w-4" />
          </Button>
        </div>
      </div>
    </ProfileGateShell>
  )
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
