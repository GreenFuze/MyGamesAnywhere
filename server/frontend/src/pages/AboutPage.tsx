export function AboutPage() {
  return (
    <div className="space-y-8">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center">
        <img
          src="/logo.png"
          alt=""
          width={96}
          height={96}
          className="h-20 w-20 shrink-0 rounded-mga border border-mga-border bg-mga-elevated object-contain p-1"
        />
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">MyGamesAnywhere</h1>
          <p className="mt-1 text-sm text-mga-muted">
            Local-first game library — scan sources, enrich metadata, play anywhere.
          </p>
        </div>
      </div>

      <div className="overflow-hidden rounded-mga border border-mga-border bg-mga-surface">
        <img
          src="/title.png"
          alt="MyGamesAnywhere title"
          className="block w-full max-w-2xl"
        />
      </div>

      <p className="max-w-xl text-sm text-mga-muted">
        Server runs on your machine; data stays local unless you choose to sync. Phase 1 focuses on shell,
        themes, and API wiring — full library UI comes in Phase 2.
      </p>
    </div>
  )
}
