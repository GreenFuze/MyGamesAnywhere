import { useQuery } from '@tanstack/react-query'
import { FileImage, Layers3 } from 'lucide-react'
import { listGames } from '@/api/client'
import { MetricCard, PageIntro, QueryFeedback, SectionCard, StatusPill, formatCount } from '@/components/management/ManagementPrimitives'

export function LibraryManagementPage() {
  const library = useQuery({ queryKey: ['management', 'library', { page: 0, pageSize: 18 }], queryFn: () => listGames({ page: 0, page_size: 18, sort_by: 'title', sort_dir: 'asc' }) })
  const games = library.data?.games ?? []
  const withMedia = games.filter((game) => (game.media?.length ?? 0) > 0).length
  return (
    <div className="mga-page-enter space-y-7">
      <PageIntro eyebrow="Inventory" title="Managed library" description="Inspect normalized game records, source coverage, and metadata readiness. Content delivery belongs to authorized frontend integrations—not this browser." />
      <div className="grid gap-4 sm:grid-cols-3">
        <MetricCard label="Canonical records" value={formatCount(library.data?.total)} detail="Visible to the selected profile" icon={<Layers3 className="h-4 w-4" />} />
        <MetricCard label="Sample with media" value={`${withMedia}/${games.length}`} detail="Media coverage in the currently loaded records" tone={games.length && withMedia < games.length ? 'attention' : 'good'} icon={<FileImage className="h-4 w-4" />} />
        <MetricCard label="Execution authority" value="None" detail="No install, repair, uninstall, elevation, or launch actions" tone="good" />
      </div>
      <SectionCard title="Library records" description="A compact management projection; deeper metadata and media workflows follow in MGA-101.">
        <QueryFeedback pending={library.isPending} error={library.error} empty={!library.isPending && games.length === 0} emptyTitle="The managed library is empty" emptyDescription="Connect a source and synchronize it to create normalized records for this profile." />
        {games.length > 0 && <div className="divide-y divide-mga-border/70 overflow-hidden rounded-lg border border-mga-border">
          {games.map((game) => <div key={game.id} className="grid gap-3 bg-mga-elevated/35 px-4 py-3 sm:grid-cols-[minmax(0,1.5fr)_minmax(0,0.8fr)_auto] sm:items-center">
            <div className="min-w-0"><p className="truncate text-sm font-medium text-mga-text">{game.title}</p><p className="mt-1 truncate text-xs text-mga-muted">{game.source_games?.length ?? 0} source record{(game.source_games?.length ?? 0) === 1 ? '' : 's'} · {game.kind || 'game'}</p></div>
            <p className="text-xs text-mga-muted">{game.platform || 'Unknown platform'}</p>
            <StatusPill label={(game.media?.length ?? 0) > 0 ? 'Media ready' : 'Needs media'} tone={(game.media?.length ?? 0) > 0 ? 'good' : 'attention'} />
          </div>)}
        </div>}
      </SectionCard>
    </div>
  )
}
