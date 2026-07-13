import { useState, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, KeyRound, Save, ShieldAlert } from 'lucide-react'
import {
  changeOwnCredential,
  getAuthSession,
  getCredentialStatus,
  loginProfile,
  type CredentialKind,
  type Profile,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'

type ProfileSignInGateProps = {
  profile: Profile
  onCancel: () => Promise<void>
  children: ReactNode
}

export function ProfileSignInGate({ profile, onCancel, children }: ProfileSignInGateProps) {
  const queryClient = useQueryClient()
  const [credential, setCredential] = useState('')
  const [verifiedCredential, setVerifiedCredential] = useState('')
  const credentialQuery = useQuery({
    queryKey: ['credential-status', profile.id],
    queryFn: getCredentialStatus,
    retry: false,
  })
  const sessionQuery = useQuery({ queryKey: ['auth-session'], queryFn: getAuthSession, retry: false })
  const login = useMutation({
    mutationFn: () => loginProfile(profile.id, credential),
    onSuccess: async (session) => {
      setVerifiedCredential(session.must_change ? credential : '')
      await queryClient.invalidateQueries({ queryKey: ['auth-session'] })
    },
  })

  if (credentialQuery.isLoading || sessionQuery.isLoading) {
    return <SignInShell profile={profile} title="Checking sign-in…" />
  }
  if (credentialQuery.error) {
    return <SignInShell profile={profile} title="Sign-in unavailable" error={errorText(credentialQuery.error)} onCancel={onCancel} />
  }
  if (!credentialQuery.data?.configured) {
    return children
  }

  const signedIn = sessionQuery.data?.authenticated && sessionQuery.data.profile?.id === profile.id
  if (signedIn && sessionQuery.data?.must_change) {
    return <BootstrapCredentialChange profile={profile} currentCredential={verifiedCredential} onCancel={onCancel} />
  }
  if (signedIn) {
    return children
  }

  const roleLabel = profile.role === 'admin_player' ? 'Administrator' : 'Player'
  return (
    <SignInShell profile={profile} title={`Sign in to ${profile.display_name}`} onCancel={onCancel}>
      <p className="text-sm leading-6 text-mga-muted">
        This {roleLabel.toLowerCase()} profile is protected with a password or PIN.
      </p>
      <div className="mt-5 space-y-3">
        <Input
          label="Password or PIN"
          type="password"
          autoFocus
          value={credential}
          onChange={(event) => setCredential(event.target.value)}
          onKeyDown={(event) => event.key === 'Enter' && credential && login.mutate()}
        />
        <Button onClick={() => login.mutate()} disabled={!credential || login.isPending} className="w-full">
          <KeyRound className="h-4 w-4" /> Sign In
        </Button>
        {credentialQuery.data.must_change && profile.role === 'admin_player' ? (
          <p className="text-xs leading-5 text-mga-muted">
            The first administrator starts with <span className="font-mono text-mga-text">changeme</span> and must replace it at first sign-in.
          </p>
        ) : null}
        {login.error ? <p className="text-sm text-red-400">{errorText(login.error)}</p> : null}
      </div>
    </SignInShell>
  )
}

function BootstrapCredentialChange({
  profile,
  currentCredential,
  onCancel,
}: {
  profile: Profile
  currentCredential: string
  onCancel: () => Promise<void>
}) {
  const queryClient = useQueryClient()
  const [current, setCurrent] = useState(currentCredential)
  const [next, setNext] = useState('')
  const [confirm, setConfirm] = useState('')
  const [kind, setKind] = useState<CredentialKind>('password')
  const change = useMutation({
    mutationFn: () => changeOwnCredential(current, next, kind),
    onSuccess: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['auth-session'] }),
        queryClient.invalidateQueries({ queryKey: ['credential-status', profile.id] }),
      ])
    },
  })
  const valid = Boolean(current) && next === confirm && (kind === 'password' ? next.length >= 8 : /^\d{6,12}$/.test(next))

  return (
    <SignInShell profile={profile} title="Replace the bootstrap password" onCancel={onCancel}>
      <div className="flex items-start gap-3 rounded-mga border border-amber-500/30 bg-amber-500/10 p-3 text-sm leading-6 text-amber-100">
        <ShieldAlert className="mt-0.5 h-5 w-5 shrink-0" />
        <p>The public default must be replaced before this administrator profile can enter MGA.</p>
      </div>
      <div className="mt-5 space-y-3">
        {!currentCredential ? (
          <Input label="Current password" type="password" autoFocus value={current} onChange={(event) => setCurrent(event.target.value)} />
        ) : null}
        <Select
          label="New credential type"
          value={kind}
          onChange={(event) => setKind(event.target.value as CredentialKind)}
          options={[{ value: 'password', label: 'Password' }, { value: 'pin', label: 'PIN' }]}
        />
        <Input
          label={kind === 'pin' ? 'New PIN (6–12 digits)' : 'New password (8+ characters)'}
          type="password"
          autoFocus={Boolean(currentCredential)}
          value={next}
          onChange={(event) => setNext(event.target.value)}
        />
        <Input label="Confirm new credential" type="password" value={confirm} onChange={(event) => setConfirm(event.target.value)} />
        <Button onClick={() => change.mutate()} disabled={!valid || change.isPending} className="w-full">
          <Save className="h-4 w-4" /> Save And Sign In
        </Button>
        {confirm && next !== confirm ? <p className="text-sm text-red-400">Credentials do not match.</p> : null}
        {change.error ? <p className="text-sm text-red-400">{errorText(change.error)}</p> : null}
      </div>
    </SignInShell>
  )
}

function SignInShell({
  profile,
  title,
  error,
  onCancel,
  children,
}: {
  profile: Profile
  title: string
  error?: string
  onCancel?: () => Promise<void>
  children?: ReactNode
}) {
  return (
    <div className="relative flex min-h-screen items-center justify-center overflow-hidden bg-mga-bg p-4 font-mga text-mga-text">
      <div className="absolute inset-0 bg-[linear-gradient(90deg,rgba(255,255,255,0.035)_1px,transparent_1px),linear-gradient(0deg,rgba(255,255,255,0.028)_1px,transparent_1px)] bg-[size:44px_44px]" />
      <section className="relative w-full max-w-lg rounded-mga border border-mga-border bg-mga-surface p-6 shadow-2xl">
        <div className="flex items-start justify-between gap-4">
          <div>
            <div className="text-xs font-bold uppercase tracking-[0.22em] text-mga-accent">
              {profile.role === 'admin_player' ? 'Administrator sign-in' : 'Player sign-in'}
            </div>
            <h1 className="mt-2 text-2xl font-black text-mga-text">{title}</h1>
          </div>
          {onCancel ? (
            <Button variant="outline" size="sm" onClick={() => void onCancel()}>
              <ArrowLeft className="h-4 w-4" /> Profiles
            </Button>
          ) : null}
        </div>
        {error ? <p className="mt-4 text-sm text-red-400">{error}</p> : null}
        {children ? <div className="mt-4">{children}</div> : null}
      </section>
    </div>
  )
}

function errorText(error: unknown): string {
  if (error && typeof error === 'object' && 'responseText' in error) {
    const responseText = String((error as { responseText?: string }).responseText ?? '').trim()
    if (responseText) return responseText
  }
  return error instanceof Error ? error.message : 'Sign-in failed'
}
