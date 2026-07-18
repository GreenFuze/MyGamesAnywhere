import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { AlertCircle, CheckCircle2, Info, X } from 'lucide-react'
import { cn } from '@/lib/utils'

export type ToastTone = 'info' | 'success' | 'error'

export type NotificationDetail = {
  kind?: 'added' | 'removed' | 'info'
  title: string
  context?: string
}

export type NotificationAction = {
  label: string
  href: string
}

type Toast = {
  id: number
  title: string
  description?: string
  tone: ToastTone
  details?: NotificationDetail[]
  detailsOmitted?: number
  action?: NotificationAction
}

type ToastInput = Omit<Toast, 'id'>

export type NotificationHistoryItem = ToastInput & {
  id: string
  createdAt: string
  read: boolean
}

type ToastContextValue = {
  notify: (toast: ToastInput) => void
  dismiss: (id: number) => void
  notifications: NotificationHistoryItem[]
  unreadCount: number
  markAllRead: () => void
  clearHistory: () => void
}

const ToastContext = createContext<ToastContextValue | null>(null)

const toneStyles: Record<ToastTone, string> = {
  info: 'border-mga-border bg-mga-surface text-mga-text',
  success: 'border-emerald-500/30 bg-emerald-500/10 text-mga-text',
  error: 'border-red-500/30 bg-red-500/10 text-mga-text',
}

const toneIcons = {
  info: Info,
  success: CheckCircle2,
  error: AlertCircle,
} satisfies Record<ToastTone, typeof Info>

const MAX_NOTIFICATION_HISTORY = 100
const NOTIFICATION_HISTORY_VERSION = 1

class BrowserNotificationHistoryStore {
  private readonly storageKey: string

  constructor(scope: string) {
    const normalizedScope = scope.trim()
    if (!normalizedScope) {
      throw new Error('Notification history requires a non-empty profile scope')
    }
    this.storageKey = `mga.notification-history.v${NOTIFICATION_HISTORY_VERSION}:${normalizedScope}`
  }

  read(): NotificationHistoryItem[] {
    try {
      const raw = localStorage.getItem(this.storageKey)
      if (!raw) return []
      const parsed: unknown = JSON.parse(raw)
      if (!Array.isArray(parsed)) return []
      return parsed.filter(isNotificationHistoryItem).slice(0, MAX_NOTIFICATION_HISTORY)
    } catch {
      return []
    }
  }

  write(items: NotificationHistoryItem[]) {
    try {
      localStorage.setItem(this.storageKey, JSON.stringify(items.slice(0, MAX_NOTIFICATION_HISTORY)))
    } catch {
      // Browser storage can be unavailable in private or embedded contexts.
    }
  }
}

function isNotificationHistoryItem(value: unknown): value is NotificationHistoryItem {
  if (!value || typeof value !== 'object') return false
  const item = value as Partial<NotificationHistoryItem>
  const validDetails = item.details === undefined || (
    Array.isArray(item.details) && item.details.every(isNotificationDetail)
  )
  const validAction = item.action === undefined || isNotificationAction(item.action)
  return (
    typeof item.id === 'string' &&
    typeof item.title === 'string' &&
    (item.description === undefined || typeof item.description === 'string') &&
    (item.tone === 'info' || item.tone === 'success' || item.tone === 'error') &&
    typeof item.createdAt === 'string' &&
    typeof item.read === 'boolean' &&
    validDetails &&
    (item.detailsOmitted === undefined || (typeof item.detailsOmitted === 'number' && item.detailsOmitted >= 0)) &&
    validAction
  )
}

function isNotificationDetail(value: unknown): value is NotificationDetail {
  if (!value || typeof value !== 'object') return false
  const detail = value as Partial<NotificationDetail>
  return (
    typeof detail.title === 'string' &&
    (detail.context === undefined || typeof detail.context === 'string') &&
    (detail.kind === undefined || detail.kind === 'added' || detail.kind === 'removed' || detail.kind === 'info')
  )
}

