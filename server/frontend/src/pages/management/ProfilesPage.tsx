import { ShieldCheck, UserRound } from 'lucide-react'
import { PageIntro, SectionCard, StatusPill, formatDate } from '@/components/management/ManagementPrimitives'
import { profileAvatarFor, useProfiles } from '@/hooks/useProfiles'

export function ProfilesPage() {
  const { profiles, currentProfile, selectProfile } = useProfiles()
  return (
    <div className="mga-page-enter space-y-7">
      <PageIntro eyebrow="Access" title="Profiles and policy" description="Profile context controls every private library, source, achievement, and integration request. Switching context clears profile-owned cached data first." />
      <SectionCard title="Available profiles" description="Only identity and role are shown here. Provider credentials and private source data never cross profile boundaries.">
        <div className="grid gap-3 lg:grid-cols-2">
          {profiles.map((profile) => {
            const Avatar = profileAvatarFor(profile.avatar_key).Icon
            const selected = profile.id === currentProfile?.id
            return (
              <button key={profile.id} type="button" disabled={selected} onClick={() => void selectProfile(profile.id)} className="flex min-h-24 items-center gap-4 rounded-xl border border-mga-border bg-mga-elevated/50 p-4 text-left transition hover:border-mga-accent/35 disabled:cursor-default disabled:border-mga-accent/30 disabled:bg-mga-accent/5 focus:outline-none focus:ring-2 focus:ring-mga-accent/40">
                <span className="grid h-12 w-12 shrink-0 place-items-center rounded-xl bg-gradient-to-br from-mga-accent/25 to-violet-400/15 text-mga-accent"><Avatar className="h-5 w-5" /></span>
                <span className="min-w-0 flex-1">
                  <span className="flex flex-wrap items-center gap-2"><span className="truncate text-sm font-semibold text-mga-text">{profile.display_name}</span>{selected && <StatusPill label="Selected" tone="good" />}</span>
                  <span className="mt-1 block text-xs text-mga-muted">{profile.role === 'admin_player' ? 'Administrator' : 'Profile'} · updated {formatDate(profile.updated_at)}</span>
                </span>
                {profile.role === 'admin_player' ? <ShieldCheck className="h-5 w-5 text-emerald-300" /> : <UserRound className="h-5 w-5 text-mga-muted" />}
              </button>
            )
          })}
        </div>
      </SectionCard>
    </div>
  )
}
