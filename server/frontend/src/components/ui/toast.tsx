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

type ToastTone = 'info' | 'success' | 'error'

type Toast = {
  id: number
  title: string
  description?: string
  tone: ToastTone
}

type ToastInput = Omit<Toast, 'id'>

type ToastContextValue = {
  notify: (toast: ToastInput) => void
  dismiss: (id: number) => void
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

export function ToastProvider({ children }: { children: ReactNode }) {
  const [toasts, setToasts] = useState<Toast[]>([])
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
    ({ title, description, tone }: ToastInput) => {
      const id = nextIdRef.current++
      setToasts((prev) => [...prev, { id, title, description, tone }])
      const timer = setTimeout(() => dismiss(id), 5000)
      timersRef.current.set(id, timer)
    },
    [dismiss],
  )

  useEffect(() => {
    return () => {
      for (const timer of timersRef.current.values()) {
        clearTimeout(timer)
      }
      timersRef.current.clear()
    }
  }, [])

  const value = useMemo(() => ({ notify, dismiss }), [dismiss, notify])

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
