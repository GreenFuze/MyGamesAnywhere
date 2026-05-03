import { useEffect, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Check, Plus, Save, ShieldCheck, Trash2, UserRound, X } from 'lucide-react'
import { createProfile, deleteProfile, updateProfile, type Profile, type ProfileRole } from '@/api/client'
import { AvatarChooser, ProfileAvatar, profileAvatarFor, useProfiles, type ProfileAvatarKey } from '@/hooks/useProfiles'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { cn } from '@/lib/utils'

const roles: Array<{ value: ProfileRole; label: string }> = [
  { value: 'admin_player', label: 'Admin Player' },
  { value: 'player', label: 'Player' },
]

type ProfileDraft = {
  display_name: string
  avatar_key: ProfileAvatarKey
  role: ProfileRole
}

export function ProfilesTab() {
  const queryClient = useQueryClient()
  const { profiles, currentProfile, refreshProfiles } = useProfiles()
  const [creating, setCreating] = useState(false)

  const refresh = async () => {
    refreshProfiles()
    await queryClient.invalidateQueries()
  }

  return (
    <div className="space-y-5">
      {currentProfile ? (
        <section className="overflow-hidden rounded-mga border border-mga-border bg-mga-surface shadow-xl">
          <div className="grid gap-0 lg:grid-cols-[18rem_1fr]">
            <div className="relative border-b border-mga-border bg-mga-bg/80 p-5 lg:border-b-0 lg:border-r">
              <ProfileBackdrop profile={currentProfile} />
              <div className="relative">
                <div className="flex items-center gap-4">
                  <ProfileAvatar profile={currentProfile} className="h-16 w-16" />
                  <div className="min-w-0">
                    <div className="text-xs font-bold uppercase tracking-[0.24em] text-mga-accent">Current Profile</div>
                    <h2 className="mt-1 truncate text-2xl font-black text-mga-text">{currentProfile.display_name}</h2>
                    <div className="mt-2 inline-flex items-center gap-1.5 rounded-full border border-mga-border bg-mga-surface px-2.5 py-1 text-xs font-semibold text-mga-muted">
                      <ShieldCheck className="h-3.5 w-3.5" />
                      {roleLabel(currentProfile.role)}
                    </div>
                  </div>
                </div>
              </div>
            </div>
            <ProfileEditor profile={currentProfile} onChanged={refresh} compact />
          </div>
        </section>
      ) : null}

      <section className="rounded-mga border border-mga-border bg-mga-surface p-4 shadow-lg">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 className="text-lg font-bold text-mga-text">Profiles</h2>
            <div className="mt-1 text-sm text-mga-muted">{profiles.length} total</div>
          </div>
          <Button onClick={() => setCreating((value) => !value)} className="shrink-0">
            {creating ? <X className="h-4 w-4" /> : <Plus className="h-4 w-4" />}
            {creating ? 'Close' : 'Create Profile'}
          </Button>
        </div>

        {creating ? (
          <CreateProfilePanel
            onCancel={() => setCreating(false)}
            onCreated={async () => {
              setCreating(false)
              await refresh()
            }}
          />
        ) : null}

        <div className="mt-4 grid gap-3 xl:grid-cols-2">
          {profiles.map((profile) => (
            <ManagedProfileCard
              key={profile.id}
              profile={profile}
              isCurrent={profile.id === currentProfile?.id}
              onChanged={refresh}
            />
          ))}
        </div>
      </section>
    </div>
  )
}

function CreateProfilePanel({ onCancel, onCreated }: { onCancel: () => void; onCreated: () => void }) {
  const [draft, setDraft] = useState<ProfileDraft>({
    display_name: '',
    avatar_key: 'player-2',
    role: 'player',
  })
  const [error, setError] = useState('')

  const create = useMutation({
    mutationFn: () => createProfile(draft),
    onSuccess: onCreated,
    onError: (err) => setError(errorText(err, 'Create failed')),
  })

  return (
    <div className="mt-4 rounded-mga border border-mga-border bg-mga-bg/70 p-4">
      <div className="grid gap-4 lg:grid-cols-[1fr_13rem]">
        <div className="space-y-4">
          <Input
            label="Profile name"
            value={draft.display_name}
            onChange={(event) => setDraft({ ...draft, display_name: event.target.value })}
            placeholder="Player name"
          />
          <AvatarChooser
            value={draft.avatar_key}
            onChange={(avatar_key) => setDraft({ ...draft, avatar_key })}
          />
        </div>
        <div className="space-y-4">
          <Select
            label="Role"
            value={draft.role}
            onChange={(event) => setDraft({ ...draft, role: event.target.value as ProfileRole })}
            options={roles}
          />
          <div className="flex gap-2 pt-1 lg:pt-6">
            <Button
              onClick={() => {
                setError('')
                create.mutate()
              }}
              disabled={draft.display_name.trim() === '' || create.isPending}
              className="flex-1"
            >
              <Plus className="h-4 w-4" />
              Create
            </Button>
            <Button variant="outline" onClick={onCancel} disabled={create.isPending}>
              Cancel
            </Button>
          </div>
        </div>
      </div>
      {error ? <p className="mt-3 text-sm text-red-400">{error}</p> : null}
    </div>
  )
}