function isNotificationAction(value: unknown): value is NotificationAction {
  if (!value || typeof value !== 'object') return false
  const action = value as Partial<NotificationAction>
  return typeof action.label === 'string' && typeof action.href === 'string' && isInternalNotificationPath(action.href)
}

function isInternalNotificationPath(value: string): boolean {
  return value.startsWith('/') && !value.startsWith('//')
}

export function ToastProvider({ children, historyScope }: { children: ReactNode; historyScope: string }) {
  const historyStoreRef = useRef(new BrowserNotificationHistoryStore(historyScope))
  const [toasts, setToasts] = useState<Toast[]>([])
  const [notifications, setNotifications] = useState<NotificationHistoryItem[]>(() => historyStoreRef.current.read())
  const nextIdRef = useRef(1)
  const timersRef = useRef(new Map<number, ReturnType<typeof setTimeout>>())

  const dismiss = useCallback((id: number) => {
    const timer = timersRef.current.get(id)
    if (timer) {
      clearTimeout(timer)
      timersRef.current.delete(id)
    }
    setToasts((prev) => prev.filter((toast) => toast.id !== id))
  }, [])

  const notify = useCallback(
    (input: ToastInput) => {
      const id = nextIdRef.current++
      setToasts((prev) => [...prev, { id, ...input }])
      setNotifications((prev) => [
        {
          ...input,
          id: `${Date.now()}-${id}`,
          createdAt: new Date().toISOString(),
          read: false,
        },
        ...prev,
      ].slice(0, MAX_NOTIFICATION_HISTORY))
      const timer = setTimeout(() => dismiss(id), 5000)
      timersRef.current.set(id, timer)
    },
    [dismiss],
  )

  const markAllRead = useCallback(() => {
    setNotifications((prev) => prev.map((item) => item.read ? item : { ...item, read: true }))
  }, [])

  const clearHistory = useCallback(() => {
    setNotifications([])
  }, [])

  useEffect(() => {
    historyStoreRef.current.write(notifications)
  }, [notifications])

  useEffect(() => {
    return () => {
      for (const timer of timersRef.current.values()) {
        clearTimeout(timer)
      }
      timersRef.current.clear()
    }
  }, [])

  const unreadCount = useMemo(
    () => notifications.reduce((count, item) => count + Number(!item.read), 0),
    [notifications],
  )
  const value = useMemo(
    () => ({ notify, dismiss, notifications, unreadCount, markAllRead, clearHistory }),
    [clearHistory, dismiss, markAllRead, notifications, notify, unreadCount],
  )

  return (
    <ToastContext.Provider value={value}>
      {children}
      <div className="pointer-events-none fixed right-4 top-4 z-50 flex w-full max-w-sm flex-col gap-3">
        {toasts.map((toast) => {
          const Icon = toneIcons[toast.tone]
          return (
            <div
              key={toast.id}
              className={cn(
                'pointer-events-auto rounded-mga border p-3 shadow-lg shadow-black/20 backdrop-blur',
                toneStyles[toast.tone],
              )}
            >
              <div className="flex items-start gap-3">
                <Icon
                  size={18}
                  className={cn(
                    'mt-0.5 shrink-0',
                    toast.tone === 'success'
                      ? 'text-emerald-400'
                      : toast.tone === 'error'
                        ? 'text-red-400'
                        : 'text-mga-accent',
                  )}
                />
                <div className="min-w-0 flex-1">
                  <p className="text-sm font-medium">{toast.title}</p>
                  {toast.description && (
                    <p className="mt-1 text-xs leading-5 text-mga-muted">{toast.description}</p>
                  )}
                </div>
                <button
                  type="button"
                  onClick={() => dismiss(toast.id)}
                  className="rounded-sm p-1 text-mga-muted transition-colors hover:bg-mga-elevated hover:text-mga-text"
                  aria-label="Dismiss notification"
                >
                  <X size={14} />
                </button>
              </div>
            </div>
          )
        })}
      </div>
    </ToastContext.Provider>
  )
}

export function useToast() {
  const ctx = useContext(ToastContext)
  if (!ctx) {
    throw new Error('useToast must be used inside <ToastProvider>')
  }
  return ctx
}
