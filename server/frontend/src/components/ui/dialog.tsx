import { cn } from '@/lib/utils'
import { useEffect, useRef, type ReactNode } from 'react'

interface DialogProps {
  open: boolean
  onClose: () => void
  title: string
  children: ReactNode
  className?: string
}

/** Modal dialog with backdrop. Uses native <dialog> for accessibility. */
export function Dialog({ open, onClose, title, children, className }: DialogProps) {
  const ref = useRef<HTMLDialogElement>(null!)

  useEffect(() => {
    const el = ref.current
    if (open && !el.open) {
      el.showModal()
    } else if (!open && el.open) {
      el.close()
    }
  }, [open])

  // Handle backdrop click.
  const handleClick = (e: React.MouseEvent<HTMLDialogElement>) => {
    if (e.target === ref.current) onClose()
  }

  return (
    <dialog
      ref={ref}
      onClick={handleClick}
      onClose={onClose}
      className={cn(
        'backdrop:bg-black/50 bg-mga-surface text-mga-text',
        'rounded-mga border border-mga-border shadow-xl',
        'p-0 max-w-lg w-full',
        className,
      )}
    >
      <div className="p-6">
        {/* Header */}
        <div className="flex items-center justify-between mb-4">
          <h2 className="text-lg font-semibold">{title}</h2>
          <button
            type="button"
            onClick={onClose}
            className="text-mga-muted hover:text-mga-text transition-colors p-1"
            aria-label="Close"
          >
            ✕
          </button>
        </div>

        {/* Content */}
        {children}
      </div>
    </dialog>
  )
}

interface ConfirmDialogProps {
  open: boolean
  onClose: () => void
  onConfirm: () => void
  title: string
  message: string
  confirmLabel?: string
  confirmVariant?: 'danger' | 'primary'
}

/** Pre-built confirmation dialog with Cancel / Confirm buttons. */
export function ConfirmDialog({
  open,
  onClose,
  onConfirm,
  title,
  message,
  confirmLabel = 'Confirm',
  confirmVariant = 'primary',
}: ConfirmDialogProps) {
  return (
    <Dialog open={open} onClose={onClose} title={title}>
      <p className="text-mga-muted text-sm mb-6">{message}</p>
      <div className="flex justify-end gap-3">
        <button
          type="button"
          onClick={onClose}
          className="px-4 py-2 text-sm rounded-mga border border-mga-border text-mga-muted hover:text-mga-text hover:bg-mga-elevated transition-colors"
        >
          Cancel
        </button>
        <button
          type="button"
          onClick={() => {
            onConfirm()
            onClose()
          }}
          className={cn(
            'px-4 py-2 text-sm rounded-mga font-medium transition-colors',
            confirmVariant === 'danger'
              ? 'bg-red-500/20 text-red-400 hover:bg-red-500/30'
              : 'bg-mga-accent text-white hover:bg-mga-accent/80',
          )}
        >
          {confirmLabel}
        </button>
      </div>
    </Dialog>
  )
}