function ManagedProfileCard({
  profile,
  isCurrent,
  onChanged,
}: {
  profile: Profile
  isCurrent: boolean
  onChanged: () => void
}) {
  const [editing, setEditing] = useState(isCurrent)
  const avatar = profileAvatarFor(profile.avatar_key)

  return (
    <div className="overflow-hidden rounded-mga border border-mga-border bg-mga-bg shadow-md">
      <div className={cn('h-1 bg-gradient-to-r', avatar.tone)} />
      <div className="p-4">
        <div className="flex items-start justify-between gap-3">
          <div className="flex min-w-0 items-center gap-3">
            <ProfileAvatar profile={profile} className="h-12 w-12" />
            <div className="min-w-0">
              <div className="flex items-center gap-2">
                <h3 className="truncate text-lg font-bold text-mga-text">{profile.display_name}</h3>
                {isCurrent ? (
                  <span className="rounded-full bg-mga-accent/15 px-2 py-0.5 text-[0.68rem] font-bold uppercase tracking-[0.14em] text-mga-accent">
                    Current
                  </span>
                ) : null}
              </div>
              <div className="mt-1 flex items-center gap-1.5 text-sm text-mga-muted">
                {profile.role === 'admin_player' ? <ShieldCheck className="h-4 w-4" /> : <UserRound className="h-4 w-4" />}
                {roleLabel(profile.role)}
              </div>
            </div>
          </div>
          <Button variant="outline" size="sm" onClick={() => setEditing((value) => !value)}>
            {editing ? 'Close' : 'Edit'}
          </Button>
        </div>
        {editing ? (
          <div className="mt-4 border-t border-mga-border pt-4">
            <ProfileEditor profile={profile} isCurrent={isCurrent} onChanged={onChanged} />
          </div>
        ) : null}
      </div>
    </div>
  )
}

function ProfileEditor({
  profile,
  isCurrent = true,
  compact = false,
  onChanged,
}: {
  profile: Profile
  isCurrent?: boolean
  compact?: boolean
  onChanged: () => void
}) {
  const [draft, setDraft] = useState<ProfileDraft>(() => draftFromProfile(profile))
  const [error, setError] = useState('')

  useEffect(() => {
    setDraft(draftFromProfile(profile))
    setError('')
  }, [profile])

  const dirty =
    draft.display_name !== profile.display_name ||
    draft.avatar_key !== (profile.avatar_key || 'player-1') ||
    draft.role !== profile.role

  const save = useMutation({
    mutationFn: () => updateProfile(profile.id, draft),
    onSuccess: onChanged,
    onError: (err) => setError(errorText(err, 'Save failed')),
  })
  const remove = useMutation({
    mutationFn: () => deleteProfile(profile.id),
    onSuccess: onChanged,
    onError: (err) => setError(errorText(err, 'Delete failed')),
  })

  return (
    <div className={cn(compact ? 'p-5' : 'space-y-4')}>
      <div className={cn('grid gap-4', compact ? 'lg:grid-cols-[1fr_12rem]' : '')}>
        <div className="space-y-4">
          <Input
            label="Name"
            value={draft.display_name}
            onChange={(event) => setDraft({ ...draft, display_name: event.target.value })}
          />
          <AvatarChooser
            value={draft.avatar_key}
            onChange={(avatar_key) => setDraft({ ...draft, avatar_key })}
          />
        </div>
        <div className="space-y-4">
          <Select
            label="Role"
            value={draft.role}
            onChange={(event) => setDraft({ ...draft, role: event.target.value as ProfileRole })}
            options={roles}
          />
          <div className="flex flex-wrap gap-2 pt-1 lg:pt-6">
            <Button
              onClick={() => {
                setError('')
                save.mutate()
              }}
              disabled={!dirty || draft.display_name.trim() === '' || save.isPending}
            >
              {save.isSuccess && !dirty ? <Check className="h-4 w-4" /> : <Save className="h-4 w-4" />}
              Save
            </Button>
            <Button
              variant="outline"
              onClick={() => {
                setError('')
                remove.mutate()
              }}
              disabled={isCurrent || remove.isPending}
              title={isCurrent ? 'Switch profiles before deleting this one' : 'Delete profile'}
            >
              <Trash2 className="h-4 w-4" />
              Delete
            </Button>
          </div>
        </div>
      </div>
      {error ? <p className="mt-3 text-sm text-red-400">{error}</p> : null}
    </div>
  )
}

function ProfileBackdrop({ profile }: { profile: Profile }) {
  const avatar = profileAvatarFor(profile.avatar_key)
  const Icon = avatar.Icon
  return (
    <div className="absolute inset-0 overflow-hidden">
      <div className={cn('absolute -right-8 -top-10 grid h-32 w-32 rotate-12 place-items-center rounded-mga bg-gradient-to-br opacity-20', avatar.tone)}>
        <Icon className="h-16 w-16" />
      </div>
      <div className="absolute inset-0 bg-[linear-gradient(135deg,rgba(255,255,255,0.06)_0,transparent_42%)]" />
    </div>
  )
}

function draftFromProfile(profile: Profile): ProfileDraft {
  return {
    display_name: profile.display_name,
    avatar_key: (profile.avatar_key || 'player-1') as ProfileAvatarKey,
    role: profile.role,
  }
}

function roleLabel(role: ProfileRole): string {
  return role === 'admin_player' ? 'Admin Player' : 'Player'
}

function errorText(err: unknown, fallback: string): string {
  if (err instanceof Error) return err.message
  return fallback
}
