import type { LibraryStats } from '@/api/client'
import { platformLabel } from '@/lib/gameUtils'

interface LibraryStatsSummaryProps {
  stats: LibraryStats
}

export function LibraryStatsSummary({ stats }: LibraryStatsSummaryProps) {
  return (
    <div>
      {/* Stat cards */}
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3">
        <StatCard label="Games" value={stats.canonical_game_count} />
        <StatCard
          label="Description"
          value={`${Math.round(stats.percent_with_description)}%`}
          detail={`${stats.games_with_description} games`}
        />
        <StatCard
          label="Media"
          value={`${Math.round(stats.percent_with_media)}%`}
          detail={`${stats.games_with_media} games`}
        />
        <StatCard
          label="Achievements"
          value={`${Math.round(stats.percent_with_achievements)}%`}
          detail={`${stats.games_with_achievements} games`}
        />
        <StatCard
          label="Sources Found"
          value={stats.source_game_found_count}
        />
        <StatCard
          label="Sources Total"
          value={stats.source_game_total_count}
        />
        <StatCard
          label="Resolver Titles"
          value={`${Math.round(stats.percent_with_resolver_title)}%`}
          detail={`${stats.canonical_with_resolver_title} games`}
        />
      </div>

      {/* Platform breakdown */}
      {Object.keys(stats.by_platform).length > 0 && (
        <div className="mt-4">
          <span className="text-xs text-mga-muted uppercase tracking-wider">By Platform</span>
          <div className="flex flex-wrap gap-2 mt-2">
            {Object.entries(stats.by_platform)
              .sort(([, a], [, b]) => b - a)
              .map(([platform, count]) => (
                <span
                  key={platform}
                  className="text-xs bg-mga-elevated px-2 py-1 rounded-mga text-mga-text"
                >
                  {platformLabel(platform)}: {count}
                </span>
              ))}
          </div>
        </div>
      )}
    </div>
  )
}

function StatCard({
  label,
  value,
  detail,
}: {
  label: string
  value: number | string
  detail?: string
}) {
  return (
    <div className="border border-mga-border rounded-mga bg-mga-surface p-3 text-center">
      <div className="text-lg font-semibold text-mga-text">{value}</div>
      <div className="text-xs text-mga-muted mt-0.5">{label}</div>
      {detail && <div className="mt-1 text-[11px] text-mga-muted">{detail}</div>}
    </div>
  )
}
