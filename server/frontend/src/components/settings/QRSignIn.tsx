import { useCallback, useEffect, useRef, useState } from 'react'
import { QRCodeSVG } from 'qrcode.react'
import {
  ApiError,
  beginQRSignIn,
  pollQRSignIn,
  type ProviderIdentity,
  type QRSignInChallenge,
} from '@/api/client'
import { Button } from '@/components/ui/button'

interface QRSignInProps {
  pluginId: string
  integrationId: string
  /** Player-facing name of the provider app used to approve the sign-in. */
  providerAppName: string
  initialIdentity?: ProviderIdentity
  onSignedIn?: (identity: ProviderIdentity) => void | Promise<void>
}

type Phase = 'idle' | 'waiting' | 'signed_in' | 'error'

const DEFAULT_POLL_SECONDS = 5

function signInErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiError) {
    const providerMessage = error.responseText?.trim()
    if (providerMessage) return providerMessage
  }
  return error instanceof Error ? error.message : fallback
}

/**
 * QRSignIn drives a provider sign-in that the player approves in the provider's
 * own mobile app. MGA never sees the password or second factor: it only shows a
 * challenge link and waits for the provider to hand back a stored credential.
 */
export function QRSignIn({
  pluginId,
  integrationId,
  providerAppName,
  initialIdentity,
  onSignedIn,
}: QRSignInProps) {
  const [phase, setPhase] = useState<Phase>(initialIdentity ? 'signed_in' : 'idle')
  const [challenge, setChallenge] = useState<QRSignInChallenge | null>(null)
  const [identity, setIdentity] = useState<ProviderIdentity | null>(initialIdentity ?? null)
  const [libraryUpdateStarted, setLibraryUpdateStarted] = useState(false)
  const [message, setMessage] = useState('')
  const [copied, setCopied] = useState(false)
  const timerRef = useRef<number | null>(null)
  const cancelledRef = useRef(false)

  // Stop polling when the panel goes away so a closed dialog cannot keep
  // hitting the provider.
  const clearTimer = useCallback(() => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current)
      timerRef.current = null
    }
  }, [])

  useEffect(() => {
    cancelledRef.current = false
    return () => {
      cancelledRef.current = true
      clearTimer()
    }
  }, [clearTimer])

  const poll = useCallback(
    (session: QRSignInChallenge) => {
      const intervalMs = Math.max(session.interval_seconds ?? DEFAULT_POLL_SECONDS, 1) * 1000
      timerRef.current = window.setTimeout(async () => {
        if (cancelledRef.current) return
        try {
          const result = await pollQRSignIn(pluginId, integrationId, session.client_id, session.request_id)
          if (cancelledRef.current) return
          if (result.status === 'ok') {
            const signedInIdentity = result.provider_identity ?? {
              provider: pluginId,
              subject: '',
              display_name: result.account_name,
            }
            setPhase('signed_in')
            setIdentity(signedInIdentity)
            setLibraryUpdateStarted(true)
            setChallenge(null)
            void onSignedIn?.(signedInIdentity)
            return
          }
          poll(session)
        } catch (err) {
          if (cancelledRef.current) return
          setPhase('error')
          setMessage(signInErrorMessage(err, 'Sign-in did not complete.'))
        }
      }, intervalMs)
    },
    [integrationId, onSignedIn, pluginId],
  )

  const start = async () => {
    clearTimer()
    setPhase('waiting')
    setMessage('')
    setCopied(false)
    setLibraryUpdateStarted(false)
    try {
      const session = await beginQRSignIn(pluginId, integrationId)
      if (cancelledRef.current) return
      setChallenge(session)
      poll(session)
    } catch (err) {
      setPhase('error')
      setMessage(signInErrorMessage(err, 'Could not start sign-in.'))
    }
  }

  const copyLink = async () => {
    if (!challenge) return
    try {
      await navigator.clipboard.writeText(challenge.challenge_url)
      setCopied(true)
    } catch {
      setCopied(false)
    }
  }

  return (
    <div className="rounded-mga border border-mga-border bg-mga-bg/60 p-3 space-y-2">
      <div className="flex items-center justify-between gap-2">
        <div>
          <p className="text-sm font-medium text-mga-text">Sign in for shared games</p>
          <p className="text-xs text-mga-muted">
            Approve in the {providerAppName} app. Your password and login code never reach MGA.
          </p>
        </div>
        <Button type="button" variant="outline" onClick={start} disabled={phase === 'waiting'}>
          {phase === 'waiting' ? 'Waiting…' : phase === 'signed_in' ? 'Change account' : 'Sign in'}
        </Button>
      </div>

      {phase === 'waiting' && challenge && (
        <div className="space-y-2">
          <p className="text-xs text-mga-muted">
            Scan this code with the {providerAppName} mobile app, then approve the sign-in. This page
            updates by itself.
          </p>
          <div className="flex justify-center rounded-mga bg-white p-3">
            <QRCodeSVG
              value={challenge.challenge_url}
              size={196}
              level="M"
              marginSize={4}
              title={`Scan with the ${providerAppName} mobile app`}
            />
          </div>
          <p className="text-center text-xs text-mga-muted">Can&apos;t scan it? Copy the link instead.</p>
          <div className="flex items-center gap-2">
            <code className="flex-1 truncate rounded bg-mga-elevated px-2 py-1 text-xs text-mga-text">
              {challenge.challenge_url}
            </code>
            <Button type="button" variant="ghost" onClick={copyLink}>
              {copied ? 'Copied' : 'Copy'}
            </Button>
          </div>
        </div>
      )}

      {phase === 'signed_in' && identity && (
        <div className="flex items-center gap-3 rounded-mga border border-green-500/30 bg-green-500/10 p-3">
          {identity.avatar_url ? (
            <img
              src={identity.avatar_url}
              alt=""
              className="h-12 w-12 rounded-full border border-green-400/40 object-cover"
            />
          ) : (
            <div className="grid h-12 w-12 place-items-center rounded-full bg-green-500/20 text-lg text-green-300">
              ✓
            </div>
          )}
          <div>
            <p className="text-sm font-medium text-green-300">
              Logged in{identity.display_name ? ` as ${identity.display_name}` : ''}
            </p>
            <p className="text-xs text-mga-muted">
              {libraryUpdateStarted ? 'Updating your Steam library now…' : 'Your Steam account is connected.'}
            </p>
          </div>
        </div>
      )}

      {phase === 'error' && <p className="text-sm text-red-300">{message}</p>}
    </div>
  )
}
