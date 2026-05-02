import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  getSetupStatus,
  listProfiles,
  SELECTED_PROFILE_STORAGE_KEY,
  startFreshSetup,
  type Profile,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

type ProfileContextValue = {
  profiles: Profile[]
  currentProfile: Profile | null
  setupRequired: boolean
  selectProfile: (id: string) => void
  clearProfile: () => void
  refreshProfiles: () => void
}

const ProfileContext = createContext<ProfileContextValue | null>(null)

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
    if (!selectedProfileId || currentProfile || profiles.length === 0) return
    writeSelectedProfileId('')
    setSelectedProfileId('')
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
  const [error, setError] = useState('')
  const [saving, setSaving] = useState(false)

  async function submit() {
    setSaving(true)
    setError('')
    try {
      const profile = await startFreshSetup({ display_name: displayName, avatar_key: 'player-1' })
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
    <ProfileGateShell title="Welcome to MGA">
      <div className="space-y-4">
        <Input value={displayName} onChange={(event) => setDisplayName(event.target.value)} aria-label="Profile name" />
        {error ? <p className="text-sm text-red-400">{error}</p> : null}
        <div className="flex gap-2">
          <Button onClick={submit} disabled={saving || displayName.trim() === ''}>
            Start Fresh
          </Button>
          <Button disabled variant="outline" title="Sync restore will land with sync payload v2 import">
            Restore From Sync
          </Button>
        </div>
      </div>
    </ProfileGateShell>
  )
}

function ProfilePicker({ profiles, onSelect }: { profiles: Profile[]; onSelect: (id: string) => void }) {
  return (
    <ProfileGateShell title="Choose Profile">
      <div className="grid gap-2 sm:grid-cols-2">
        {profiles.map((profile) => (
          <button
            key={profile.id}
            type="button"
            onClick={() => onSelect(profile.id)}
            className="flex items-center gap-3 rounded-mga border border-mga-border bg-mga-surface p-3 text-left transition hover:bg-mga-elevated"
          >
            <ProfileAvatar profile={profile} />
            <div>
              <div className="font-medium text-mga-text">{profile.display_name}</div>
              <div className="text-xs text-mga-muted">{profile.role === 'admin_player' ? 'Admin Player' : 'Player'}</div>
            </div>
          </button>
        ))}
      </div>
    </ProfileGateShell>
  )
}

function ProfileGateShell({ title, children }: { title: string; children?: ReactNode }) {
  return (
    <div className="flex min-h-screen items-center justify-center bg-mga-bg p-4 font-mga text-mga-text">
      <div className="w-full max-w-md space-y-5 rounded-mga border border-mga-border bg-mga-surface p-5 shadow-xl">
        <div className="flex items-center gap-3">
          <img src="/logo.png" alt="" width={40} height={40} className="h-10 w-10 object-contain" />
          <h1 className="text-xl font-semibold">{title}</h1>
        </div>
        {children}
      </div>
    </div>
  )
}

export function ProfileAvatar({ profile, className = '' }: { profile: Profile; className?: string }) {
  const label = profile.display_name.trim().slice(0, 1).toUpperCase() || 'P'
  return (
    <span className={`grid h-9 w-9 shrink-0 place-items-center rounded-full bg-mga-accent text-sm font-bold text-mga-bg ${className}`}>
      {label}
    </span>
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
