import { useMemo, useState } from 'react'
import { CheckCircle2, ClipboardPaste, ExternalLink, Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

type OAuthCallbackPanelProps = {
  providerLabel: string
  authorizeUrl: string
  remoteBrowserHint?: boolean
  pasteCallbackSupported?: boolean
  busy?: boolean
  error?: string | null
  complete?: boolean
  onOpenSignIn: () => void
  onSubmitCallback: (callbackUrl: string) => Promise<void> | void
  onCancel?: () => void
  className?: string
}

export function OAuthCallbackPanel({
  providerLabel,
  authorizeUrl,
  remoteBrowserHint,
  pasteCallbackSupported = true,
  busy,
  error,
  complete,
  onOpenSignIn,
  onSubmitCallback,
  onCancel,
  className,
}: OAuthCallbackPanelProps) {
  const [callbackUrl, setCallbackUrl] = useState('')
  const [pasteOpen, setPasteOpen] = useState(Boolean(remoteBrowserHint))
  const [submitting, setSubmitting] = useState(false)
  const canPaste = pasteCallbackSupported
  const trimmedCallbackUrl = callbackUrl.trim()
  const helperText = useMemo(() => {
    if (remoteBrowserHint) {
      return 'You are using MGA from another device. If sign-in cannot return to MGA, paste the final callback URL here.'
    }
    return 'Most local sign-ins complete automatically. Use paste only if the browser ends on a page that cannot connect.'
  }, [remoteBrowserHint])

  async function submitPastedCallback() {
    if (!trimmedCallbackUrl || submitting || busy) return
    setSubmitting(true)
    try {
      await onSubmitCallback(trimmedCallbackUrl)
      setCallbackUrl('')
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <div className={cn('space-y-4 rounded-mga border border-mga-border bg-mga-surface/80 p-4 text-left', className)}>
      <div className="flex items-start gap-3">
        <div className="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-mga-accent/15 text-mga-accent">
          {complete ? <CheckCircle2 className="h-5 w-5" /> : <ExternalLink className="h-5 w-5" />}
        </div>
        <div className="min-w-0 flex-1">
          <h3 className="text-base font-bold text-mga-text">Sign in to {providerLabel}</h3>
          <p className="mt-1 text-sm leading-6 text-mga-muted">{helperText}</p>
        </div>
      </div>

      <div className="grid gap-3 md:grid-cols-3">
        {[
          ['1', 'Sign in', 'Use the provider page opened by MGA.'],
          ['2', 'Return or paste callback', 'Automatic return is preferred; paste is the fallback.'],
          ['3', 'MGA verifies and continues', 'Tokens stay server-side and are never shown here.'],
        ].map(([step, title, copy]) => (
          <div key={step} className="rounded-mga border border-mga-border/70 bg-mga-bg p-3">
            <div className="text-xs font-black uppercase tracking-wide text-mga-accent">Step {step}</div>
            <div className="mt-1 text-sm font-bold text-mga-text">{title}</div>
            <div className="mt-1 text-xs leading-5 text-mga-muted">{copy}</div>
          </div>
        ))}
      </div>

      <div className="flex flex-wrap gap-3">
        <Button type="button" onClick={onOpenSignIn} disabled={busy || submitting}>
          <ExternalLink className="h-4 w-4" />
          Open sign-in again
        </Button>
        {canPaste ? (
          <Button type="button" variant="outline" onClick={() => setPasteOpen((value) => !value)} disabled={busy || submitting}>
            <ClipboardPaste className="h-4 w-4" />
            {pasteOpen ? 'Hide paste box' : 'Paste callback URL'}
          </Button>
        ) : null}
        {onCancel ? (
          <Button type="button" variant="outline" onClick={onCancel} disabled={submitting}>
            Cancel
          </Button>
        ) : null}
      </div>

      {pasteOpen && canPaste ? (
        <div className="space-y-3 rounded-mga border border-mga-border/70 bg-mga-bg p-3">
          <p className="text-sm leading-6 text-mga-muted">
            If the browser ends on a page that cannot connect, copy the full URL from the address bar and paste it here.
          </p>
          <textarea
            value={callbackUrl}
            onChange={(event) => setCallbackUrl(event.target.value)}
            placeholder="http://127.0.0.1:8900/api/auth/callback/..."
            className="min-h-24 w-full resize-y rounded-mga border border-mga-border bg-mga-surface px-3 py-2 font-mono text-xs text-mga-text outline-none transition focus:border-mga-accent focus:ring-2 focus:ring-mga-accent/25"
          />
          <Button type="button" onClick={submitPastedCallback} disabled={!trimmedCallbackUrl || submitting || busy}>
            {(submitting || busy) ? <Loader2 className="h-4 w-4 animate-spin" /> : <ClipboardPaste className="h-4 w-4" />}
            Submit callback URL
          </Button>
        </div>
      ) : null}

      {authorizeUrl ? (
        <p className="truncate text-xs text-mga-muted">
          Sign-in URL: <span className="font-mono">{authorizeUrl}</span>
        </p>
      ) : null}
      {complete ? <p className="text-sm font-semibold text-emerald-300">Authentication complete. MGA is continuing...</p> : null}
      {error ? <p className="text-sm text-red-400">{error}</p> : null}
      {!error && !complete ? <p className="text-sm text-mga-muted">{busy ? 'MGA is verifying the sign-in...' : 'Waiting for the provider callback.'}</p> : null}
    </div>
  )
}
