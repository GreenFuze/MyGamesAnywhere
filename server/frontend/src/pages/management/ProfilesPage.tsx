import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { KeyRound, Pencil, Plus, ShieldCheck, Trash2, UserRound } from 'lucide-react'
import {
  createCredentialTicket,
  createProfile,
  deleteProfile,
  getActiveCredentialTicket,
  listIntegrations,
  revokeCredentialTicket,
  updateProfile,
  type IssuedCredentialTicket,
  type Profile,
  type ProfileRole,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import {
  ActionError,
  ConfirmDialog,
  FormDialog,
  RestrictedNotice,
  ShowOnceSecret,
} from '@/components/management/ManagementActions'
import { PageIntro, SectionCard, StatusPill, formatDate } from '@/components/management/ManagementPrimitives'
import { profileAvatarFor, useProfiles } from '@/hooks/useProfiles'
import { DESTRUCTIVE_ACTIONS, ManagementPolicy } from '@/lib/managementPolicy'

const ROLE_OPTIONS = [
  { value: 'player', label: 'Player' },
  { value: 'admin_player', label: 'Administrator' },
]

export function ProfilesPage() {
  const { profiles, currentProfile, selectProfile, refreshProfiles } = useProfiles()
  const queryClient = useQueryClient()
  const policy = new ManagementPolicy(currentProfile)
  const isAdmin = policy.can('profile.create')

  const [editing, setEditing] = useState<Profile | null>(null)
  const [creating, setCreating] = useState(false)
  const [deleting, setDeleting] = useState<Profile | null>(null)
  const [issuedTicket, setIssuedTicket] = useState<IssuedCredentialTicket | null>(null)

  // Connection counts are aggregate health for the selected profile only; an
  // administrator never sees another profile's connections or their secrets.
  const integrations = useQuery({
    queryKey: ['management', 'integrations'],
    queryFn: listIntegrations,
  })

  const afterProfileChange = async () => {
    refreshProfiles()
    await queryClient.invalidateQueries({ queryKey: ['profiles'] })
  }

  const create = useMutation({
    mutationFn: (body: { display_name: string; role: ProfileRole }) => createProfile(body),
    onSuccess: async () => {
      setCreating(false)
      await afterProfileChange()
    },
  })

  const update = useMutation({
    mutationFn: ({ id, body }: { id: string; body: { display_name: string; role: ProfileRole } }) =>
      updateProfile(id, body),
    onSuccess: async () => {
      setEditing(null)
      await afterProfileChange()
    },
  })

  const remove = useMutation({
    mutationFn: (id: string) => deleteProfile(id),
    onSuccess: async () => {
      setDeleting(null)
      await afterProfileChange()
    },
  })

  const issueTicket = useMutation({
    mutationFn: (profileId: string) => createCredentialTicket(profileId),
    onSuccess: (ticket) => setIssuedTicket(ticket),
  })

  return (
    <div className="mga-page-enter space-y-7">
      <PageIntro
        eyebrow="Access"
        title="Profiles"
        description="Each profile has its own games, sources and achievements. Nothing is shared between them."
        actions={isAdmin ? (
          <Button onClick={() => { create.reset(); setCreating(true) }}>
            <Plus className="h-4 w-4" /> Add profile
          </Button>
        ) : undefined}
      />

      {!isAdmin && (
        <RestrictedNotice>
          Creating, editing, and removing profiles requires an administrator profile. You can still
          switch context and manage your own credentials.
        </RestrictedNotice>
      )}

      <SectionCard
        title="Available profiles"
        description="Names and roles only. One profile can never see another's accounts or games."
      >
        <div className="grid gap-3 lg:grid-cols-2">
          {profiles.map((profile) => (
            <ProfileCard
              key={profile.id}
              profile={profile}
              selected={profile.id === currentProfile?.id}
              connectionCount={profile.id === currentProfile?.id ? integrations.data?.length : undefined}
              canManage={isAdmin}
              onSelect={() => void selectProfile(profile.id)}
              onEdit={() => { update.reset(); setEditing(profile) }}
              onDelete={() => { remove.reset(); setDeleting(profile) }}
              onIssueTicket={() => { issueTicket.reset(); setIssuedTicket(null); issueTicket.mutate(profile.id) }}
              issuing={issueTicket.isPending && issueTicket.variables === profile.id}
            />
          ))}
        </div>
        <ActionError error={issueTicket.error} className="mt-4" />
      </SectionCard>

      {issuedTicket && (
        <SectionCard title="Password setup link" description="Send this to whoever owns the profile, somewhere private. It works once.">
          <ShowOnceSecret
            label="One-time setup link"
            value={issuedTicket.setup_url || issuedTicket.token}
            warning={`Expires ${formatDate(issuedTicket.ticket.expires_at)}. Anyone holding this link can set that profile's credential until it expires or is revoked.`}
          />
          <div className="mt-3 flex flex-wrap gap-2">
            <Button variant="outline" size="sm" onClick={() => setIssuedTicket(null)}>Done</Button>
            <RevokeTicketButton profileId={issuedTicket.ticket.profile_id} onRevoked={() => setIssuedTicket(null)} />
          </div>
        </SectionCard>
      )}

      <ProfileFormDialog
        open={creating}
        title="Add profile"
        description="A new profile starts empty, with no sources and no games."
        submitLabel="Create profile"
        submitting={create.isPending}
        error={create.error}
        onClose={() => setCreating(false)}
        onSubmit={(body) => create.mutate(body)}
      />

      <ProfileFormDialog
        open={editing !== null}
        title="Edit profile"
        submitLabel="Save changes"
        initial={editing ?? undefined}
        submitting={update.isPending}
        error={update.error}
        onClose={() => setEditing(null)}
        onSubmit={(body) => editing && update.mutate({ id: editing.id, body })}
      />

      <ConfirmDialog
        open={deleting !== null}
        title={`Delete ${deleting?.display_name ?? 'profile'}?`}
        confirmLabel="Delete profile"
        submitting={remove.isPending}
        error={remove.error}
        onClose={() => setDeleting(null)}
        onConfirm={() => deleting && remove.mutate(deleting.id)}
        consequences={DESTRUCTIVE_ACTIONS['profile.delete'].consequences}
        preserves={DESTRUCTIVE_ACTIONS['profile.delete'].preserves}
      />
    </div>
  )
}

function ProfileCard({
  profile, selected, connectionCount, canManage, onSelect, onEdit, onDelete, onIssueTicket, issuing,
}: {
  profile: Profile
  selected: boolean
  connectionCount?: number
  canManage: boolean
  onSelect: () => void
  onEdit: () => void
  onDelete: () => void
  onIssueTicket: () => void
  issuing: boolean
}) {
  const Avatar = profileAvatarFor(profile.avatar_key).Icon
  return (
    <article className="rounded-xl border border-mga-border bg-mga-elevated/50 p-4">
      <div className="flex items-start gap-4">
        <span className="grid h-12 w-12 shrink-0 place-items-center rounded-xl bg-gradient-to-br from-mga-accent/25 to-violet-400/15 text-mga-accent">
          <Avatar className="h-5 w-5" />
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="truncate text-sm font-semibold text-mga-text">{profile.display_name}</span>
            {selected && <StatusPill label="Selected" tone="good" />}
          </div>
          <p className="mt-1 text-xs text-mga-muted">
            {profile.role === 'admin_player' ? 'Administrator' : 'Profile'}
            {selected && connectionCount !== undefined && ` · ${connectionCount} connection${connectionCount === 1 ? '' : 's'}`}
            {' · updated '}{formatDate(profile.updated_at)}
          </p>
        </div>
        {profile.role === 'admin_player'
          ? <ShieldCheck className="h-5 w-5 shrink-0 text-emerald-300" />
          : <UserRound className="h-5 w-5 shrink-0 text-mga-muted" />}
      </div>
      <div className="mt-4 flex flex-wrap gap-2">
        <Button variant="outline" size="sm" onClick={onSelect} disabled={selected}>
          {selected ? 'Current context' : 'Switch to profile'}
        </Button>
        {canManage && (
          <>
            <Button variant="ghost" size="sm" onClick={onEdit}><Pencil className="h-3.5 w-3.5" /> Edit</Button>
            <Button variant="ghost" size="sm" onClick={onIssueTicket} disabled={issuing}>
              <KeyRound className="h-3.5 w-3.5" /> {issuing ? 'Issuing…' : 'Credential link'}
            </Button>
            <Button variant="ghost" size="sm" onClick={onDelete} className="text-rose-300 hover:bg-rose-500/10">
              <Trash2 className="h-3.5 w-3.5" /> Delete
            </Button>
          </>
        )}
      </div>
    </article>
  )
}

function ProfileFormDialog({
  open, title, description, submitLabel, initial, submitting, error, onClose, onSubmit,
}: {
  open: boolean
  title: string
  description?: string
  submitLabel: string
  initial?: Profile
  submitting: boolean
  error: unknown
  onClose: () => void
  onSubmit: (body: { display_name: string; role: ProfileRole }) => void
}) {
  const [name, setName] = useState('')
  const [role, setRole] = useState<ProfileRole>('player')
  const [seeded, setSeeded] = useState<string | null>(null)

  // Seed the form once per opened target rather than on every render.
  const seedKey = open ? initial?.id ?? 'new' : null
  if (seedKey !== seeded) {
    setSeeded(seedKey)
    setName(initial?.display_name ?? '')
    setRole(initial?.role ?? 'player')
  }

  const trimmed = name.trim()
  return (
    <FormDialog
      open={open}
      onClose={onClose}
      title={title}
      description={description}
      submitLabel={submitLabel}
      submitting={submitting}
      error={error}
      disabled={trimmed === ''}
      onSubmit={() => onSubmit({ display_name: trimmed, role })}
    >
      <Input label="Display name" value={name} onChange={(event) => setName(event.target.value)} autoFocus />
      <Select
        label="Role"
        value={role}
        options={ROLE_OPTIONS}
        onChange={(event) => setRole(event.target.value as ProfileRole)}
      />
      <p className="text-xs leading-5 text-mga-muted">
        Administrators can manage server configuration, sources, and API clients. Players manage only
        their own credentials, connections, and tokens.
      </p>
    </FormDialog>
  )
}

function RevokeTicketButton({ profileId, onRevoked }: { profileId: string; onRevoked: () => void }) {
  const active = useQuery({
    queryKey: ['management', 'credential-ticket', profileId],
    queryFn: () => getActiveCredentialTicket(profileId),
  })
  const revoke = useMutation({
    mutationFn: (ticketId: string) => revokeCredentialTicket(profileId, ticketId),
    onSuccess: async () => {
      await active.refetch()
      onRevoked()
    },
  })
  const ticket = active.data?.ticket
  if (!ticket) return null
  return (
    <Button
      variant="ghost"
      size="sm"
      className="text-rose-300 hover:bg-rose-500/10"
      disabled={revoke.isPending}
      onClick={() => revoke.mutate(ticket.id)}
    >
      {revoke.isPending ? 'Revoking…' : 'Revoke this link'}
    </Button>
  )
}
