import { useCallback, useEffect, useRef, useState } from 'react'
import { QRCodeSVG } from 'qrcode.react'
import { Check, QrCode } from 'lucide-react'
import {
  ApiError, beginQRSignIn, pollQRSignIn,
  type ProviderIdentity, type QRSignInChallenge,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { rotateQRChallenge } from '@/lib/qrSignIn'

/**
 * Signing in to a provider by approving it in that provider's own app.
 *
 * MGA never sees the password or the second factor. It shows a challenge, the
 * player approves it on their phone, and the provider hands back a credential
 * MGA can renew for months.
 *
 * This existed and was deleted by mistake in the retirement of the first-party
 * player (338ce345), which took the whole settings shell with it. The server
 * side never went anywhere: /api/auth/qr/{plugin}/begin and /poll have been
 * answering the entire time with nothing to call them.
 */

type Phase = 'idle' | 'waiting' | 'signed_in' | 'error'

const DEFAULT_POLL_SECONDS = 5

function signInErrorMessage(error: unknown, fallback: string): string {
  if (error instanceof ApiError) {
    const providerMessage = error.responseText?.trim()
    if (providerMessage) return providerMessage
  }
  return error instanceof Error ? error.message : fallback
}

export function QRSignIn({
  pluginId, integrationId, providerAppName, purposeLabel, initialIdentity, autoStart, onSignedIn,
}: {
  pluginId: string
  integrationId: string
  /** The app the player approves it in, named as they know it. */
  providerAppName: string
  /** What this sign-in is for, in the player's terms. */
  purposeLabel: string
  initialIdentity?: ProviderIdentity
  /** Ask for the code as soon as the panel appears, for a caller whose own
   *  button already said "sign in" — two clicks to start one sign-in is one
   *  click too many. */
  autoStart?: boolean
  onSignedIn?: (identity: ProviderIdentity) => void | Promise<void>
}) {
  const [phase, setPhase] = useState<Phase>(initialIdentity ? 'signed_in' : 'idle')
  const [challenge, setChallenge] = useState<QRSignInChallenge | null>(null)
  const [identity, setIdentity] = useState<ProviderIdentity | null>(initialIdentity ?? null)
  const [message, setMessage] = useState('')
  const [copied, setCopied] = useState(false)
  const timerRef = useRef<number | null>(null)
  const cancelledRef = useRef(false)

  // Stop polling when the panel goes away, so a closed dialog cannot keep
  // asking the provider about a sign-in nobody is waiting for.
  const clearTimer = useCallback(() => {
    if (timerRef.current !== null) {
      window.clearTimeout(timerRef.current)
      timerRef.current = null
    }
  }, [])

  useEffect(() => {
    cancelledRef.current = false
    return () => { cancelledRef.current = true; clearTimer() }
  }, [clearTimer])

  const poll = useCallback((session: QRSignInChallenge) => {
    const intervalMs = Math.max(session.interval_seconds ?? DEFAULT_POLL_SECONDS, 1) * 1000
    timerRef.current = window.setTimeout(async () => {
      if (cancelledRef.current) return
      try {
        const result = await pollQRSignIn(pluginId, integrationId, session.client_id, session.request_id)
        if (cancelledRef.current) return
        if (result.status === 'ok') {
          const signedIn = result.provider_identity ?? {
            provider: pluginId,
            subject: '',
            display_name: result.account_name,
          }
          setPhase('signed_in')
          setIdentity(signedIn)
          setChallenge(null)
          void onSignedIn?.(signedIn)
          return
        }
        // The provider rotates the code while keeping the same session, so a
        // replacement replaces the picture and nothing else.
        const active = rotateQRChallenge(session, result.challenge_url)
        if (active !== session) setChallenge(active)
        poll(active)
      } catch (err) {
        if (cancelledRef.current) return
        setPhase('error')
        setMessage(signInErrorMessage(err, 'The sign-in did not finish.'))
      }
    }, intervalMs)
  }, [integrationId, onSignedIn, pluginId])

  const start = useCallback(async () => {
    clearTimer()
    setPhase('waiting')
    setMessage('')
    setCopied(false)
    try {
      const session = await beginQRSignIn(pluginId, integrationId)
      if (cancelledRef.current) return
      setChallenge(session)
      poll(session)
    } catch (err) {
      setPhase('error')
      setMessage(signInErrorMessage(err, 'The sign-in could not be started.'))
    }
  }, [clearTimer, integrationId, pluginId, poll])

  const started = useRef(false)
  useEffect(() => {
    if (!autoStart || started.current || initialIdentity) return
    started.current = true
    void start()
  }, [autoStart, initialIdentity, start])

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
    <div className="space-y-3 rounded-lg border border-mga-border bg-mga-elevated/40 p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium text-mga-text">{purposeLabel}</p>
          <p className="mt-1 text-xs leading-5 text-mga-muted">
            Approve it in the {providerAppName} app. Your password and your sign-in code never reach MGA.
          </p>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={start} disabled={phase === 'waiting'}>
          <QrCode className="h-3.5 w-3.5" />
          {phase === 'waiting' ? 'Waiting…' : phase === 'signed_in' ? 'Use another account' : 'Sign in'}
        </Button>
      </div>

      {phase === 'waiting' && challenge && (
        <div className="space-y-2">
          <p className="text-xs leading-5 text-mga-muted">
            Scan this with the {providerAppName} app and approve it. This page notices by itself.
          </p>
          <div className="flex justify-center rounded-lg bg-white p-3">
            <QRCodeSVG
              value={challenge.challenge_url}
              size={196}
              level="M"
              marginSize={4}
              title={`Scan with the ${providerAppName} app`}
            />
          </div>
          <div className="flex items-center gap-2">
            <code className="flex-1 truncate rounded bg-mga-bg px-2 py-1 text-[0.68rem] text-mga-text">
              {challenge.challenge_url}
            </code>
            <Button type="button" variant="ghost" size="sm" onClick={copyLink}>
              {copied ? 'Copied' : 'Copy link'}
            </Button>
          </div>
          <p className="text-[0.68rem] text-mga-muted">No camera? Open the link on the phone instead.</p>
        </div>
      )}

      {phase === 'signed_in' && identity && (
        <div className="flex items-center gap-3 rounded-lg border border-emerald-400/25 bg-emerald-400/5 p-3">
          {identity.avatar_url ? (
            <img src={identity.avatar_url} alt="" className="h-10 w-10 rounded-full border border-emerald-400/40 object-cover" />
          ) : (
            <span className="grid h-10 w-10 place-items-center rounded-full bg-emerald-400/15 text-emerald-300">
              <Check className="h-5 w-5" />
            </span>
          )}
          <div className="min-w-0">
            <p className="truncate text-sm font-medium text-emerald-300">
              Signed in{identity.display_name ? ` as ${identity.display_name}` : ''}
            </p>
            <p className="text-xs text-mga-muted">Scan this connection to pick up what it can now see.</p>
          </div>
        </div>
      )}

      {phase === 'error' && <p className="text-xs text-rose-300" role="alert">{message}</p>}
    </div>
  )
}
