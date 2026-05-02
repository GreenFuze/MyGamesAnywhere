import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { createProfile, deleteProfile, updateProfile, type Profile, type ProfileRole } from '@/api/client'
import { ProfileAvatar, useProfiles } from '@/hooks/useProfiles'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'

const roles: Array<{ id: ProfileRole; label: string }> = [
  { id: 'admin_player', label: 'Admin Player' },
  { id: 'player', label: 'Player' },
]

export function ProfilesTab() {
  const queryClient = useQueryClient()
  const { profiles, currentProfile, refreshProfiles } = useProfiles()
  const [newName, setNewName] = useState('')
  const [newRole, setNewRole] = useState<ProfileRole>('player')
  const [error, setError] = useState('')

  const refresh = async () => {
    refreshProfiles()
    await queryClient.invalidateQueries()
  }

  const createMutation = useMutation({
    mutationFn: createProfile,
    onSuccess: async () => {
      setNewName('')
      setNewRole('player')
      await refresh()
    },
    onError: (err) => setError(err instanceof Error ? err.message : 'Create failed'),
  })

  return (
    <div className="space-y-4">
      <div className="rounded-mga border border-mga-border bg-mga-surface p-4">
        <h2 className="text-base font-semibold text-mga-text">Profiles</h2>
        <div className="mt-4 space-y-3">
          {profiles.map((profile) => (
            <ProfileRow key={profile.id} profile={profile} isCurrent={profile.id === currentProfile?.id} onChanged={refresh} />
          ))}
        </div>
      </div>

      <div className="rounded-mga border border-mga-border bg-mga-surface p-4">
        <h2 className="text-base font-semibold text-mga-text">Add Profile</h2>
        <div className="mt-4 grid gap-3 md:grid-cols-[1fr_auto_auto]">
          <Input value={newName} onChange={(event) => setNewName(event.target.value)} placeholder="Profile name" />
          <select
            value={newRole}
            onChange={(event) => setNewRole(event.target.value as ProfileRole)}
            className="h-9 rounded-mga border border-mga-border bg-mga-bg px-3 text-sm text-mga-text"
          >
            {roles.map((role) => (
              <option key={role.id} value={role.id}>{role.label}</option>
            ))}
          </select>
          <Button
            onClick={() => {
              setError('')
              createMutation.mutate({ display_name: newName, role: newRole, avatar_key: 'player-1' })
            }}
            disabled={newName.trim() === '' || createMutation.isPending}
          >
            Add
          </Button>
        </div>
        {error ? <p className="mt-2 text-sm text-red-400">{error}</p> : null}
      </div>
    </div>
  )
}

function ProfileRow({ profile, isCurrent, onChanged }: { profile: Profile; isCurrent: boolean; onChanged: () => void }) {
  const [name, setName] = useState(profile.display_name)
  const [role, setRole] = useState<ProfileRole>(profile.role)
  const [error, setError] = useState('')

  const save = useMutation({
    mutationFn: () => updateProfile(profile.id, { display_name: name, role, avatar_key: profile.avatar_key || 'player-1' }),
    onSuccess: onChanged,
    onError: (err) => setError(err instanceof Error ? err.message : 'Save failed'),
  })
  const remove = useMutation({
    mutationFn: () => deleteProfile(profile.id),
    onSuccess: onChanged,
    onError: (err) => setError(err instanceof Error ? err.message : 'Delete failed'),
  })

  return (
    <div className="grid gap-3 rounded-mga border border-mga-border bg-mga-bg p-3 md:grid-cols-[auto_1fr_auto_auto_auto] md:items-center">
      <ProfileAvatar profile={profile} />
      <Input value={name} onChange={(event) => setName(event.target.value)} aria-label={`${profile.display_name} name`} />
      <select
        value={role}
        onChange={(event) => setRole(event.target.value as ProfileRole)}
        className="h-9 rounded-mga border border-mga-border bg-mga-surface px-3 text-sm text-mga-text"
      >
        {roles.map((item) => (
          <option key={item.id} value={item.id}>{item.label}</option>
        ))}
      </select>
      <Button variant="outline" onClick={() => save.mutate()} disabled={save.isPending || name.trim() === ''}>Save</Button>
      <Button variant="outline" onClick={() => remove.mutate()} disabled={remove.isPending || isCurrent}>Delete</Button>
      {error ? <p className="text-sm text-red-400 md:col-span-5">{error}</p> : null}
    </div>
  )
}
