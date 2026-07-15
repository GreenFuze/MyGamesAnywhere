import { useEffect, useRef, useState } from 'react'
import { AlertCircle, Bell, CheckCircle2, Info, Trash2 } from 'lucide-react'
import { useDateTimeFormat } from '@/hooks/useDateTimeFormat'
import { cn } from '@/lib/utils'
import { useToast, type ToastTone } from '@/components/ui/toast'

const toneIcons = {
  info: Info,
  success: CheckCircle2,
  error: AlertCircle,
} satisfies Record<ToastTone, typeof Info>

const toneStyles: Record<ToastTone, string> = {
  info: 'bg-mga-accent/15 text-mga-accent',
  success: 'bg-emerald-500/15 text-emerald-400',
  error: 'bg-red-500/15 text-red-400',
}

export function NotificationCenter() {
  const { notifications, unreadCount, markAllRead, clearHistory } = useToast()
  const { format } = useDateTimeFormat()
  const [open, setOpen] = useState(false)
  const centerRef = useRef<HTMLDivElement | null>(null)

  useEffect(() => {
    if (!open) return

    function onPointerDown(event: PointerEvent) {
      if (!centerRef.current?.contains(event.target as Node)) setOpen(false)
    }

    function onKeyDown(event: KeyboardEvent) {
      if (event.key === 'Escape') setOpen(false)
    }

    document.addEventListener('pointerdown', onPointerDown)
    document.addEventListener('keydown', onKeyDown)
    return () => {
      document.removeEventListener('pointerdown', onPointerDown)
      document.removeEventListener('keydown', onKeyDown)
    }
  }, [open])

  function toggleOpen() {
    const next = !open
    setOpen(next)
    if (next) markAllRead()
  }

  return (
    <div ref={centerRef} className="relative shrink-0">
      <button
        type="button"
        onClick={toggleOpen}
        className="relative grid h-9 w-9 place-items-center rounded-mga border border-mga-border bg-mga-bg text-mga-muted transition-colors hover:bg-mga-elevated hover:text-mga-text focus:outline-none focus-visible:ring-2 focus-visible:ring-mga-accent"
        title="Notifications"
        aria-label={unreadCount > 0 ? `Notifications, ${unreadCount} unread` : 'Notifications'}
        aria-expanded={open}
      >
        <Bell className="h-4 w-4" />
        {unreadCount > 0 ? (
          <span className="absolute -right-1.5 -top-1.5 grid min-h-4 min-w-4 place-items-center rounded-full bg-mga-accent px-1 text-[9px] font-black leading-none text-mga-bg ring-2 ring-mga-surface">
            {unreadCount > 99 ? '99+' : unreadCount}
          </span>
        ) : null}
      </button>

      {open ? (
        <section
          className="absolute right-0 top-[calc(100%+0.5rem)] z-50 w-[min(24rem,calc(100vw-1.5rem))] overflow-hidden rounded-mga border border-mga-border bg-mga-surface shadow-2xl"
          aria-label="Notification history"
        >
          <div className="flex items-center justify-between gap-3 border-b border-mga-border bg-mga-bg/70 px-3 py-2.5">
            <div>
              <h2 className="text-sm font-bold text-mga-text">Notifications</h2>
              <p className="text-[10px] text-mga-muted">Stored for this profile in this browser</p>
            </div>
            <div className="flex items-center gap-1">
              {notifications.length > 0 ? (
                <button
                  type="button"
                  onClick={clearHistory}
                  className="rounded-mga p-1.5 text-mga-muted transition-colors hover:bg-red-500/10 hover:text-red-400"
                  title="Clear notification history"
                  aria-label="Clear notification history"
                >
                  <Trash2 className="h-4 w-4" />
                </button>
              ) : null}
            </div>
          </div>

          {notifications.length === 0 ? (
            <div className="grid min-h-36 place-items-center px-6 py-8 text-center">
              <div>
                <Bell className="mx-auto h-6 w-6 text-mga-muted" />
                <p className="mt-2 text-sm font-medium text-mga-text">No notifications yet</p>
                <p className="mt-1 text-xs leading-5 text-mga-muted">
                  Scan results, integration status changes, and errors will appear here.
                </p>
              </div>
            </div>
          ) : (
            <div className="max-h-[min(32rem,70vh)] overflow-y-auto">
              {notifications.map((notification) => {
                const Icon = toneIcons[notification.tone]
                return (
                  <article
                    key={notification.id}
                    className={cn(
                      'flex gap-3 border-b border-mga-border/70 px-3 py-3 last:border-b-0',
                      notification.read ? 'bg-mga-surface' : 'bg-mga-accent/[0.04]',
                    )}
                  >
                    <span className={cn('mt-0.5 grid h-8 w-8 shrink-0 place-items-center rounded-full', toneStyles[notification.tone])}>
                      <Icon className="h-4 w-4" />
                    </span>
                    <div className="min-w-0 flex-1">
                      <div className="flex items-start justify-between gap-3">
                        <h3 className="text-xs font-semibold leading-5 text-mga-text">{notification.title}</h3>
                        {!notification.read ? <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-mga-accent" /> : null}
                      </div>
                      {notification.description ? (
                        <p className="mt-0.5 text-[11px] leading-5 text-mga-muted">{notification.description}</p>
                      ) : null}
                      <time className="mt-1 block text-[10px] text-mga-muted" dateTime={notification.createdAt}>
                        {format(notification.createdAt)}
                      </time>
                    </div>
                  </article>
                )
              })}
            </div>
          )}
        </section>
      ) : null}
    </div>
  )
}
