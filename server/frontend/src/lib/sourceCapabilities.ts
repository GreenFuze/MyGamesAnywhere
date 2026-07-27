import type { GameLaunchOptionDTO, SourceGameDetailDTO } from '../api/client.ts'
import { platformLabel, sourceLabel } from './displayText.ts'

function distinctParts(parts: Array<string | undefined>): string[] {
  const seen = new Set<string>()
  const result: string[] = []
  for (const candidate of parts) {
    const part = candidate?.trim()
    if (!part) continue
    const key = part.toLocaleLowerCase()
    if (seen.has(key)) continue
    seen.add(key)
    result.push(part)
  }
  return result
}

export function sourceVersionContext(
  source: Pick<SourceGameDetailDTO, 'plugin_id' | 'integration_label' | 'platform' | 'raw_title'>,
): string {
  return distinctParts([
    source.integration_label || (source.plugin_id ? sourceLabel(source.plugin_id) : undefined),
    source.platform && source.platform !== 'unknown' ? platformLabel(source.platform) : undefined,
    source.raw_title,
  ]).join(' · ')
}

export function launchOptionVersionContext(
  option: GameLaunchOptionDTO,
  sources: SourceGameDetailDTO[],
): string {
  const source = sources.find((candidate) => candidate.id === option.source_game_id)
  if (source) return sourceVersionContext(source)
  return distinctParts([
    option.integration_label || (option.plugin_id ? sourceLabel(option.plugin_id) : undefined),
    option.platform && option.platform !== 'unknown' ? platformLabel(option.platform) : undefined,
    option.source_title,
  ]).join(' · ')
}
