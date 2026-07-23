import { useEffect, useState, type ReactNode } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Check, Clipboard, KeyRound, LifeBuoy, Save, ShieldAlert } from 'lucide-react'
import {
  changeOwnCredential,
  getAuthSession,
  getCredentialStatus,
  loginProfile,
  type CredentialKind,
  type Profile,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { SecretInput } from '@/components/ui/secret-input'
import { Select } from '@/components/ui/select'
import { credentialPolicy } from '@/components/auth/credentialPolicy'

type ProfileSignInGateProps = {
  profile: Profile
  onCancel: () => Promise<void>
  children: ReactNode
}

export function ProfileSignInGate({ profile, onCancel, children }: ProfileSignInGateProps) {
  const queryClient = useQueryClient()
  const [credential, setCredential] = useState('')
  const [verifiedCredential, setVerifiedCredential] = useState('')
  const [recoveryOpen, setRecoveryOpen] = useState(false)
  const [continueWithoutCredential, setContinueWithoutCredential] = useState(false)
  useEffect(() => setContinueWithoutCredential(false), [profile.id])
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
    if (continueWithoutCredential) return children
    return (
      <InitialCredentialSetup
        profile={profile}
        onCancel={onCancel}
        onContinue={() => setContinueWithoutCredential(true)}
      />
    )
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
        <SecretInput
          label="Password or PIN"
          autoFocus
          value={credential}
          onChange={(event) => setCredential(event.target.value)}
          onKeyDown={(event) => event.key === 'Enter' && credential && login.mutate()}
        />
        <Button onClick={() => login.mutate()} disabled={!credential || login.isPending} className="w-full">
          <KeyRound className="h-4 w-4" /> Sign In
        </Button>
        <Button variant="outline" onClick={() => setRecoveryOpen((current) => !current)} className="w-full">
          <LifeBuoy className="h-4 w-4" /> Forgot password or PIN?
        </Button>
        {recoveryOpen ? <RecoveryInstructions profile={profile} /> : null}
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

function InitialCredentialSetup({
  profile,
  onCancel,
  onContinue,
}: {
  profile: Profile
  onCancel: () => Promise<void>
  onContinue: () => void
}) {
  return (
    <SignInShell profile={profile} title={`Welcome, ${profile.display_name}`} onCancel={onCancel}>
      <p className="text-sm leading-6 text-mga-muted">
        This profile is intentionally passwordless. You can keep using it this way on your trusted MGA network.
      </p>
      <div className="mt-5 space-y-3">
        <div className="rounded-mga border border-mga-border bg-mga-bg/70 p-4 text-sm leading-6 text-mga-muted">
          To add a password or PIN, ask an administrator to create a private setup link in Settings → Profiles. Open that link on any MGA computer and choose your own secret.
        </div>
        <Button onClick={onContinue} className="w-full">
          Continue Without A Password
        </Button>
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
  const valid = Boolean(current) && next === confirm && credentialPolicy.isValid(kind, next)

  return (
    <SignInShell profile={profile} title="Replace the bootstrap password" onCancel={onCancel}>
      <div className="flex items-start gap-3 rounded-mga border border-amber-500/30 bg-amber-500/10 p-3 text-sm leading-6 text-amber-100">
        <ShieldAlert className="mt-0.5 h-5 w-5 shrink-0" />
        <p>The public recovery password must be replaced before this profile can enter MGA.</p>
      </div>
      <div className="mt-5 space-y-3">
        {!currentCredential ? (
          <SecretInput label="Current password" autoFocus value={current} onChange={(event) => setCurrent(event.target.value)} />
        ) : null}
        <Select
          label="New credential type"
          value={kind}
          onChange={(event) => setKind(event.target.value as CredentialKind)}
          options={[{ value: 'password', label: 'Password' }, { value: 'pin', label: 'PIN' }]}
        />
        <SecretInput
          label={credentialPolicy.label(kind)}
          autoFocus={Boolean(currentCredential)}
          value={next}
          onChange={(event) => setNext(event.target.value)}
        />
        <SecretInput label="Confirm new credential" value={confirm} onChange={(event) => setConfirm(event.target.value)} />
        <Button onClick={() => change.mutate()} disabled={!valid || change.isPending} className="w-full">
          <Save className="h-4 w-4" /> Save And Sign In
        </Button>
        {confirm && next !== confirm ? <p className="text-sm text-red-400">Credentials do not match.</p> : null}
        {change.error ? <p className="text-sm text-red-400">{errorText(change.error)}</p> : null}
      </div>
    </SignInShell>
  )
}

function RecoveryInstructions({ profile }: { profile: Profile }) {
  const [copied, setCopied] = useState('')
  const commands = [
    { mode: 'Portable', command: `.\\mga_server.exe --reset-profile-credential "${profile.id}"` },
    { mode: 'Per-user install', command: `.\\mga_server.exe --runtime-mode user --reset-profile-credential "${profile.id}"` },
    { mode: 'All-users service', command: `.\\mga_server.exe --runtime-mode machine --reset-profile-credential "${profile.id}"` },
  ]

  const copy = async (command: string) => {
    await navigator.clipboard.writeText(command)
    setCopied(command)
  }

  return (
    <div className="space-y-3 rounded-mga border border-mga-border bg-mga-bg/70 p-4">
      <p className="text-sm leading-6 text-mga-muted">
        Ask an MGA administrator to open Settings → Profiles and create a private setup link for {profile.display_name}. Open it on this computer and choose a new password or PIN. The administrator never sees your new secret.
      </p>
      <p className="text-xs leading-5 text-mga-muted">If no administrator can sign in, use the server-local break-glass command matching the installation:</p>
      {commands.map(({ mode, command }) => (
        <div key={mode} className="space-y-1">
          <div className="text-xs font-bold uppercase tracking-[0.14em] text-mga-muted">{mode}</div>
          <div className="flex items-center gap-2 rounded-mga border border-mga-border bg-mga-surface p-2">
            <code className="min-w-0 flex-1 overflow-x-auto whitespace-nowrap text-xs text-mga-text">{command}</code>
            <button
              type="button"
              onClick={() => void copy(command)}
              className="grid h-8 w-8 shrink-0 place-items-center rounded-mga text-mga-muted hover:bg-mga-elevated hover:text-mga-text focus:outline-none focus-visible:ring-2 focus-visible:ring-mga-accent"
              aria-label={`Copy ${mode} recovery command`}
              title={`Copy ${mode} recovery command`}
            >
              {copied === command ? <Check className="h-4 w-4 text-emerald-300" /> : <Clipboard className="h-4 w-4" />}
            </button>
          </div>
        </div>
      ))}
      <p className="text-xs leading-5 text-mga-muted">
        Run the all-users command from an elevated terminal. Recovery invalidates this profile’s sessions, resets it to <span className="font-mono text-mga-text">changeme</span>, and requires an immediate replacement at sign-in.
      </p>
    </div>
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
