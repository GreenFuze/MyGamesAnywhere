import { useEffect, useState, type FormEvent, type ReactNode } from 'react'
import { AlertTriangle, Check, Copy, LoaderCircle, ShieldAlert } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { cn } from '@/lib/utils'

/** Renders a failed management write in place, next to the control that caused
 * it. Management mutations must never fail silently. */
export function ActionError({ error, className }: { error: unknown; className?: string }) {
  if (!error) return null
  const message = error instanceof Error ? error.message : 'The server rejected this change.'
  return (
    <p className={cn('flex items-start gap-2 text-xs leading-5 text-rose-300', className)} role="alert">
      <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
      <span>{message}</span>
    </p>
  )
}

/**
 * A modal form for one management write. It owns submit wiring, the busy
 * state, and error placement so every create/edit surface behaves identically.
 */
export function FormDialog({
  open,
  onClose,
  title,
  description,
  submitLabel,
  submitting,
  error,
  onSubmit,
  disabled,
  destructive,
  hideSubmit,
  leading,
  children,
}: {
  open: boolean
  onClose: () => void
  title: string
  description?: string
  submitLabel: string
  submitting?: boolean
  error?: unknown
  onSubmit: () => void
  disabled?: boolean
  destructive?: boolean
  /** Multi-step forms hide the submit until the final step. */
  hideSubmit?: boolean
  /** Left-aligned footer control, such as a Back button. */
  leading?: ReactNode
  children: ReactNode
}) {
  const handleSubmit = (event: FormEvent) => {
    event.preventDefault()
    if (submitting || disabled) return
    onSubmit()
  }
  return (
    <Dialog open={open} onClose={onClose} title={title} className="max-w-xl">
      <form onSubmit={handleSubmit} className="space-y-4">
        {description && <p className="text-xs leading-5 text-mga-muted">{description}</p>}
        {children}
        <ActionError error={error} />
        <div className="flex items-center gap-2 border-t border-mga-border/70 pt-4">
          {leading}
          <div className="ml-auto flex gap-2">
            <Button type="button" variant="outline" onClick={onClose} disabled={submitting}>Cancel</Button>
            {!hideSubmit && (
              <Button
                type="submit"
                disabled={submitting || disabled}
                className={destructive ? 'bg-rose-500 text-white hover:opacity-90' : undefined}
              >
                {submitting && <LoaderCircle className="h-4 w-4 animate-spin" />}
                {submitLabel}
              </Button>
            )}
          </div>
        </div>
      </form>
    </Dialog>
  )
}

/**
 * Confirmation for a destructive management action. `consequences` must state
 * exactly what changes, and `preserves` states what is deliberately left
 * untouched, so an operator can never mistake a metadata change for content
 * deletion.
 */
export function ConfirmDialog({
  open,
  onClose,
  title,
  confirmLabel,
  consequences,
  preserves,
  submitting,
  error,
  onConfirm,
}: {
  open: boolean
  onClose: () => void
  title: string
  confirmLabel: string
  consequences: string[]
  preserves?: string[]
  submitting?: boolean
  error?: unknown
  onConfirm: () => void
}) {
  return (
    <FormDialog
      open={open}
      onClose={onClose}
      title={title}
      submitLabel={confirmLabel}
      submitting={submitting}
      error={error}
      onSubmit={onConfirm}
      destructive
    >
      <div className="rounded-lg border border-rose-400/25 bg-rose-500/5 p-4">
        <p className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wider text-rose-200">
          <ShieldAlert className="h-4 w-4" /> This will
        </p>
        <ul className="mt-2 space-y-1 text-xs leading-5 text-mga-text">
          {consequences.map((item) => <li key={item}>• {item}</li>)}
        </ul>
      </div>
      {preserves && preserves.length > 0 && (
        <div className="rounded-lg border border-emerald-400/20 bg-emerald-400/5 p-4">
          <p className="text-xs font-semibold uppercase tracking-wider text-emerald-300">This will not</p>
          <ul className="mt-2 space-y-1 text-xs leading-5 text-mga-muted">
            {preserves.map((item) => <li key={item}>• {item}</li>)}
          </ul>
        </div>
      )}
    </FormDialog>
  )
}

/**
 * Shows a credential exactly once. The server never returns it again, so the
 * component makes that explicit and offers a copy affordance rather than
 * pretending the value can be retrieved later.
 */
export function ShowOnceSecret({ label, value, warning }: { label: string; value: string; warning?: string }) {
  const [copied, setCopied] = useState(false)

  // Reset the acknowledgement whenever a different secret is shown.
  useEffect(() => setCopied(false), [value])

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
    } catch {
      // Clipboard access can be denied; the value stays selectable on screen.
      setCopied(false)
    }
  }

  return (
    <div className="rounded-lg border border-amber-400/30 bg-amber-400/5 p-4">
      <p className="text-xs font-semibold uppercase tracking-wider text-amber-200">{label}</p>
      <p className="mt-1 text-xs leading-5 text-mga-muted">
        This value is shown once and cannot be retrieved again. Store it now.
      </p>
      <div className="mt-3 flex items-center gap-2">
        <code className="min-w-0 flex-1 truncate rounded-md border border-mga-border bg-mga-bg px-3 py-2 font-mono text-xs text-mga-text">{value}</code>
        <Button type="button" variant="outline" size="sm" onClick={() => void copy()}>
          {copied ? <Check className="h-3.5 w-3.5" /> : <Copy className="h-3.5 w-3.5" />}
          {copied ? 'Copied' : 'Copy'}
        </Button>
      </div>
      {warning && <p className="mt-3 text-xs leading-5 text-amber-200/90">{warning}</p>}
    </div>
  )
}

/** Explains why a management action is unavailable to this profile instead of
 * hiding the capability without a reason. */
export function RestrictedNotice({ children }: { children: ReactNode }) {
  return (
    <div className="rounded-lg border border-amber-400/20 bg-amber-400/5 p-4 text-xs leading-5 text-mga-muted">
      {children}
    </div>
  )
}
