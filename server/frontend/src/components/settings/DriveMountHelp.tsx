import { useState } from 'react'
import { Copy, Check, ExternalLink } from 'lucide-react'
import { MOUNT_PLATFORMS, guessPlatform, mountInstructions, type MountPlatform } from '@/lib/driveMountHelp'

/**
 * Shown when no synced Google Drive folder could be found.
 *
 * On Linux this is the expected outcome rather than a failure, because Google
 * ships no Drive client for it, so the panel has to be a way forward rather
 * than an apology. Everything is text to copy: a downloaded PowerShell script
 * will not run under the default Windows execution policy, and handing someone
 * a file they cannot execute is worse than handing them instructions.
 */
export function DriveMountHelp() {
  const [platform, setPlatform] = useState<MountPlatform>(() =>
    guessPlatform(typeof navigator === 'undefined' ? '' : navigator.userAgent),
  )
  const [copied, setCopied] = useState(false)
  const instructions = mountInstructions(platform)

  async function copyScript() {
    try {
      await navigator.clipboard.writeText(instructions.script)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    } catch {
      // Clipboard access can be refused; the text is on screen either way, so
      // failing silently is better than an error about a convenience.
      setCopied(false)
    }
  }

  return (
    <div className="space-y-3 p-4 text-left">
      <div>
        <p className="text-sm font-medium text-mga-text">No Google Drive folder found on this server</p>
        <p className="mt-1 text-xs leading-5 text-mga-muted">{instructions.summary}</p>
      </div>

      {/* The guess comes from the browser, which is often not the machine
          running MGA, so switching has to be one click away. */}
      <div className="flex flex-wrap gap-1.5">
        {MOUNT_PLATFORMS.map((option) => (
          <button
            key={option.id}
            type="button"
            onClick={() => setPlatform(option.id)}
            className={`rounded-md border px-2.5 py-1 text-xs transition ${
              option.id === platform
                ? 'border-mga-accent/50 bg-mga-accent/10 text-mga-text'
                : 'border-mga-border text-mga-muted hover:text-mga-text'
            }`}
          >
            {option.label}
          </button>
        ))}
      </div>

      <p className="text-xs font-medium text-mga-text">{instructions.heading}</p>

      {instructions.download && (
        <a
          href={instructions.download.url}
          target="_blank"
          rel="noreferrer noopener"
          className="inline-flex items-center gap-1.5 text-xs text-mga-accent hover:underline"
        >
          {instructions.download.label}
          <ExternalLink className="h-3 w-3" />
        </a>
      )}

      {instructions.script && (
        <div className="relative">
          <pre className="max-h-56 overflow-auto rounded-md border border-mga-border bg-mga-bg/60 p-3 text-[0.68rem] leading-5 text-mga-muted">
            <code>{instructions.script}</code>
          </pre>
          <button
            type="button"
            onClick={() => void copyScript()}
            className="absolute right-2 top-2 inline-flex items-center gap-1 rounded border border-mga-border bg-mga-surface px-2 py-1 text-[0.68rem] text-mga-muted transition hover:text-mga-text"
          >
            {copied ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
            {copied ? 'Copied' : 'Copy'}
          </button>
        </div>
      )}

      <p className="text-xs text-mga-muted">
        Once it is mounted, close this and browse again — or type the path straight into Base Path.
      </p>
    </div>
  )
}
