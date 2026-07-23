import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { KeyRound } from 'lucide-react'
import { Link, useSearchParams } from 'react-router-dom'
import { listProfiles, redeemCredentialTicket, type CredentialKind } from '@/api/client'
import { credentialPolicy } from '@/components/auth/credentialPolicy'
import { Button, buttonVariants } from '@/components/ui/button'
import { SecretInput } from '@/components/ui/secret-input'
import { Select } from '@/components/ui/select'

function ticketFromFragment(): string {
  return new URLSearchParams(window.location.hash.replace(/^#/, '')).get('ticket')?.trim() ?? ''
}

export function CredentialSetupPage() {
  const [search] = useSearchParams()
  const profileId = search.get('profile_id')?.trim() ?? ''
  const ticket = useMemo(ticketFromFragment, [])
  const profiles = useQuery({ queryKey: ['credential-setup-profiles'], queryFn: listProfiles })
  const profile = profiles.data?.find((candidate) => candidate.id === profileId)
  const [kind, setKind] = useState<CredentialKind>('password')
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [saving, setSaving] = useState(false)
  const [complete, setComplete] = useState(false)
  const [error, setError] = useState('')
  const valid = Boolean(profileId && ticket && next === confirm && credentialPolicy.isValid(kind, next))

  async function submit() {
    setSaving(true)
    setError('')
    try {
      await redeemCredentialTicket(profileId, ticket, next, kind)
      window.history.replaceState(null, '', `/credential-setup?profile_id=${encodeURIComponent(profileId)}`)
      setComplete(true)
      setNext('')
      setConfirm('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'This setup link could not be used.')
    } finally {
      setSaving(false)
    }
  }

  return (
    <main className="grid min-h-screen place-items-center bg-mga-bg p-4 text-mga-text">
      <section className="w-full max-w-lg rounded-mga border border-mga-border bg-mga-surface p-6 shadow-2xl">
        <div className="grid h-12 w-12 place-items-center rounded-full bg-mga-accent text-mga-bg">
          <KeyRound className="h-6 w-6" />
        </div>
        <h1 className="mt-4 text-2xl font-black">{complete ? 'Sign-in protection is ready' : 'Choose your password or PIN'}</h1>
        <p className="mt-2 text-sm leading-6 text-mga-muted">
          {profile ? `This private setup link is for ${profile.display_name}.` : 'MGA is checking this private setup link.'}
        </p>
        {complete ? (
          <div className="mt-5 space-y-4">
            <p className="rounded-mga border border-emerald-400/30 bg-emerald-400/10 p-4 text-sm text-emerald-100">
              Your new sign-in is saved. Other sessions for this profile were signed out.
            </p>
            <Link to="/" className={buttonVariants()}>Continue to MGA</Link>
          </div>
        ) : (
          <div className="mt-5 space-y-4">
            <Select
              label="Sign-in type"
              value={kind}
              onChange={(event) => setKind(event.target.value as CredentialKind)}
              options={[{ value: 'password', label: 'Password' }, { value: 'pin', label: 'PIN' }]}
            />
            <SecretInput label={credentialPolicy.label(kind)} value={next} onChange={(event) => setNext(event.target.value)} />
            <SecretInput label="Confirm" value={confirm} onChange={(event) => setConfirm(event.target.value)} />
            {next !== confirm && confirm ? <p className="text-sm text-red-400">The entries do not match.</p> : null}
            {!ticket || !profileId ? <p className="text-sm text-red-400">This setup link is incomplete. Ask an administrator for a new link.</p> : null}
            {error ? <p className="text-sm text-red-400">{error}</p> : null}
            <Button onClick={submit} disabled={!valid || saving || profiles.isLoading} className="w-full">
              Save Sign-in
            </Button>
          </div>
        )}
      </section>
    </main>
  )
}
