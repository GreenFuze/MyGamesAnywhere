import { AlertCircle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ProgressBar } from '@/components/ui/progress-bar'
import type { LibraryLoadProgressModel } from '@/lib/libraryLoadProgress'

interface LibraryLoadProgressProps {
  model: LibraryLoadProgressModel
  onRetry: () => void
}

export function LibraryLoadProgress({ model, onRetry }: LibraryLoadProgressProps) {
  if (!model.isVisible) return null

  if (model.isError) {
    return (
      <section
        className="rounded-mga border border-red-400/30 bg-red-500/10 p-4"
        role="alert"
      >
        <div className="flex flex-wrap items-center gap-3">
          <AlertCircle className="shrink-0 text-red-300" size={20} aria-hidden="true" />
          <div className="min-w-0 flex-1">
            <p className="font-medium text-red-100">{model.title}</p>
            <p className="mt-1 text-sm text-red-100/75">{model.detail}</p>
          </div>
          <Button type="button" variant="outline" size="sm" onClick={onRetry}>
            Try again
          </Button>
        </div>
        <details className="mt-3 text-xs text-red-100/65">
          <summary className="cursor-pointer">Technical details</summary>
          <p className="mt-2 break-words">{model.errorMessage}</p>
        </details>
      </section>
    )
  }

  return (
    <section
      className="rounded-mga border border-mga-accent/30 bg-mga-accent/10 px-4 py-3"
      aria-live="polite"
      aria-busy="true"
    >
      <div className="mb-2 flex items-center justify-between gap-3">
        <p className="text-sm font-medium text-mga-text">{model.title}</p>
        {model.totalCount > 0 ? (
          <p className="text-xs text-mga-muted">{model.loadedCount} / {model.totalCount}</p>
        ) : null}
      </div>
      <ProgressBar value={model.percentage} label={model.detail} />
    </section>
  )
}
