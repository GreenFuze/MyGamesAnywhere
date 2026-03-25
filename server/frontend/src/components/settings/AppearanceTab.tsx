import { useTheme } from '@/theme/ThemeProvider'
import { THEME_IDS, THEME_LABELS, THEME_PRESETS, type ThemeId } from '@/theme/presets'
import { useDateTimeFormat, formatDateTime, type DateFormat, type TimeFormat } from '@/hooks/useDateTimeFormat'
import { cn } from '@/lib/utils'

/** Swatch keys to display in the mini preview. */
const SWATCH_KEYS = [
  '--mga-bg',
  '--mga-surface',
  '--mga-elevated',
  '--mga-text',
  '--mga-accent',
] as const

export function AppearanceTab() {
  const { themeId, setThemeId } = useTheme()
  const { prefs, setDateFormat, setTimeFormat } = useDateTimeFormat()

  return (
    <div className="space-y-8">
      {/* Theme section */}
      <div className="space-y-4">
        <div>
          <h3 className="text-sm font-medium text-mga-text">Theme</h3>
          <p className="text-xs text-mga-muted mt-0.5">Choose your visual theme</p>
        </div>

        <div className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 gap-3">
          {THEME_IDS.map((id) => (
            <ThemeCard
              key={id}
              id={id}
              selected={id === themeId}
              onClick={() => setThemeId(id)}
            />
          ))}
        </div>
      </div>

      {/* Date & Time section */}
      <div className="space-y-4">
        <div>
          <h3 className="text-sm font-medium text-mga-text">Date & Time</h3>
          <p className="text-xs text-mga-muted mt-0.5">Choose how dates and times are displayed</p>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          {/* Date format */}
          <div className="space-y-2">
            <span className="text-xs font-medium text-mga-muted uppercase tracking-wider">Date Format</span>
            <div className="flex gap-2">
              {(['M/d/yyyy', 'd/M/yyyy'] as DateFormat[]).map((fmt) => (
                <button
                  key={fmt}
                  type="button"
                  onClick={() => setDateFormat(fmt)}
                  className={cn(
                    'flex-1 px-3 py-2 rounded-mga border text-sm transition-all',
                    prefs.dateFormat === fmt
                      ? 'border-mga-accent bg-mga-accent/10 text-mga-accent font-medium'
                      : 'border-mga-border bg-mga-surface text-mga-muted hover:border-mga-muted',
                  )}
                >
                  {fmt}
                </button>
              ))}
            </div>
          </div>

          {/* Time format */}
          <div className="space-y-2">
            <span className="text-xs font-medium text-mga-muted uppercase tracking-wider">Time Format</span>
            <div className="flex gap-2">
              {(['12h', '24h'] as TimeFormat[]).map((fmt) => (
                <button
                  key={fmt}
                  type="button"
                  onClick={() => setTimeFormat(fmt)}
                  className={cn(
                    'flex-1 px-3 py-2 rounded-mga border text-sm transition-all',
                    prefs.timeFormat === fmt
                      ? 'border-mga-accent bg-mga-accent/10 text-mga-accent font-medium'
                      : 'border-mga-border bg-mga-surface text-mga-muted hover:border-mga-muted',
                  )}
                >
                  {fmt === '12h' ? '12-hour' : '24-hour'}
                </button>
              ))}
            </div>
          </div>
        </div>

        {/* Live preview */}
        <div className="border border-mga-border rounded-mga bg-mga-surface px-4 py-3">
          <span className="text-xs text-mga-muted">Preview: </span>
          <span className="text-sm text-mga-text font-mono">
            {formatDateTime(new Date().toISOString(), prefs)}
          </span>
        </div>
      </div>
    </div>
  )
}

function ThemeCard({
  id,
  selected,
  onClick,
}: {
  id: ThemeId
  selected: boolean
  onClick: () => void
}) {
  const vars = THEME_PRESETS[id]

  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        'relative rounded-mga border p-3 text-left transition-all',
        'hover:scale-[1.02]',
        selected
          ? 'border-mga-accent ring-2 ring-mga-accent/30'
          : 'border-mga-border hover:border-mga-muted',
      )}
      style={{ backgroundColor: vars['--mga-bg'] }}
    >
      {/* Selected badge */}
      {selected && (
        <span
          className="absolute top-1.5 right-1.5 text-[10px] font-bold px-1.5 py-0.5 rounded-full"
          style={{ backgroundColor: vars['--mga-accent'], color: vars['--mga-bg'] }}
        >
          Active
        </span>
      )}

      {/* Theme name */}
      <span
        className="block text-sm font-medium mb-2"
        style={{ color: vars['--mga-text'] }}
      >
        {THEME_LABELS[id]}
      </span>

      {/* Color swatches */}
      <div className="flex gap-1">
        {SWATCH_KEYS.map((key) => (
          <span
            key={key}
            className="h-5 flex-1 rounded-sm border border-white/10"
            style={{ backgroundColor: vars[key] }}
            title={key.replace('--mga-', '')}
          />
        ))}
      </div>

      {/* Mini UI preview */}
      <div
        className="mt-2 rounded-sm p-1.5 text-[8px] leading-tight"
        style={{ backgroundColor: vars['--mga-surface'], border: `1px solid ${vars['--mga-border'] ?? 'transparent'}` }}
      >
        <span style={{ color: vars['--mga-text'] }}>Title</span>
        <span className="ml-1" style={{ color: vars['--mga-muted'] }}>subtitle</span>
        <div className="mt-1 flex gap-1">
          <span
            className="px-1 rounded-sm"
            style={{ backgroundColor: vars['--mga-accent'], color: vars['--mga-bg'] }}
          >
            btn
          </span>
          <span
            className="px-1 rounded-sm"
            style={{ backgroundColor: vars['--mga-elevated'], color: vars['--mga-muted'] }}
          >
            tag
          </span>
        </div>
      </div>
    </button>
  )
}
