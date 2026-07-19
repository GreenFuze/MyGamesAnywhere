import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ArrowLeft,
  ArrowRightLeft,
  Database,
  Download,
  ExternalLink,
  FileText,
  FolderOpen,
  Loader2,
  Monitor,
  MoreHorizontal,
  PlayCircle,
	Save,
  Trophy,
  Trash2,
  Video,
} from 'lucide-react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import {
  ApiError,
	cleanupFailedGameOnDevice,
  clearSourceGameCanonicalPin,
  getGame,
  getGameAchievements,
	getFrontendConfig,
	getEndpointInstallPreference,
  installArchiveOnDevice,
  installGogInnoOnDevice,
	preflightInstallationOnDevice,
	ignoreFailedGameOnDevice,
	launchGameOnDevice,
	launchEmulatorGameOnDevice,
	claimScummVMSaveDomain,
	releaseSaveDomain,
	reconcileSaveDomain,
	restoreSaveDomain,
	snapshotSaveDomain,
  listDeviceCommands,
  mergeSourceGameCanonical,
  refreshGameMetadata,
	reopenFailedGameCleanup,
  searchCanonicalGames,
  splitSourceGameCanonical,
	setDeviceGameLaunchTarget,
  uninstallGameFromDevice,
	useExistingInstallationOnDevice,
  type AchievementDTO,
  type AchievementSetDTO,
  type CanonicalGameSearchResult,
  type CanonicalGroupingResponse,
  type DeleteSourceGameResponse,
  type DeviceCommand,
  type ExternalIDDTO,
  type GameLaunchOptionDTO,
	type InstallationPreflightResult,
  type GameMediaDetailDTO,
  type ResolverMatchDTO,
  type SourceGameDetailDTO,
} from '@/api/client'
import { useGameFavoriteAction } from '@/hooks/useGameFavorite'
import { useRecentPlayed } from '@/hooks/useRecentPlayed'
import { useProfiles } from '@/hooks/useProfiles'
import { AchievementProgressRing } from '@/components/library/AchievementProgressRing'
import { SourceGameHardDeleteDialog } from '@/components/library/SourceGameHardDeleteDialog'
import { BrandBadge, BrandIcon } from '@/components/ui/brand-icon'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Dialog } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { PlatformIcon } from '@/components/ui/platform-icon'
import { SourceFileInventory } from '@/components/ui/source-file-inventory'
import {
  brandLabel,
  resolveBrandDefinition,
  resolveBrandDefinitionFromUrl,
} from '@/lib/brands'
import {
  clearBrowserPlaySourcePreference,
  browserPlaySourceContext,
  browserPlaySourceOptionLabel,
  getBrowserPlayPreferenceRuntime,
  readBrowserPlaySourcePreference,
  resolveBrowserPlaySelection,
  writeBrowserPlaySourcePreference,
} from '@/lib/browserPlay'
import { inferOriginLabel, readGameRouteState } from '@/lib/gameNavigation'
import {
  hasBrowserPlaySupport,
  pluginLabel,
  platformLabel,
  selectSourcePlugins,
  sourceGameIntegrationLabel,
} from '@/lib/gameUtils'
import { GameMediaCollection, mediaOriginalUrl, mediaUrl, youtubeEmbedUrl, youtubeThumbnailUrl } from '@/lib/gameMedia'
import { buildFeaturedMediaRail, mergeDisplayMedia } from '@/lib/gameMediaDisplay'
import { evaluateBackgroundSuitability } from '@/lib/backgroundSuitability'
import { cn } from '@/lib/utils'
import { collectSaveDomains, saveDomainStatusLabel } from '@/lib/saveDomains'

type MetadataField =
  | 'title'
  | 'description'
  | 'release_date'
  | 'developer'
  | 'publisher'
  | 'genres'
  | 'rating'
  | 'max_players'

type ExternalLinkItem = {
  id: string
  label: string
  url: string
  subtitle: string
  source: string
  host: string
  actionLabel: string
  brandId?: string
}

function hasTextValue(value: string | undefined): boolean {
  return typeof value === 'string' && value.trim().length > 0
}

function hasNumberValue(value: number | undefined): boolean {
  return typeof value === 'number' && Number.isFinite(value) && value > 0
}

function formatDateValue(value: string | undefined): string {
  if (!value) return 'Unknown'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium' }).format(date)
}

function formatDateTimeValue(value: string | undefined): string {
  if (!value) return 'Unknown'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}

function formatHours(value: number | undefined): string {
  if (value === undefined || value <= 0) return 'Unknown'
  return `${Math.round(value)}h`
}

function formatStorageBytes(value: number | undefined): string {
  if (!value || value <= 0) return 'Unknown'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  return `${(value / 1024 ** index).toFixed(index >= 3 ? 1 : 0)} ${units[index]}`
}

function deviceSetupLabel(status: string): string {
  switch (status) {
    case 'installed':
      return 'Installed'
    case 'missing':
      return 'Missing'
    case 'needs_repair':
      return 'Needs repair'
    case 'ready_for_setup':
      return 'Ready for setup'
		case 'ready_to_play':
			return 'Ready to play'
		case 'needs_setup':
			return 'Emulator setup needed'
    case 'not_scanned':
      return 'Scan needed'
    case 'update_required':
      return 'Client update needed'
    case 'offline':
      return 'Offline'
    case 'unsupported':
      return 'Not supported yet'
    default:
      return 'Checking availability'
  }
}

type DeviceInstallChoice = {
  deviceId: string
  deviceName: string
  sourceGameId: string
  sourceLabel: string
  packageName: string
  installKind: 'managed_archive' | 'gog_inno'
}

function DeviceInstallDialog({
  gameId,
  choice,
  busy,
  error,
  onClose,
  onInstall,
}: {
  gameId: string
  choice: DeviceInstallChoice | null
  busy: boolean
  error: string
  onClose: () => void
  onInstall: (root: string) => void
}) {
	const { currentProfile } = useProfiles()
  const [root, setRoot] = useState('%USERPROFILE%\\Games')
	const [checkedRoot, setCheckedRoot] = useState('')
	const preference = useQuery({
		queryKey: ['endpoint-install-preference', choice?.deviceId, currentProfile?.id],
		queryFn: () => getEndpointInstallPreference(choice!.deviceId),
		enabled: Boolean(choice),
	})

  useEffect(() => {
    if (choice) {
		setRoot('%USERPROFILE%\\Games')
		setCheckedRoot('')
	}
  }, [choice])
	useEffect(() => {
		if (choice && preference.data) setRoot(preference.data.effective_root)
	}, [choice, preference.data])
	useEffect(() => {
		if (!choice || preference.isLoading || !root.trim()) {
			setCheckedRoot('')
			return
		}
		const timer = window.setTimeout(() => setCheckedRoot(root.trim()), 450)
		return () => window.clearTimeout(timer)
	}, [choice, preference.isLoading, root])
	const preflight = useQuery({
		queryKey: ['installation-preflight', choice?.deviceId, gameId, choice?.sourceGameId, choice?.installKind, checkedRoot],
		queryFn: async ({ signal }) => {
			const command = await preflightInstallationOnDevice(choice!.deviceId, gameId, choice!.sourceGameId, choice!.installKind, checkedRoot)
			for (let attempt = 0; attempt < 60; attempt += 1) {
				if (signal.aborted) throw new Error('Device check canceled.')
				await new Promise<void>((resolve, reject) => {
					const timer = window.setTimeout(resolve, 500)
					signal.addEventListener('abort', () => { window.clearTimeout(timer); reject(new Error('Device check canceled.')) }, { once: true })
				})
				const current = (await listDeviceCommands(choice!.deviceId)).find((item) => item.id === command.id)
				if (!current || !['succeeded', 'failed', 'rejected', 'canceled', 'expired'].includes(current.status)) continue
				if (current.status !== 'succeeded') throw new Error(current.error_message || 'MGA Client could not check this device.')
				const result = current.result as InstallationPreflightResult | undefined
				if (!result || !Array.isArray(result.checks)) throw new Error('MGA Client returned an invalid device check.')
				return result
			}
			throw new Error('The device check took too long. Make sure MGA Client is connected.')
		},
		enabled: Boolean(choice && checkedRoot && !preference.isLoading),
		retry: false,
		staleTime: 30_000,
		refetchOnWindowFocus: false,
		refetchOnReconnect: false,
	})
	const checking = Boolean(choice && root.trim() && (root.trim() !== checkedRoot || preflight.isLoading || preflight.isFetching))
	const blocked = preflight.data?.can_install === false

  return (
    <Dialog open={choice !== null} onClose={busy ? () => undefined : onClose} title="Install on device">
      {choice ? (
        <div className="space-y-4">
          <div className="rounded-mga border border-mga-border bg-mga-bg/60 p-3 text-sm">
            <p className="font-semibold text-mga-text">{choice.deviceName}</p>
            <p className="mt-1 text-xs text-mga-muted">{choice.sourceLabel} · {choice.packageName}</p>
          </div>
          <Input
            label="Install folder"
            value={root}
            onChange={(event) => setRoot(event.target.value)}
            disabled={busy || preference.isLoading}
            placeholder="%USERPROFILE%\\Games"
          />
          <p className="text-xs leading-5 text-mga-muted">
            {choice.installKind === 'gog_inno'
              ? `MGA Client on ${choice.deviceName} will verify the GOG publisher and installer type before it starts. Windows may ask for permission on that device.`
              : 'Environment variables are expanded by MGA Client for the selected Windows user. This installation can override your future profile default.'}
          </p>
		  {preference.data ? (
			<p className="text-xs text-mga-muted">
			  Starting with {preference.data.source === 'device' ? `${choice.deviceName}'s folder` : 'your default folder'}. You can change it for this installation.
			</p>
		  ) : null}
          {preference.error ? <p className="text-xs text-amber-300">Saved folder could not be loaded. Check the folder before installing.</p> : null}
          <section className="rounded-mga border border-mga-border bg-black/20 p-3">
			<div className="flex items-center justify-between gap-3">
			  <p className="text-xs font-bold uppercase tracking-[0.16em] text-mga-muted">Before installing</p>
			  {checking ? <span className="flex items-center gap-1.5 text-xs text-mga-muted"><Loader2 size={13} className="animate-spin" /> Checking device</span> : null}
			</div>
			{preflight.data ? (
			  <div className="mt-2 space-y-2">
				{preflight.data.checks.map((check) => (
				  <div key={check.id} className="flex gap-2 text-xs leading-5">
					<span className={cn('mt-1.5 h-2 w-2 shrink-0 rounded-full', check.status === 'ready' ? 'bg-emerald-400' : check.status === 'missing' ? 'bg-red-400' : check.status === 'installer_managed' ? 'bg-purple-400' : 'bg-amber-400')} />
					<div><span className="font-semibold text-mga-text">{check.name}.</span> <span className="text-mga-muted">{check.message}</span>{check.required_bytes ? <span className="ml-1 text-mga-muted">Package: {formatStorageBytes(check.required_bytes)}{check.available_bytes ? ` · Free: ${formatStorageBytes(check.available_bytes)}` : ''}.</span> : null}</div>
				  </div>
				))}
			  </div>
			) : preflight.error && checkedRoot === root.trim() ? (
			  <p className="mt-2 text-xs leading-5 text-amber-300">MGA couldn’t check this device. You can still install, but MGA has not verified space or extra components.</p>
			) : null}
		  </section>
          {error ? <p className="text-sm text-red-400">{error}</p> : null}
          <div className="flex justify-end gap-2">
            <Button variant="outline" onClick={onClose} disabled={busy}>Cancel</Button>
            <Button onClick={() => onInstall(root.trim())} disabled={busy || preference.isLoading || checking || blocked || !root.trim()}>
              {busy ? <Loader2 size={16} className="animate-spin" /> : <Download size={16} />}
              {busy ? 'Starting…' : preflight.error ? 'Install anyway' : blocked ? 'Requirements missing' : 'Install'}
            </Button>
          </div>
        </div>
      ) : null}
    </Dialog>
  )
}

function fileBasename(path: string): string {
  return path.split(/[\\/]/).pop() || path
}

function escapeRegExp(value: string): string {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}

function gogInnoPackage(source: SourceGameDetailDTO): { installerName: string; companionCount: number } | null {
  if (source.kind.toLowerCase() !== 'base_game') return null

  const executableFiles = source.files.filter((file) => /\.exe$/i.test(fileBasename(file.path)))
  const installers = executableFiles.filter((file) => /^setup_.+\.exe$/i.test(fileBasename(file.path)))
  if (executableFiles.length !== 1 || installers.length !== 1) return null

  const installerName = fileBasename(installers[0].path)
  const setupStem = installerName.replace(/\.exe$/i, '')
  const companionPattern = new RegExp(`^${escapeRegExp(setupStem)}-\\d+\\.bin$`, 'i')
  const binaryFiles = source.files.filter((file) => /\.bin$/i.test(fileBasename(file.path)))
  if (binaryFiles.some((file) => !companionPattern.test(fileBasename(file.path)))) return null
  const companionCount = binaryFiles.length
  return { installerName, companionCount }
}

function commandProgressMessage(command: DeviceCommand): string {
  if (command.name !== 'game.install_gog_inno') {
    return command.progress_message || humanizeValue(command.status)
  }

  const progress = `${command.progress_phase ?? ''} ${command.progress_message ?? ''}`.toLowerCase()
  if (/windows permission|uac|elevat/.test(progress)) return 'Waiting for Windows permission'
  if (/installer.?running|running installer|process.?running/.test(progress)) return 'Installer running'
  if (/verif|signature|publisher/.test(progress)) return 'Verifying publisher'
  if (/download|transfer/.test(progress)) return 'Downloading installer'
  if (/checking|validat|finaliz|launch target/.test(progress)) return 'Checking installed game'
  if (/prepar|staging/.test(progress)) return 'Preparing installer'
  if (command.status === 'succeeded') return 'Installed'
  if (['failed', 'rejected', 'canceled', 'expired'].includes(command.status)) return "Couldn't install"
  return command.progress_message || 'Preparing installer'
}

function extractInstallerExitCode(message: string | undefined): string | null {
  if (!message) return null
  const hex = message.match(/0x[0-9a-fA-F]{8}/)
  if (hex) return hex[0].toUpperCase()
  const decimal = message.match(/exited with code (-?\d+)/i)
  if (!decimal) return null
  const value = Number(decimal[1])
  if (!Number.isFinite(value)) return decimal[1]
  return `0x${(value >>> 0).toString(16).toUpperCase().padStart(8, '0')}`
}

function gogInstallErrorMessage(code: string | undefined, message: string | undefined, deviceName: string): string {
  const detail = `${code ?? ''} ${message ?? ''}`.toLowerCase()
  const exitCode = extractInstallerExitCode(message)
  if (/invalid_installer_signature|signature|publisher/.test(detail)) {
    return "This installer isn’t a verified GOG package. MGA won’t run it."
  }
  if (/unsupported_installer|invalid_companion_set|unsupported package/.test(detail)) {
    return "This installer isn’t supported yet (wrong shape, missing parts, or not Inno Setup)."
  }
  if (/local_confirmation_declined/.test(detail)) {
    return `The action was canceled on ${deviceName}.`
  }
  if (/local_confirmation_timeout/.test(detail)) {
    return `Confirmation timed out on ${deviceName}. Try again when you can confirm it there.`
  }
  if (/cleanup_marker_missing|cleanup_marker_mismatch|cleanup_boundary_failed/.test(detail)) {
    return `MGA couldn’t verify that the failed game folder is safe to remove on ${deviceName}. Files were preserved.`
  }
  if (/cleanup_uninstaller_failed|cleanup_failed/.test(detail)) {
    return `Cleanup couldn’t finish on ${deviceName}. Files were preserved.`
  }
  if (/uac_declined/.test(detail)) {
    return `Windows permission wasn’t approved on ${deviceName}.`
  }
  if (/installer_timeout/.test(detail)) {
    return `The installer may still be running on ${deviceName}. Check the device before trying again.`
  }
  if (/installer_start_failed/.test(detail)) {
    return `MGA couldn’t start the installer on ${deviceName}.`
  }
  if (/installer_exit_nonzero/.test(detail)) {
    return exitCode
      ? `The installer on ${deviceName} finished with an error (${exitCode}). Files may be incomplete — check that device before trying again.`
      : `The installer on ${deviceName} finished with an error. Files may be incomplete — check that device before trying again.`
  }
  if (/install_validation_failed|uninstaller_missing/.test(detail)) {
    return `The installer ran on ${deviceName}, but MGA couldn’t confirm a complete install (missing game folder or uninstaller). Check the device before trying again.`
  }
  if (/uninstaller_mismatch/.test(detail)) {
    return `MGA couldn’t verify this game’s uninstaller on ${deviceName}.`
  }
  if (/uninstaller_exit_nonzero/.test(detail)) {
    return exitCode
      ? `The game’s uninstaller couldn’t finish on ${deviceName} (${exitCode}). Saves and settings may remain.`
      : `The game’s uninstaller couldn’t finish on ${deviceName}. Saves and settings may remain.`
  }
  return message || "MGA couldn’t install this game."
}

function apiErrorDetail(error: unknown): { code?: string; message?: string } {
  if (error instanceof ApiError && error.responseText) {
    try {
      const parsed = JSON.parse(error.responseText) as { error?: string; error_code?: string; message?: string }
      return { code: parsed.error_code || parsed.error, message: parsed.message }
    } catch {
      return { message: error.responseText }
    }
  }
  return { message: error instanceof Error ? error.message : undefined }
}

function attentionRequiredMessage(reason: string | undefined, deviceName: string): string {
  const detail = (reason ?? '').toLowerCase()
  const exitCode = extractInstallerExitCode(reason)
  if (/timeout|still_running|unknown.*outcome|connection|restart/.test(detail)) {
    return `The installer may still be running on ${deviceName}. Check the device before trying again.`
  }
  if (/invalid_installer_signature|signature|publisher/.test(detail)) {
    return `This installer wasn’t a verified GOG package on ${deviceName}. MGA did not treat it as installed.`
  }
  if (/unsupported_installer|invalid_companion_set/.test(detail)) {
    return `This installer isn’t supported on ${deviceName}. MGA did not treat it as installed.`
  }
  if (/installer_exit_nonzero/.test(detail)) {
    return exitCode
      ? `The installer on ${deviceName} reported an error (${exitCode}). Files may be incomplete — check the device before playing or trying again.`
      : `The installer on ${deviceName} reported an error. Files may be incomplete — check the device before playing or trying again.`
  }
  if (/install_validation_failed|uninstaller_missing/.test(detail)) {
    return `Install on ${deviceName} didn’t look complete (missing game folder or uninstaller). Check the device before playing or trying again.`
  }
  if (/uac_declined/.test(detail)) {
	return `Installation didn’t finish on ${deviceName} because Windows permission was declined.`
  }
  if (reason?.trim()) {
    return `This installation needs attention on ${deviceName}: ${reason.trim()}`
  }
  return `This installation needs attention on ${deviceName}. Check the device before playing or trying again.`
}

function installationVerificationMessage(reason: string | undefined): string {
  switch ((reason ?? '').trim()) {
    case 'manifest_missing': return 'MGA’s installation record is missing from the game folder.'
    case 'manifest_invalid':
    case 'manifest_identity_mismatch':
    case 'manifest_schema_unsupported': return 'MGA could not verify the game folder.'
    case 'launch_target_missing': return 'the executable used to start the game is missing.'
    case 'uninstall_target_missing': return 'the game’s uninstaller is missing.'
    case 'registered_program_missing': return 'Windows no longer lists the game as installed.'
    case 'files_missing_registration_present': return 'Windows lists the game, but its files are missing.'
    case 'unsafe_reparse_point': return 'the game folder redirects somewhere MGA cannot verify safely.'
    default: return reason?.trim() || 'MGA could not verify all required files.'
  }
}

function humanizeValue(value: string): string {
  return value
    .split(/[_-]+/g)
    .filter(Boolean)
    .map((part) => {
      if (part.length <= 3) return part.toUpperCase()
      return part.charAt(0).toUpperCase() + part.slice(1)
    })
    .join(' ')
}

function formatHostname(url: string): string {
  try {
    return new URL(url).hostname.toLowerCase().replace(/^www\./, '')
  } catch {
    return 'external link'
  }
}

function mediaTypeLabel(type: string): string {
  if (!type) return 'Media'
  return type
    .split(/[_-]/g)
    .filter(Boolean)
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

function splitHeroDescription(value: string | undefined): { tagline: string | null; body: string | null } {
  if (!value) return { tagline: null, body: null }
  const normalized = value.replace(/\s+/g, ' ').trim()
  if (!normalized) return { tagline: null, body: null }
  const parts = normalized.split(/(?<=[.!?])\s+/)
  if (parts.length < 2 || parts[0].length > 96) {
    return { tagline: null, body: normalized }
  }
  return {
    tagline: parts[0].trim(),
    body: parts.slice(1).join(' ').trim() || parts[0].trim(),
  }
}

function buildExternalLinks(externalIds: ExternalIDDTO[] | undefined): ExternalLinkItem[] {
  if (!externalIds) return []

  const links = externalIds
    .filter((externalId) => typeof externalId.url === 'string' && externalId.url.length > 0)
    .map((externalId) => {
      const brand =
        resolveBrandDefinition(externalId.source) ?? resolveBrandDefinitionFromUrl(externalId.url!)
      const label = brand?.label ?? brandLabel(externalId.source, formatHostname(externalId.url!))
      return {
        id: `${externalId.source}:${externalId.external_id}`,
        label,
        url: externalId.url!,
        subtitle: externalId.external_id,
        source: externalId.source,
        host: formatHostname(externalId.url!),
        actionLabel: brand ? `Open in ${brand.label}` : 'Open external link',
        brandId: brand?.id,
      }
    })
    .sort((a, b) => a.label.localeCompare(b.label) || a.host.localeCompare(b.host))

  return Array.from(new Map(links.map((link) => [link.id, link])).values())
}

function detailValue(value: string | number | undefined | null): string {
  if (value === null || value === undefined || value === '') return 'Unknown'
  return String(value)
}

function identityStatusLabel(state: string | undefined): string {
  switch (state) {
    case 'provider_confirmed':
      return 'Matched'
    case 'manual':
      return 'Grouped by you'
    case 'legacy_review':
      return 'Needs a quick review'
    case 'unresolved':
      return 'Kept separate for now'
    default:
      return 'Not checked yet'
  }
}

function editionLabel(
  platform: string,
  region?: string,
  edition?: string,
): string {
  const parts = [
    platform && platform !== 'unknown' ? platformLabel(platform) : '',
    region?.trim(),
    edition?.trim(),
  ].filter((part): part is string => Boolean(part))
  return parts.length > 0 ? Array.from(new Set(parts)).join(' · ') : 'Version not identified yet'
}

function isMetadataPlugin(pluginId: string): boolean {
  return pluginId.startsWith('metadata-')
}

function resolverHasField(match: ResolverMatchDTO, field: MetadataField): boolean {
  switch (field) {
    case 'title':
      return hasTextValue(match.title)
    case 'description':
      return hasTextValue(match.description)
    case 'release_date':
      return hasTextValue(match.release_date)
    case 'developer':
      return hasTextValue(match.developer)
    case 'publisher':
      return hasTextValue(match.publisher)
    case 'genres':
      return Array.isArray(match.genres) && match.genres.length > 0
    case 'rating':
      return hasNumberValue(match.rating)
    case 'max_players':
      return hasNumberValue(match.max_players)
  }
}

function collectMetadataAttributions(
  sourceGames: SourceGameDetailDTO[],
  field: MetadataField,
): string[] {
  const matches = sourceGames
    .flatMap((sourceGame) => sourceGame.resolver_matches)
    .filter((match) => !match.outvoted && resolverHasField(match, field))

  const metadataMatches = matches.filter(
    (match) => isMetadataPlugin(match.plugin_id) && resolverHasField(match, field),
  )
  const relevant = metadataMatches.length > 0 ? metadataMatches : matches

  return Array.from(new Set(relevant.map((match) => match.plugin_id)))
}

function collectUnifiedMetadataSources(
  sourceGames: SourceGameDetailDTO[],
  media: GameMediaDetailDTO[] | undefined,
): string[] {
  const fieldSources = [
    ...collectMetadataAttributions(sourceGames, 'title'),
    ...collectMetadataAttributions(sourceGames, 'description'),
    ...collectMetadataAttributions(sourceGames, 'release_date'),
    ...collectMetadataAttributions(sourceGames, 'developer'),
    ...collectMetadataAttributions(sourceGames, 'publisher'),
    ...collectMetadataAttributions(sourceGames, 'genres'),
    ...collectMetadataAttributions(sourceGames, 'rating'),
    ...collectMetadataAttributions(sourceGames, 'max_players'),
  ]
  const mediaSources = (media ?? [])
    .map((item) => item.source?.trim())
    .filter((item): item is string => Boolean(item))

  return Array.from(new Set([...fieldSources, ...mediaSources])).sort((a, b) => pluginLabel(a).localeCompare(pluginLabel(b)))
}

function sortAchievementSet(set: AchievementSetDTO): AchievementSetDTO {
  return {
    ...set,
    achievements: [...set.achievements].sort((a, b) => {
      if (a.unlocked !== b.unlocked) return a.unlocked ? -1 : 1
      if (a.unlocked_at && b.unlocked_at) {
        return new Date(b.unlocked_at).getTime() - new Date(a.unlocked_at).getTime()
      }
      return a.title.localeCompare(b.title)
    }),
  }
}

function achievementSetTitle(set: AchievementSetDTO): string {
  const source = pluginLabel(set.source)
  const platform = set.platform && set.platform !== 'unknown' ? platformLabel(set.platform) : ''
  return platform ? `${source} • ${platform}` : source
}

function achievementVersionLabel(set: AchievementSetDTO): string {
  if (set.platform === 'arcade') return 'MAME'
  if (set.platform && set.platform !== 'unknown') return platformLabel(set.platform)
  return set.source_title?.trim() || 'Version'
}

function achievementSetContext(set: AchievementSetDTO): string {
  const parts = [
    set.source_title?.trim(),
    set.external_game_id ? `${pluginLabel(set.source)} #${set.external_game_id}` : '',
    set.integration_label?.trim(),
  ].filter((part): part is string => Boolean(part))
  return Array.from(new Set(parts)).join(' · ')
}

function launchOptionContext(option: GameLaunchOptionDTO): string {
  const parts = [
    option.integration_label?.trim(),
    option.platform && option.platform !== 'unknown' ? platformLabel(option.platform) : '',
    option.source_title?.trim(),
  ].filter((part): part is string => Boolean(part))
  return Array.from(new Set(parts)).join(' · ')
}

function launchOptionKey(option: GameLaunchOptionDTO): string {
  return `${option.kind}:${option.source_game_id}:${option.url ?? option.file_id ?? option.root_file_id ?? ''}`
}

function SectionCard({
  id,
  title,
  icon,
  description,
  actions,
  className,
  children,
}: {
  id?: string
  title: string
  icon?: ReactNode
  description?: ReactNode
  actions?: ReactNode
  className?: string
  children: ReactNode
}) {
  return (
    <section
      id={id}
      className={cn(
        'scroll-mt-28 overflow-hidden rounded-[28px] border border-white/[0.05] bg-[linear-gradient(180deg,rgba(14,19,30,0.92),rgba(10,14,23,0.92))] shadow-[0_18px_42px_rgba(0,0,0,0.2)]',
        className,
      )}
    >
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-white/[0.05] px-5 py-4 md:px-6">
        <div className="space-y-1">
          <div className="flex items-center gap-2">
            {icon}
            <h2 className="text-base font-semibold text-white">{title}</h2>
          </div>
          {description ? <div className="text-sm leading-6 text-white/60">{description}</div> : null}
        </div>
        {actions ? <div className="flex shrink-0 flex-wrap gap-2">{actions}</div> : null}
      </div>
      <div className="p-5 md:p-6">{children}</div>
    </section>
  )
}

function SourceBadge({ source, className }: { source: string; className?: string }) {
  return <BrandBadge brand={source} label={brandLabel(source, pluginLabel(source))} className={className} />
}

function SourceGameBadge({ source, className }: { source: Pick<SourceGameDetailDTO, 'plugin_id' | 'integration_label'>; className?: string }) {
  return <BrandBadge brand={source.plugin_id} label={sourceGameIntegrationLabel(source)} className={className} />
}

function AttributionNote({ sources, prefix = 'Source' }: { sources?: string[] | null; prefix?: string }) {
  if (!sources || sources.length === 0) return null
  return (
    <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-white/58">
      <span className="text-white/52">{prefix}</span>
      {sources.map((source) => (
        <SourceBadge key={source} source={source} className="border-white/[0.08] bg-white/[0.04] text-white" />
      ))}
    </div>
  )
}

function MetaItem({ label, value, attributionSources, attributionPrefix }: { label: string; value: ReactNode; attributionSources?: string[] | null; attributionPrefix?: string }) {
  return (
    <div className="rounded-[20px] border border-white/[0.05] bg-[#101723] p-4 shadow-[0_10px_22px_rgba(0,0,0,0.14)]">
      <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-white/42">{label}</p>
      <div className="mt-2 text-sm font-medium text-white">{value}</div>
      <AttributionNote sources={attributionSources} prefix={attributionPrefix} />
    </div>
  )
}

function HeroStatCard({ label, value, detail }: { label: string; value: ReactNode; detail?: ReactNode }) {
  return (
    <div className="rounded-[20px] border border-white/[0.05] bg-[#101824] px-4 py-4 shadow-[0_12px_28px_rgba(0,0,0,0.16)]">
      <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-white/42">{label}</p>
      <div className="mt-2 text-lg font-semibold text-white">{value}</div>
      {detail ? <div className="mt-1.5 text-xs leading-5 text-white/58">{detail}</div> : null}
    </div>
  )
}

function HeroFactStripItem({ label, value, detail }: { label: string; value: ReactNode; detail?: ReactNode }) {
  return (
    <div className="min-w-0 px-4 py-4">
      <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-white/38">{label}</p>
      <div className="mt-2 text-lg font-semibold text-white">{value}</div>
      {detail ? <div className="mt-1 text-xs text-white/54">{detail}</div> : null}
    </div>
  )
}

function HeroTabLink({ href, label }: { href: string; label: string }) {
  return (
    <a
      href={href}
      className="inline-flex h-11 items-center justify-center rounded-full border border-white/[0.04] bg-white/[0.03] px-4 text-sm font-medium text-white/72 transition-colors hover:bg-white/[0.08] hover:text-white"
    >
      {label}
    </a>
  )
}

function mediaItemKey(media: GameMediaDetailDTO): string {
  return `${media.asset_id}:${media.type}`
}

function HeroMediaThumb({
  media,
  label,
  onSelect,
}: {
  media: GameMediaDetailDTO
  label: string
  onSelect: (media: GameMediaDetailDTO) => void
}) {
  const mediaCollection = new GameMediaCollection([media])
  const isImage = mediaCollection.isImage(media)
  const isVideo = !isImage && (Boolean(youtubeEmbedUrl(media)) || mediaCollection.isInlineVideo(media))
  const youtubeThumb = youtubeThumbnailUrl(media)

  return (
    <button
      type="button"
      onClick={() => onSelect(media)}
      className="group relative h-20 w-32 shrink-0 overflow-hidden rounded-[16px] bg-black/20 ring-1 ring-white/[0.05] transition-all duration-200 hover:-translate-y-0.5 hover:ring-white/[0.14] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/60 focus-visible:ring-offset-0"
      title={label}
      aria-label={label}
    >
      {isImage ? (
        <img
          src={mediaUrl(media)}
          alt=""
          aria-hidden="true"
          loading="lazy"
          decoding="async"
          className="h-full w-full object-contain transition-transform duration-200 group-hover:scale-[1.03]"
        />
      ) : youtubeThumb ? (
        <div className="relative h-full w-full">
          <img
            src={youtubeThumb}
            alt=""
            aria-hidden="true"
            loading="lazy"
            decoding="async"
            className="h-full w-full object-cover transition-transform duration-200 group-hover:scale-[1.03]"
          />
          <div className="absolute inset-0 bg-black/20" />
        </div>
      ) : (
        <div className="flex h-full w-full items-center justify-center bg-[radial-gradient(circle_at_top,rgba(255,255,255,0.18),transparent_55%),linear-gradient(180deg,rgba(18,17,23,0.92),rgba(10,10,14,0.98))]">
          <div className="rounded-full border-2 border-white/80 bg-black/50 p-1 text-white shadow-[0_10px_24px_rgba(0,0,0,0.34)]">
            <PlayCircle size={34} strokeWidth={1.8} fill="rgba(255,255,255,0.12)" />
          </div>
        </div>
      )}
      <div className="absolute inset-0 bg-gradient-to-t from-black/70 via-transparent to-transparent" />
      {isVideo ? (
        <div className="absolute inset-0 flex items-center justify-center">
          <div className="rounded-full border-2 border-white/90 bg-black/52 p-1 text-white shadow-[0_10px_24px_rgba(0,0,0,0.34)]">
            <PlayCircle size={34} strokeWidth={1.8} fill="rgba(255,255,255,0.12)" />
          </div>
        </div>
      ) : null}
      <div className="absolute inset-x-2 bottom-1.5 truncate text-[11px] font-medium text-white/86">{label}</div>
    </button>
  )
}

function HeroActionButton({
  children,
  primary = false,
  className,
  ...props
}: React.ComponentProps<'button'> & { primary?: boolean }) {
  return (
    <button
      {...props}
      className={cn(
        'inline-flex h-11 items-center justify-center gap-2 rounded-[15px] px-4.5 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-60',
        primary
          ? 'bg-[linear-gradient(180deg,#7379ff,#5960ef)] text-white shadow-[0_12px_24px_rgba(90,97,239,0.28)] hover:opacity-95'
          : 'border border-white/[0.08] bg-[#101620] text-white hover:bg-white/[0.06]',
        className,
      )}
    >
      {children}
    </button>
  )
}

function FavoriteActionButton({
  favorite,
  busy,
  onClick,
  className,
}: {
  favorite: boolean
  busy: boolean
  onClick: () => void
  className?: string
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={busy}
      aria-label={favorite ? 'Remove from favorites' : 'Add to favorites'}
      title={favorite ? 'Remove from favorites' : 'Add to favorites'}
      className={cn(
        'inline-flex h-11 w-11 items-center justify-center rounded-[15px] border text-lg transition-colors disabled:cursor-wait disabled:opacity-60',
        favorite
          ? 'border-rose-300/60 bg-rose-500/18 text-rose-100 hover:bg-rose-500/24'
          : 'border-white/[0.08] bg-[#101620] text-white hover:bg-white/[0.06]',
        className,
      )}
    >
      <span aria-hidden="true" className={cn('leading-none', favorite ? 'text-rose-400' : '')}>
        {favorite ? '♥' : '♡'}
      </span>
    </button>
  )
}

function HeroOverflowMenu({
  onRefresh,
  onReclassify,
  refreshBusy,
  className,
  direction = 'down',
}: {
  onRefresh: () => void
  onReclassify: () => void
  refreshBusy: boolean
  className?: string
  direction?: 'down' | 'up'
}) {
  const [open, setOpen] = useState(false)
  const buttonRef = useRef<HTMLButtonElement | null>(null)
  const menuRef = useRef<HTMLDivElement | null>(null)
  const [menuPosition, setMenuPosition] = useState<{
    top?: number
    bottom?: number
    left: number
    width: number
  } | null>(null)

  useEffect(() => {
    if (!open) return
    const updatePosition = () => {
      const button = buttonRef.current
      if (!button) return
      const rect = button.getBoundingClientRect()
      const width = Math.min(336, window.innerWidth - 32)
      const left = Math.min(Math.max(16, rect.right - width), window.innerWidth - width - 16)
      setMenuPosition(
        direction === 'down'
          ? { top: rect.bottom + 8, left, width }
          : { bottom: window.innerHeight - rect.top + 8, left, width },
      )
    }
    updatePosition()
    window.addEventListener('resize', updatePosition)
    window.addEventListener('scroll', updatePosition, true)
    return () => {
      window.removeEventListener('resize', updatePosition)
      window.removeEventListener('scroll', updatePosition, true)
    }
  }, [direction, open])

  useEffect(() => {
    if (!open) return
    const handlePointerDown = (event: MouseEvent) => {
      const target = event.target as Node
      if (!menuRef.current?.contains(target) && !buttonRef.current?.contains(target)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handlePointerDown)
    return () => document.removeEventListener('mousedown', handlePointerDown)
  }, [open])

  return (
    <div className={cn('relative', className)}>
      <button
        ref={buttonRef}
        type="button"
        onClick={() => setOpen((current) => !current)}
        aria-haspopup="menu"
        aria-expanded={open}
        className="inline-flex h-11 w-11 items-center justify-center rounded-[15px] border border-white/[0.08] bg-[#101620] text-white transition-colors hover:bg-white/[0.06]"
      >
        <MoreHorizontal size={18} />
      </button>
      {open && menuPosition
        ? createPortal(
        <div
          ref={menuRef}
          className={cn(
            'fixed z-[1000] overflow-hidden rounded-[18px] border border-white/[0.08] bg-[#0f1520] p-2 shadow-[0_18px_44px_rgba(0,0,0,0.34)] backdrop-blur-xl',
          )}
          style={menuPosition}
        >
          <button
            type="button"
            onClick={() => {
              setOpen(false)
              onRefresh()
            }}
            disabled={refreshBusy}
            className="flex w-full items-center gap-2 rounded-[12px] px-3 py-2.5 text-left text-sm text-white transition-colors hover:bg-white/[0.06] disabled:cursor-not-allowed disabled:opacity-60"
          >
            <Database size={16} />
            {refreshBusy ? 'Refreshing...' : 'Refresh Metadata and Achievements'}
          </button>
          <button
            type="button"
            onClick={() => {
              setOpen(false)
              onReclassify()
            }}
            className="flex w-full items-center gap-2 rounded-[12px] px-3 py-2.5 text-left text-sm text-white transition-colors hover:bg-white/[0.06]"
          >
            <ArrowRightLeft size={16} />
            Reclassify
          </button>
        </div>,
        document.body,
      )
        : null}
    </div>
  )
}

function AchievementPreviewCard({ achievement }: { achievement: AchievementDTO }) {
  const iconUrl = achievement.unlocked ? achievement.unlocked_icon : achievement.locked_icon

  return (
    <article className="overflow-hidden rounded-[18px] border border-white/10 bg-[rgba(28,26,34,0.92)] shadow-[0_12px_24px_rgba(0,0,0,0.18)]">
      <div className="relative aspect-[4/3] overflow-hidden bg-[radial-gradient(circle_at_top,rgba(255,255,255,0.16),transparent_52%),linear-gradient(180deg,rgba(34,44,40,0.96),rgba(22,24,30,0.98))]">
        {iconUrl ? (
          <img
            src={iconUrl}
            alt=""
            aria-hidden="true"
            loading="lazy"
            decoding="async"
            className={cn('h-full w-full object-cover', achievement.unlocked ? '' : 'opacity-55 grayscale')}
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-white/72">
            <Trophy size={24} />
          </div>
        )}
        {!achievement.unlocked ? (
          <div className="absolute inset-0 flex items-center justify-center">
            <div className="rounded-full bg-black/55 p-2.5 text-white/88">
              <Trophy size={18} />
            </div>
          </div>
        ) : null}
      </div>
      <div className="space-y-2 p-3">
        <div className="flex flex-wrap items-center gap-2">
          {achievement.unlocked ? <Badge variant="accent">Unlocked</Badge> : <Badge variant="muted">Locked</Badge>}
          {achievement.points !== undefined && achievement.points > 0 ? <Badge>{achievement.points} pts</Badge> : null}
          {achievement.rarity !== undefined && achievement.rarity > 0 ? <Badge>{achievement.rarity.toFixed(1)}%</Badge> : null}
        </div>
        <p className="line-clamp-2 text-base font-semibold leading-5 text-mga-text">{achievement.title}</p>
        <p className="line-clamp-2 text-xs leading-5 text-mga-muted">
          {achievement.description || (achievement.unlocked ? 'Unlocked achievement.' : 'Achievement details unavailable.')}
        </p>
      </div>
    </article>
  )
}

function sourceHasBrowserPlayDelivery(source: SourceGameDetailDTO): boolean {
  return (
    source.delivery?.profiles?.some((profile) => profile.mode === 'direct' || profile.mode === 'materialized') ??
    false
  )
}

function AchievementRow({ achievement }: { achievement: AchievementDTO }) {
  const iconUrl = achievement.unlocked ? achievement.unlocked_icon : achievement.locked_icon

  return (
    <div className="flex items-start gap-3 rounded-mga border border-mga-border bg-mga-bg/60 p-3">
      <div className="h-12 w-12 shrink-0 overflow-hidden rounded-mga border border-mga-border bg-mga-surface">
        {iconUrl ? (
          <img
            src={iconUrl}
            alt=""
            loading="lazy"
            decoding="async"
            className={cn('h-full w-full object-cover', achievement.unlocked ? '' : 'opacity-70 grayscale')}
          />
        ) : (
          <div className="flex h-full w-full items-center justify-center text-mga-muted">
            <Trophy size={18} />
          </div>
        )}
      </div>
      <div className="min-w-0 flex-1 space-y-1">
        <div className="flex flex-wrap items-center gap-2">
          <p className="font-medium text-mga-text">{achievement.title}</p>
          {achievement.unlocked ? <Badge variant="accent">Unlocked</Badge> : <Badge variant="muted">Locked</Badge>}
          {achievement.points !== undefined && achievement.points > 0 && <Badge>{achievement.points} pts</Badge>}
          {achievement.rarity !== undefined && achievement.rarity > 0 && <Badge>{achievement.rarity.toFixed(1)}%</Badge>}
        </div>
        {achievement.description && <p className="text-sm leading-6 text-mga-muted">{achievement.description}</p>}
        {achievement.unlocked_at && <p className="text-xs text-mga-muted">Unlocked {formatDateTimeValue(achievement.unlocked_at)}</p>}
      </div>
    </div>
  )
}

function summarizeResolverMatch(match: ResolverMatchDTO): string[] {
  const facts: string[] = []
  if (match.release_date) facts.push(formatDateValue(match.release_date))
  if (match.developer) facts.push(match.developer)
  if (match.publisher) facts.push(match.publisher)
  if (match.genres && match.genres.length > 0) facts.push(match.genres.join(', '))
  if (match.rating && match.rating > 0) facts.push(`Rating ${match.rating.toFixed(1)}`)
  if (match.max_players && match.max_players > 0) facts.push(`${match.max_players} players`)
  if (match.xcloud_available) facts.push('xCloud ready')
  if (match.is_game_pass) facts.push('Game Pass')
  return facts
}

function ResolverMatchRow({ match }: { match: ResolverMatchDTO }) {
  const facts = summarizeResolverMatch(match)

  return (
    <div className="rounded-mga border border-mga-border bg-mga-bg/60 p-3 shadow-sm shadow-black/5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <SourceBadge source={match.plugin_id} />
            {match.outvoted ? <Badge variant="muted">Outvoted</Badge> : <Badge variant="accent">Active</Badge>}
            {match.xcloud_available ? <BrandBadge brand="xcloud" label="xCloud" /> : null}
            {match.is_game_pass ? <Badge variant="gamepass">Game Pass</Badge> : null}
          </div>
          <p className="text-sm font-semibold text-mga-text">{match.title ?? 'Unknown title'}</p>
        </div>
        {match.url ? (
          <a href={match.url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-xs font-medium text-mga-accent hover:underline">
            Open match
            <ExternalLink size={12} />
          </a>
        ) : null}
      </div>
      <div className="mt-3 grid gap-2 text-sm md:grid-cols-2">
        <MetaItem label="External ID" value={match.external_id} />
        <MetaItem label="Platform" value={match.platform ? humanizeValue(match.platform) : 'Unknown'} />
      </div>
      {facts.length > 0 && <p className="mt-3 text-xs leading-5 text-mga-muted">{facts.join(' • ')}</p>}
    </div>
  )
}

function sourceRecordLabel(source: SourceGameDetailDTO): string {
  return `${source.integration_label || source.integration_id} · ${source.raw_title || source.external_id}`
}

function SourceRecordCard({
  source,
  canonicalGameId,
  onHardDelete,
  onSplit,
  onMerge,
  onClearPin,
}: {
  source: SourceGameDetailDTO
  canonicalGameId: string
  onHardDelete: (source: SourceGameDetailDTO) => void
  onSplit: (source: SourceGameDetailDTO) => void
  onMerge: (source: SourceGameDetailDTO) => void
  onClearPin: (source: SourceGameDetailDTO) => void
}) {
  const browserPlayable = sourceHasBrowserPlayDelivery(source)
  const hardDeleteEligible = source.hard_delete?.eligible ?? false
  const pin = source.canonical_pin

  return (
    <article className="space-y-4 rounded-mga border border-mga-border bg-mga-bg/60 p-4 shadow-sm shadow-black/5">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-2">
          <div className="flex flex-wrap items-center gap-2">
            <SourceGameBadge source={source} />
            <Badge variant="source">{source.status}</Badge>
            <Badge variant="platform"><PlatformIcon platform={source.platform} showLabel /></Badge>
            {browserPlayable ? <Badge variant="accent">Browser Play</Badge> : null}
            {pin ? <Badge className="bg-amber-500/20 text-amber-100">{pin.mode === 'split' ? 'Manual split' : 'Manual merge'}</Badge> : null}
          </div>
          <div>
            <p className="text-sm font-semibold text-mga-text">{source.raw_title || source.external_id}</p>
            <p className="mt-1 text-xs text-mga-muted">{source.external_id}</p>
          </div>
        </div>

        {source.url && (
          <a href={source.url} target="_blank" rel="noreferrer" className="inline-flex items-center gap-2 rounded-mga border border-mga-border bg-mga-surface px-3 py-1.5 text-xs font-medium text-mga-text transition-colors hover:bg-mga-elevated">
            <BrandIcon brand={source.plugin_id} />
            Open Source
            <ExternalLink size={14} />
          </a>
        )}
      </div>

      <div className="grid gap-3 text-sm md:grid-cols-2 xl:grid-cols-3">
        <MetaItem label="Integration" value={source.integration_label || source.integration_id} />
        <MetaItem label="Kind" value={source.kind} />
        <MetaItem label="Created" value={formatDateTimeValue(source.created_at)} />
        <MetaItem label="Last Seen" value={formatDateTimeValue(source.last_seen_at)} />
        <MetaItem label="Root Path" value={source.root_path ?? 'Unknown'} />
        <MetaItem label="Files" value={source.files.length} />
        <MetaItem label="Resolver Matches" value={source.resolver_matches.length} />
      </div>

      {hardDeleteEligible ? (
        <div className="flex flex-wrap justify-end gap-2">
          {pin ? (
            <Button
              type="button"
              variant="outline"
              onClick={() => onClearPin(source)}
              className="border-amber-500/30 text-amber-100 hover:bg-amber-500/10"
            >
              Clear Manual Grouping
            </Button>
          ) : null}
          <Button
            type="button"
            variant="outline"
            onClick={() => onSplit(source)}
            disabled={pin?.mode === 'split' && pin.canonical_id === canonicalGameId}
          >
            Split into Separate Game
          </Button>
          <Button type="button" variant="outline" onClick={() => onMerge(source)}>
            <ArrowRightLeft size={16} />
            Merge into Existing Game
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() => onHardDelete(source)}
            className="border-red-500/30 text-red-200 hover:bg-red-500/10"
          >
            <FileText size={16} />
            Hard Delete Source Record
          </Button>
        </div>
      ) : (
        <div className="flex flex-wrap justify-end gap-2">
          {pin ? (
            <Button
              type="button"
              variant="outline"
              onClick={() => onClearPin(source)}
              className="border-amber-500/30 text-amber-100 hover:bg-amber-500/10"
            >
              Clear Manual Grouping
            </Button>
          ) : null}
          <Button
            type="button"
            variant="outline"
            onClick={() => onSplit(source)}
            disabled={pin?.mode === 'split' && pin.canonical_id === canonicalGameId}
          >
            Split into Separate Game
          </Button>
          <Button type="button" variant="outline" onClick={() => onMerge(source)}>
            <ArrowRightLeft size={16} />
            Merge into Existing Game
          </Button>
        </div>
      )}

      <details className="rounded-mga border border-mga-border bg-mga-surface px-3 py-2">
        <summary className="cursor-pointer list-none text-sm font-medium text-mga-text">
          Resolver Matches ({source.resolver_matches.length})
        </summary>
        <div className="mt-3 space-y-2">
          {source.resolver_matches.length === 0 ? (
            <p className="text-sm text-mga-muted">No resolver matches stored for this source game.</p>
          ) : (
            source.resolver_matches.map((match) => (
              <ResolverMatchRow key={`${match.plugin_id}:${match.external_id}:${match.title ?? ''}`} match={match} />
            ))
          )}
        </div>
      </details>

      <details className="rounded-mga border border-mga-border bg-mga-surface px-3 py-2">
        <summary className="cursor-pointer list-none text-sm font-medium text-mga-text">
          Source File Inventory ({source.files.length})
        </summary>
        <div className="mt-3">
          <SourceFileInventory files={source.files} emptyMessage="No files associated with this source game." />
        </div>
      </details>
    </article>
  )
}

function MergeCanonicalDialog({
  source,
  currentCanonicalId,
  busy,
  error,
  onClose,
  onConfirm,
}: {
  source: SourceGameDetailDTO | null
  currentCanonicalId: string
  busy: boolean
  error: string
  onClose: () => void
  onConfirm: (target: CanonicalGameSearchResult) => void
}) {
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState<CanonicalGameSearchResult | null>(null)
  useEffect(() => {
    setQuery(source?.raw_title ?? '')
    setSelected(null)
  }, [source?.id, source?.raw_title])

  const search = useQuery({
    queryKey: ['canonical-games-search', query],
    queryFn: () => searchCanonicalGames({ q: query, limit: 20 }),
    enabled: source !== null,
  })
  const games = (search.data?.games ?? []).filter((game) => game.id !== currentCanonicalId)

  return (
    <Dialog open={source !== null} onClose={busy ? () => undefined : onClose} title="Merge Source into Existing Game" className="max-w-2xl">
      <div className="space-y-4">
        <p className="text-sm text-mga-muted">
          Pick the canonical game that should own this source record. Future scans will keep this source pinned there.
        </p>
        <input
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          className="w-full rounded-mga border border-mga-border bg-mga-bg px-3 py-2 text-sm text-mga-text outline-none focus:border-mga-accent"
          placeholder="Search canonical games"
          disabled={busy}
        />
        <div className="max-h-80 space-y-2 overflow-y-auto">
          {search.isPending ? (
            <div className="flex items-center gap-2 text-sm text-mga-muted">
              <Loader2 size={16} className="animate-spin" />
              Searching games...
            </div>
          ) : games.length === 0 ? (
            <p className="text-sm text-mga-muted">No merge targets found.</p>
          ) : (
            games.map((game) => (
              <button
                key={game.id}
                type="button"
                onClick={() => setSelected(game)}
                className={cn(
                  'w-full rounded-mga border px-3 py-2 text-left transition-colors',
                  selected?.id === game.id
                    ? 'border-mga-accent bg-mga-accent/10'
                    : 'border-mga-border bg-mga-bg hover:bg-mga-elevated',
                )}
                disabled={busy}
              >
                <span className="block text-sm font-semibold text-mga-text">{game.title}</span>
                <span className="mt-1 block text-xs text-mga-muted">
                  {platformLabel(game.platform)} · {game.kind} · {game.source_count} source record{game.source_count === 1 ? '' : 's'}
                </span>
              </button>
            ))
          )}
        </div>
        {error ? <p className="rounded-mga border border-red-500/30 bg-red-500/10 px-3 py-2 text-sm text-red-100">{error}</p> : null}
        <div className="flex justify-end gap-2">
          <Button type="button" variant="outline" onClick={onClose} disabled={busy}>
            Cancel
          </Button>
          <Button type="button" onClick={() => selected && onConfirm(selected)} disabled={!selected || busy}>
            {busy ? <Loader2 size={16} className="animate-spin" /> : <ArrowRightLeft size={16} />}
            Merge Source
          </Button>
        </div>
      </div>
    </Dialog>
  )
}

function MediaPreview({ media }: { media: GameMediaDetailDTO }) {
  const url = mediaUrl(media)
  const youtubeUrl = youtubeEmbedUrl(media)
  const mediaCollection = new GameMediaCollection([media])

  if (youtubeUrl) {
    return (
      <iframe
        src={youtubeUrl}
        title={`${mediaTypeLabel(media.type)} preview`}
        allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
        allowFullScreen
        className="h-[360px] w-full rounded-mga border border-mga-border bg-black"
      />
    )
  }
  if (mediaCollection.isInlineVideo(media)) {
    return (
      <video controls preload="metadata" className="max-h-[360px] w-full rounded-mga border border-mga-border bg-black">
        <source src={url} type={media.mime_type} />
      </video>
    )
  }
  if (mediaCollection.isInlineAudio(media)) {
    return (
      <div className="rounded-mga border border-mga-border bg-mga-surface p-4">
        <audio controls preload="metadata" className="w-full">
          <source src={url} type={media.mime_type} />
        </audio>
      </div>
    )
  }
  if (mediaCollection.isPdf(media)) {
    return <iframe src={url} title={`${mediaTypeLabel(media.type)} preview`} className="h-[360px] w-full rounded-mga border border-mga-border bg-white" />
  }
  if (mediaCollection.isInlineDocument(media)) {
    return <iframe src={url} title={`${mediaTypeLabel(media.type)} preview`} className="h-[360px] w-full rounded-mga border border-mga-border bg-mga-surface" />
  }
  return (
    <div className="flex items-center gap-2 rounded-mga border border-dashed border-mga-border bg-mga-surface px-3 py-4 text-sm text-mga-muted">
      {youtubeUrl || media.mime_type?.startsWith('video/') ? <Video size={16} /> : <FileText size={16} />}
      {`${mediaTypeLabel(media.type)} cannot be previewed inline in the browser. Use the external link above.`}
    </div>
  )
}

function MediaViewerDialog({ media, onClose }: { media: GameMediaDetailDTO | null; onClose: () => void }) {
  const mediaCollection = new GameMediaCollection(media ? [media] : [])
  const youtubeUrl = media ? youtubeEmbedUrl(media) : null
  return (
    <Dialog open={media !== null} onClose={onClose} title={media ? mediaTypeLabel(media.type) : 'Media'} className="max-w-5xl">
      {media && (
        <div className="space-y-4">
          <div className="overflow-hidden rounded-mga border border-mga-border bg-mga-bg">
            {mediaCollection.isImage(media) ? (
              <img src={mediaUrl(media)} alt={mediaTypeLabel(media.type)} className="max-h-[75vh] w-full object-contain" />
            ) : youtubeUrl ? (
              <iframe
                src={youtubeUrl}
                title={mediaTypeLabel(media.type)}
                allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture"
                allowFullScreen
                className="aspect-video w-full bg-black"
              />
            ) : mediaCollection.isInlineVideo(media) ? (
              <video controls preload="metadata" className="max-h-[75vh] w-full bg-black object-contain">
                <source src={mediaUrl(media)} type={media.mime_type} />
              </video>
            ) : (
              <MediaPreview media={media} />
            )}
          </div>
          <div className="flex flex-wrap items-center gap-2 text-sm text-mga-muted">
            <Badge>{mediaTypeLabel(media.type)}</Badge>
            {media.source && <SourceBadge source={media.source} />}
            {media.width && media.height && <span>{media.width} × {media.height}</span>}
            <a href={mediaOriginalUrl(media)} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 font-medium text-mga-accent hover:underline">
              Open original
              <ExternalLink size={14} />
            </a>
          </div>
        </div>
      )}
    </Dialog>
  )
}

function ExternalLinkCard({ link }: { link: ExternalLinkItem }) {
  return (
    <a href={link.url} target="_blank" rel="noreferrer" className="flex items-start gap-3 rounded-mga border border-mga-border bg-mga-bg/60 p-3 transition-colors hover:border-mga-accent">
      <div className="flex h-11 w-11 shrink-0 items-center justify-center rounded-mga border border-mga-border bg-mga-surface">
        {link.brandId ? <BrandIcon brand={link.brandId} className="h-6 w-6" /> : <ExternalLink size={16} className="text-mga-accent" />}
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <p className="font-medium text-mga-text">{link.label}</p>
          <Badge variant="muted">{link.host}</Badge>
        </div>
        <p className="mt-1 truncate text-xs text-mga-muted">{link.subtitle}</p>
        <div className="mt-2 flex flex-wrap items-center gap-2">
          <SourceBadge source={link.source} className="bg-mga-surface" />
          <span className="text-xs font-medium text-mga-accent">{link.actionLabel}</span>
        </div>
      </div>
      <ExternalLink size={16} className="mt-0.5 shrink-0 text-mga-accent" />
    </a>
  )
}

export function GameDetailPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const { id = '' } = useParams()
  const queryClient = useQueryClient()
  const { setFavorite, isPendingFor } = useGameFavoriteAction()
  const { recordLaunch } = useRecentPlayed()
  const [selectedMedia, setSelectedMedia] = useState<GameMediaDetailDTO | null>(null)
  const [refreshBusy, setRefreshBusy] = useState(false)
  const [refreshNotice, setRefreshNotice] = useState('')
  const [refreshWarnings, setRefreshWarnings] = useState<string[]>([])
  const [refreshError, setRefreshError] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<SourceGameDetailDTO | null>(null)
  const [deleteNotice, setDeleteNotice] = useState('')
  const [mergeTarget, setMergeTarget] = useState<SourceGameDetailDTO | null>(null)
  const [groupingBusy, setGroupingBusy] = useState(false)
  const [groupingError, setGroupingError] = useState('')
  const [groupingNotice, setGroupingNotice] = useState('')
  const [showFloatingActions, setShowFloatingActions] = useState(false)
  const [installChoice, setInstallChoice] = useState<DeviceInstallChoice | null>(null)
  const [uninstallChoice, setUninstallChoice] = useState<{ deviceId: string; deviceName: string; sourceGameId: string; installKind?: string } | null>(null)
  const [cleanupChoice, setCleanupChoice] = useState<{ deviceId: string; deviceName: string; sourceGameId: string; installPath: string; retryChoice?: DeviceInstallChoice } | null>(null)
  const [pendingRetry, setPendingRetry] = useState<DeviceInstallChoice | null>(null)
  const [activeDeviceCommand, setActiveDeviceCommand] = useState<{ deviceId: string; commandId: string } | null>(null)
  const [installError, setInstallError] = useState('')
  const hasRetried404Ref = useRef(false)

  const routeState = readGameRouteState(location.state)
  const from = routeState?.from ?? '/library'
  const originLabel = routeState?.originLabel ?? inferOriginLabel(from)

  const game = useQuery({
    queryKey: ['game', id],
    queryFn: async () => {
      try {
        return await getGame(id)
      } catch (error) {
        if (
          error instanceof ApiError &&
          error.status === 404 &&
          !hasRetried404Ref.current
        ) {
          hasRetried404Ref.current = true
          await queryClient.invalidateQueries({ queryKey: ['games'] })
          return getGame(id)
        }
        throw error
      }
    },
    enabled: id.length > 0,
  })

  const achievements = useQuery({
    queryKey: ['game', id, 'achievements'],
    queryFn: () => getGameAchievements(id),
    enabled: id.length > 0,
  })

	const frontendConfig = useQuery({
		queryKey: ['frontend-config'],
		queryFn: getFrontendConfig,
	})
	const saveSyncIntegrationId = frontendConfig.data?.saveSyncActiveIntegrationId?.trim() ?? ''

  const deviceCommands = useQuery({
    queryKey: ['device-commands', activeDeviceCommand?.deviceId],
    queryFn: () => listDeviceCommands(activeDeviceCommand!.deviceId),
    enabled: Boolean(activeDeviceCommand),
    refetchInterval: (query) => {
      const command = query.state.data?.find((item) => item.id === activeDeviceCommand?.commandId)
      return command && ['succeeded', 'failed', 'rejected', 'canceled', 'expired'].includes(command.status) ? false : 1000
    },
  })
  const trackedCommand = deviceCommands.data?.find((item) => item.id === activeDeviceCommand?.commandId)
  const installGame = useMutation({
    mutationFn: ({ choice, root }: { choice: DeviceInstallChoice; root: string }) =>
      choice.installKind === 'gog_inno'
        ? installGogInnoOnDevice(choice.deviceId, id, choice.sourceGameId, root)
        : installArchiveOnDevice(choice.deviceId, id, choice.sourceGameId, root),
    onSuccess: (command, variables) => {
      setInstallChoice(null)
      setInstallError('')
      setActiveDeviceCommand({ deviceId: variables.choice.deviceId, commandId: command.id })
    },
    onError: (error, variables) => {
      const detail = apiErrorDetail(error)
      setInstallError(
        variables.choice.installKind === 'gog_inno'
          ? gogInstallErrorMessage(detail.code, detail.message, variables.choice.deviceName)
          : (detail.message || 'Could not start installation.'),
      )
    },
  })
  const uninstallGame = useMutation({
    mutationFn: ({ deviceId, sourceGameId }: { deviceId: string; sourceGameId: string }) =>
      uninstallGameFromDevice(deviceId, id, sourceGameId),
    onSuccess: (command, variables) => {
      setUninstallChoice(null)
      setActiveDeviceCommand({ deviceId: variables.deviceId, commandId: command.id })
    },
  })
	const useExistingInstallation = useMutation({
		mutationFn: ({ deviceId, sourceGameId, localInstallationId }: { deviceId: string; sourceGameId: string; localInstallationId: string }) =>
			useExistingInstallationOnDevice(deviceId, id, sourceGameId, localInstallationId),
		onSuccess: (command, variables) => setActiveDeviceCommand({ deviceId: variables.deviceId, commandId: command.id }),
	})
	const launchGame = useMutation({
		mutationFn: ({ deviceId, sourceGameId }: { deviceId: string; sourceGameId: string }) =>
			launchGameOnDevice(deviceId, id, sourceGameId),
		onSuccess: (command, variables) => setActiveDeviceCommand({ deviceId: variables.deviceId, commandId: command.id }),
	})
	const launchEmulator = useMutation({
		mutationFn: ({ deviceId, sourceGameId, emulatorId }: { deviceId: string; sourceGameId: string; emulatorId: string }) =>
			launchEmulatorGameOnDevice(deviceId, id, sourceGameId, emulatorId),
		onSuccess: (command, variables) => setActiveDeviceCommand({ deviceId: variables.deviceId, commandId: command.id }),
	})
	const saveDomainAction = useMutation({
		mutationFn: async ({ action, deviceId, sourceGameId, localSaveDomainId }: { action: 'claim' | 'release' | 'snapshot' | 'snapshot-force' | 'restore' | 'restore-preserve' | 'reconcile-local' | 'reconcile-server'; deviceId: string; sourceGameId: string; localSaveDomainId?: string }) => {
			switch (action) {
				case 'claim': return claimScummVMSaveDomain(deviceId, id, sourceGameId, localSaveDomainId)
				case 'release': return releaseSaveDomain(deviceId, id, sourceGameId, localSaveDomainId ?? '')
				case 'snapshot': return snapshotSaveDomain(deviceId, id, sourceGameId, saveSyncIntegrationId)
				case 'snapshot-force': return snapshotSaveDomain(deviceId, id, sourceGameId, saveSyncIntegrationId, true)
				case 'restore': return restoreSaveDomain(deviceId, id, sourceGameId, saveSyncIntegrationId)
				case 'restore-preserve': return restoreSaveDomain(deviceId, id, sourceGameId, saveSyncIntegrationId, true)
				case 'reconcile-local': return reconcileSaveDomain(deviceId, id, sourceGameId, saveSyncIntegrationId, 'keep_local')
				case 'reconcile-server': return reconcileSaveDomain(deviceId, id, sourceGameId, saveSyncIntegrationId, 'keep_server')
			}
		},
		onSuccess: (command, variables) => setActiveDeviceCommand({ deviceId: variables.deviceId, commandId: command.id }),
	})
	const changeLaunchTarget = useMutation({
		mutationFn: ({ deviceId, sourceGameId, launchTarget }: { deviceId: string; sourceGameId: string; launchTarget: string }) =>
			setDeviceGameLaunchTarget(deviceId, id, sourceGameId, launchTarget),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: ['game', id] }),
	})
  const cleanupFailed = useMutation({
    mutationFn: (choice: NonNullable<typeof cleanupChoice>) => cleanupFailedGameOnDevice(choice.deviceId, id, choice.sourceGameId),
    onSuccess: (command, choice) => {
      if (choice.retryChoice) setPendingRetry(choice.retryChoice)
      setActiveDeviceCommand({ deviceId: choice.deviceId, commandId: command.id })
      setCleanupChoice(null)
    },
  })
	const ignoreFailed = useMutation({
		mutationFn: ({ deviceId, sourceGameId }: { deviceId: string; sourceGameId: string }) => ignoreFailedGameOnDevice(deviceId, id, sourceGameId),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: ['game', id] }),
	})
	const reopenFailed = useMutation({
		mutationFn: ({ deviceId, sourceGameId }: { deviceId: string; sourceGameId: string }) => reopenFailedGameCleanup(deviceId, id, sourceGameId),
		onSuccess: () => queryClient.invalidateQueries({ queryKey: ['game', id] }),
	})

	useEffect(() => {
		if (trackedCommand?.name !== 'game.cleanup_gog_inno_failed' || !['succeeded', 'failed', 'rejected', 'canceled', 'expired'].includes(trackedCommand.status)) return
		void queryClient.invalidateQueries({ queryKey: ['game', id] })
		if (trackedCommand.status === 'succeeded' && pendingRetry) {
			setInstallChoice(pendingRetry)
			setPendingRetry(null)
		} else if (trackedCommand.status !== 'succeeded' && pendingRetry) {
			setPendingRetry(null)
		}
	}, [id, pendingRetry, queryClient, trackedCommand])

	useEffect(() => {
		if (!trackedCommand?.name.startsWith('save.') || !['succeeded', 'failed', 'rejected', 'canceled', 'expired'].includes(trackedCommand.status)) return
		void queryClient.invalidateQueries({ queryKey: ['game', id] })
	}, [id, queryClient, trackedCommand])

  const gameData = game.data ?? null
  const mediaCollection = useMemo(() => new GameMediaCollection(gameData?.media), [gameData?.media])
  const coverMedia = useMemo(() => gameData?.cover_override ?? mediaCollection.cover(), [gameData?.cover_override, mediaCollection])
  const heroMedia = useMemo(() => mediaCollection.hero(), [mediaCollection])
  const mergedMedia = useMemo(() => mergeDisplayMedia(gameData?.media), [gameData?.media])
  const coverUrl = coverMedia ? mediaUrl(coverMedia) : null
  const featuredMedia = useMemo(() => buildFeaturedMediaRail(mergedMedia, 12).map((item) => item.media), [mergedMedia])
  const heroBackdropMedia = useMemo(
    () => gameData?.background_override ?? heroMedia ?? coverMedia ?? featuredMedia[0] ?? null,
    [coverMedia, featuredMedia, gameData?.background_override, heroMedia],
  )
  const heroBackdropUrl = heroBackdropMedia ? mediaUrl(heroBackdropMedia) : null
  const heroBackgroundSuitability = useMemo(
    () => (heroBackdropMedia ? evaluateBackgroundSuitability(heroBackdropMedia) : null),
    [heroBackdropMedia],
  )
  const [selectedBrowserSourceId, setSelectedBrowserSourceId] = useState('')
  const [selectedXcloudOptionKey, setSelectedXcloudOptionKey] = useState('')
  const browserSupported = gameData ? hasBrowserPlaySupport(gameData) : false
  const browserPlayResolution = useMemo(() => {
    if (!gameData) return null
    const runtime = getBrowserPlayPreferenceRuntime(gameData)
    const rememberedSourceId = selectedBrowserSourceId
      ? null
      : (runtime ? readBrowserPlaySourcePreference(gameData.id, runtime) : null)
    return resolveBrowserPlaySelection(gameData, {
      requestedSourceGameId: selectedBrowserSourceId || null,
      rememberedSourceGameId: rememberedSourceId,
    })
  }, [gameData, selectedBrowserSourceId])
  const browserPlaySelections = browserPlayResolution?.selections ?? []
  const selectedBrowserSelection = browserPlayResolution?.selection ?? null
  const serverLaunchOptions = gameData?.play?.options?.filter((option) => option.launchable) ?? []
  const xcloudLaunchOptions = serverLaunchOptions.filter((option) => option.kind === 'xcloud' && option.url)
  const primaryXcloudOption =
    xcloudLaunchOptions.find((option) => launchOptionKey(option) === selectedXcloudOptionKey) ??
    xcloudLaunchOptions[0] ??
    null
  const browserPlayable = browserPlaySelections.length > 0
  const browserPlayIssue = browserPlayResolution?.issue ?? null
  const browserPlayRuntime = browserPlayResolution?.runtime ?? null
  const sources = gameData ? selectSourcePlugins(gameData) : []
	const saveDomains = useMemo(() => gameData ? collectSaveDomains(gameData) : [], [gameData])
  const resolverCount = gameData
    ? gameData.source_games.reduce(
        (total, sourceGame) => total + sourceGame.resolver_matches.length,
        0,
      )
    : 0
  const externalLinks = useMemo(() => buildExternalLinks(gameData?.external_ids), [gameData?.external_ids])
  const sourceFileCount = useMemo(
    () => (gameData?.source_games ?? []).reduce((total, source) => total + source.files.length, 0),
    [gameData?.source_games],
  )
  const metadataSources = useMemo(
    () => collectUnifiedMetadataSources(gameData?.source_games ?? [], gameData?.media),
    [gameData?.media, gameData?.source_games],
  )
  const achievementSets = useMemo(() => (achievements.data ?? []).map(sortAchievementSet), [achievements.data])
  const launchableSourceCount = browserPlaySelections.length
  const heroDescription = useMemo(() => splitHeroDescription(gameData?.description), [gameData?.description])
  const playModeLabel =
    gameData?.max_players && gameData.max_players > 1
      ? `Multiplayer (${gameData.max_players})`
      : gameData?.max_players === 1
        ? 'Single Player'
        : 'Unknown'
  const availabilityLabel = browserPlayable
    ? 'Browser Play'
    : gameData?.xcloud_available
      ? 'xCloud'
      : gameData?.is_game_pass
        ? 'Game Pass'
        : platformLabel(gameData?.platform ?? '')
  const archiveSources = useMemo(
    () => (gameData?.source_games ?? []).flatMap((source) => {
      const archives = source.files.filter((file) => /\.(zip|7z|rar)$/i.test(file.path))
      return archives.length === 1 ? [{ source, archive: archives[0] }] : []
    }),
    [gameData?.source_games],
  )
  const gogInnoSources = useMemo(
    () => (gameData?.source_games ?? []).flatMap((source) => {
      const packageInfo = gogInnoPackage(source)
      return packageInfo ? [{ source, packageInfo }] : []
    }),
    [gameData?.source_games],
  )
  useEffect(() => {
    hasRetried404Ref.current = false
  }, [id])

  useEffect(() => {
    setSelectedBrowserSourceId('')
    setSelectedXcloudOptionKey('')
  }, [id])

  useEffect(() => {
    if (!achievements.isSuccess) return
    void queryClient.invalidateQueries({ queryKey: ['games'] })
  }, [achievements.isSuccess, queryClient])

  useEffect(() => {
    if (!trackedCommand || !['succeeded', 'failed', 'rejected', 'canceled', 'expired'].includes(trackedCommand.status)) return
    void queryClient.invalidateQueries({ queryKey: ['game', id] })
    void queryClient.invalidateQueries({ queryKey: ['devices'] })
  }, [id, queryClient, trackedCommand])

  useEffect(() => {
    const updateVisibility = () => {
      const threshold = window.innerHeight * 0.5
      setShowFloatingActions(window.scrollY > threshold)
    }
    updateVisibility()
    window.addEventListener('scroll', updateVisibility, { passive: true })
    window.addEventListener('resize', updateVisibility)
    return () => {
      window.removeEventListener('scroll', updateVisibility)
      window.removeEventListener('resize', updateVisibility)
    }
  }, [])

  useEffect(() => {
    if (!gameData || !browserPlayRuntime || !browserPlayResolution?.invalidRememberedSourceGameId) return
    clearBrowserPlaySourcePreference(gameData.id, browserPlayRuntime)
  }, [browserPlayResolution?.invalidRememberedSourceGameId, browserPlayRuntime, gameData])

  useEffect(() => {
    if (!game.data || id.length === 0 || game.data.id === id) return
    navigate(`/game/${encodeURIComponent(game.data.id)}`, {
      replace: true,
      state: location.state,
    })
  }, [game.data, id, location.state, navigate])

  const handleLaunchXcloud = (option?: GameLaunchOptionDTO | null) => {
    const currentGame = game.data
    const launchUrl = option?.url ?? currentGame?.xcloud_url
    if (!currentGame) return
    if (!launchUrl) return
    recordLaunch({
      gameId: currentGame.id,
      title: currentGame.title,
      platform: currentGame.platform,
      coverUrl,
      launchKind: 'xcloud',
      launchUrl,
    })
  }

  const handleLaunchBrowser = () => {
    const currentGame = game.data
    if (!currentGame || !browserPlayResolution?.canLaunch || !selectedBrowserSelection) return
    const params = new URLSearchParams()
    params.set('source', selectedBrowserSelection.sourceGame.id)
    navigate(
      {
        pathname: `/game/${encodeURIComponent(currentGame.id)}/play`,
        search: params.toString() ? `?${params.toString()}` : '',
      },
      { state: location.state },
    )
  }

  const handleBack = () => {
    const shouldRestoreScroll = from.startsWith('/play') || from.startsWith('/library')
    navigate(from, shouldRestoreScroll ? { state: { restoreScroll: true } } : undefined)
  }

  const handleBrowserSourceChange = (sourceGameId: string) => {
    setSelectedBrowserSourceId(sourceGameId)
    if (!gameData || !browserPlayRuntime || !sourceGameId) return
    writeBrowserPlaySourcePreference(gameData.id, browserPlayRuntime, sourceGameId)
  }

  const handleReclassify = () => {
    if (!game.data) return
    const params = new URLSearchParams()
    params.set('tab', 'undetected')
    const candidateId = game.data.source_games[0]?.id
    if (candidateId) {
      params.set('candidate_id', candidateId)
    }
    params.set('reclassify_game_id', game.data.id)
    params.set('reclassify_title', game.data.title)
    params.set('reclassify_platform', game.data.platform)
    const primarySource = game.data.source_games[0]?.plugin_id
    if (primarySource) {
      params.set('reclassify_source', primarySource)
    }
    navigate({ pathname: '/settings', search: params.toString() })
  }

  const handleToggleFavorite = async () => {
    if (!game.data) return
    await setFavorite({
      gameId: game.data.id,
      favorite: !game.data.favorite,
    })
  }

  const handleRefreshMetadata = async () => {
    if (!game.data || refreshBusy) return
    setRefreshBusy(true)
    setRefreshNotice('')
    setRefreshWarnings([])
    setRefreshError('')
    try {
      const refreshed = await refreshGameMetadata(game.data.id)

      // Strip ephemeral warnings before caching so they don't persist on later reads.
      const warnings = refreshed.metadata_warnings ?? []
      const { metadata_warnings: _dropped, ...gameData } = refreshed
      queryClient.setQueryData(['game', gameData.id], gameData)

      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['games'] }),
        queryClient.invalidateQueries({ queryKey: ['game', gameData.id, 'achievements'] }),
      ])

      setRefreshNotice('Metadata and media refresh completed.')
      // Surface non-fatal provider skips as separate amber warnings.
      if (warnings.length > 0) {
        setRefreshWarnings(warnings)
      }
    } catch (error) {
      const message =
        error instanceof ApiError
          ? error.responseText?.trim() || error.message
          : (error instanceof Error ? error.message : 'Refresh failed.')
      setRefreshError(message)
    } finally {
      setRefreshBusy(false)
    }
  }

  const handleRequestHardDelete = async (source: SourceGameDetailDTO) => {
    if (!game.data) return
    setDeleteNotice('')
    setDeleteTarget(source)
  }

  const handleHardDeleteCompleted = async (result: DeleteSourceGameResponse, source: SourceGameDetailDTO) => {
    if (!game.data) return
    setDeleteNotice('')
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['games'] }),
      queryClient.invalidateQueries({ queryKey: ['game', game.data.id, 'achievements'] }),
    ])
    if (result.game) {
      queryClient.setQueryData(['game', result.game.id], result.game)
    }
    if (result.canonical_exists && result.game) {
      const warningSuffix = result.warnings?.length ? ` ${result.warnings.length} directory cleanup warning${result.warnings.length === 1 ? '' : 's'}.` : ''
      setDeleteNotice(`Deleted ${sourceRecordLabel(source)}.${warningSuffix}`)
      if (result.game.id !== game.data.id) {
        navigate(`/game/${encodeURIComponent(result.game.id)}`, {
          replace: true,
          state: location.state,
        })
      } else {
        queryClient.setQueryData(['game', game.data.id], result.game)
      }
    } else {
      navigate('/library', { replace: true })
    }
    setDeleteTarget(null)
  }

  const applyGroupingResult = async (result: CanonicalGroupingResponse, message: string) => {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['games'] }),
      queryClient.invalidateQueries({ queryKey: ['duplicates'] }),
      queryClient.invalidateQueries({ queryKey: ['stats'] }),
      queryClient.invalidateQueries({ queryKey: ['achievements'] }),
      ...((result.affected_canonical_ids ?? []).flatMap((canonicalId) => [
        queryClient.invalidateQueries({ queryKey: ['game', canonicalId] }),
        queryClient.invalidateQueries({ queryKey: ['game', canonicalId, 'achievements'] }),
      ])),
    ])
    if (result.game) {
      queryClient.setQueryData(['game', result.game.id], result.game)
    }
    setGroupingNotice(message)
    setGroupingError('')
    if (result.canonical_game_id && result.canonical_game_id !== game.data?.id) {
      navigate(`/game/${encodeURIComponent(result.canonical_game_id)}`, {
        replace: true,
        state: location.state,
      })
    } else if (result.game && game.data) {
      queryClient.setQueryData(['game', game.data.id], result.game)
    }
  }

  const groupingErrorMessage = (error: unknown) =>
    error instanceof ApiError
      ? error.responseText?.trim() || error.message
      : (error instanceof Error ? error.message : 'Canonical grouping failed.')

  const handleSplitSource = async (source: SourceGameDetailDTO) => {
    if (!game.data || groupingBusy) return
    setGroupingBusy(true)
    setGroupingError('')
    setGroupingNotice('')
    try {
      const result = await splitSourceGameCanonical(game.data.id, source.id)
      await applyGroupingResult(result, `Split ${sourceRecordLabel(source)} into a separate game.`)
    } catch (error) {
      setGroupingError(groupingErrorMessage(error))
    } finally {
      setGroupingBusy(false)
    }
  }

  const handleMergeSource = (source: SourceGameDetailDTO) => {
    setGroupingError('')
    setGroupingNotice('')
    setMergeTarget(source)
  }

  const handleConfirmMergeSource = async (target: CanonicalGameSearchResult) => {
    if (!game.data || !mergeTarget || groupingBusy) return
    setGroupingBusy(true)
    setGroupingError('')
    try {
      const result = await mergeSourceGameCanonical(game.data.id, mergeTarget.id, target.id)
      const label = sourceRecordLabel(mergeTarget)
      setMergeTarget(null)
      await applyGroupingResult(result, `Merged ${label} into ${target.title}.`)
    } catch (error) {
      setGroupingError(groupingErrorMessage(error))
    } finally {
      setGroupingBusy(false)
    }
  }

  const handleClearSourcePin = async (source: SourceGameDetailDTO) => {
    if (!game.data || groupingBusy) return
    setGroupingBusy(true)
    setGroupingError('')
    setGroupingNotice('')
    try {
      const result = await clearSourceGameCanonicalPin(game.data.id, source.id)
      await applyGroupingResult(result, `Cleared manual grouping for ${sourceRecordLabel(source)}.`)
    } catch (error) {
      setGroupingError(groupingErrorMessage(error))
    } finally {
      setGroupingBusy(false)
    }
  }

  if (game.isPending) {
    return (
      <div className="mx-auto max-w-5xl space-y-4 p-4 md:p-6">
        <Button variant="outline" size="sm" onClick={handleBack}>
          <ArrowLeft size={14} />
          {originLabel}
        </Button>
        <div className="rounded-mga border border-mga-border bg-mga-surface p-6">
          <p className="text-sm text-mga-muted">Loading game details...</p>
        </div>
      </div>
    )
  }

  if (game.isError || !game.data) {
    return (
      <div className="mx-auto max-w-5xl space-y-4 p-4 md:p-6">
        <Button variant="outline" size="sm" onClick={handleBack}>
          <ArrowLeft size={14} />
          {originLabel}
        </Button>
        <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-6">
          <p className="text-sm text-red-400">{game.isError ? game.error.message : 'Game not found.'}</p>
        </div>
      </div>
    )
  }

  const data = game.data
  const favoriteBusy = isPendingFor(data.id)
  return (
    <div className="w-full space-y-8 pb-32 md:pb-36">
      <section
        id="overview"
        className="relative isolate min-h-[560px] overflow-hidden bg-[#050608] md:min-h-[620px] xl:min-h-[700px]"
      >
        <div className="absolute inset-0 bg-[#050608]" />
        {heroBackdropUrl ? (
          <>
            {heroBackgroundSuitability?.level === 'good' ? (
              <img
                src={heroBackdropUrl}
                alt=""
                className="absolute inset-0 h-full w-full object-cover object-center"
                aria-hidden="true"
              />
            ) : (
              <>
                <img
                  src={heroBackdropUrl}
                  alt=""
                  className="absolute inset-0 h-full w-full scale-[1.12] object-cover object-center opacity-55 blur-[28px]"
                  aria-hidden="true"
                />
                <img
                  src={heroBackdropUrl}
                  alt=""
                  className="absolute right-[clamp(32px,7vw,140px)] top-1/2 z-0 max-h-[78%] w-[min(44vw,620px)] -translate-y-1/2 rounded-[28px] object-contain object-center opacity-72"
                  aria-hidden="true"
                />
              </>
            )}
            <div
              className="absolute inset-0 pointer-events-none"
              style={{
                background:
                  'linear-gradient(90deg, rgba(4,8,20,0.96) 0%, rgba(4,8,20,0.88) 28%, rgba(4,8,20,0.60) 50%, rgba(4,8,20,0.24) 76%, rgba(4,8,20,0.08) 100%)',
              }}
            />
            <div
              className="absolute inset-0 pointer-events-none"
              style={{
                background:
                  'radial-gradient(circle at 72% 28%, rgba(64,126,255,0.08), transparent 40%), linear-gradient(180deg, rgba(0,0,0,0.22) 0%, rgba(0,0,0,0.04) 58%, rgba(0,0,0,0.00) 100%)',
              }}
            />
          </>
        ) : null}

        <div className="relative z-10 mx-auto max-w-[1540px] space-y-6 px-4 pb-10 pt-4 md:px-6 xl:px-8 xl:pb-12 xl:pt-5">
          <Button
            variant="outline"
            size="sm"
            onClick={handleBack}
            className="mb-4 rounded-[14px] border-white/[0.08] bg-black/18 text-white backdrop-blur-[6px] hover:bg-black/30"
          >
            <ArrowLeft size={14} />
            {originLabel}
          </Button>

          <div className="min-h-[460px] md:min-h-[520px] xl:min-h-[600px]">
            <div className="min-w-0 flex max-w-[42rem] flex-col gap-6 lg:gap-7">
              <div className="space-y-4">
                <div className="space-y-3">
                  <h1 className="max-w-3xl text-4xl font-semibold tracking-tight text-white md:text-5xl xl:text-[4.15rem] xl:leading-[1.02]">
                    {data.title}
                  </h1>
                  <AttributionNote sources={metadataSources} prefix="Metadata gathered from" />
                </div>
                {heroDescription.tagline ? (
                  <p className="max-w-2xl text-[18px] font-medium leading-8 text-white/90">
                    {heroDescription.tagline}
                  </p>
                ) : null}
                <p className="max-w-2xl text-sm leading-8 text-white/74 md:text-base">
                  {heroDescription.body || 'No description is available for this game yet.'}
                </p>
              </div>

              {browserPlayable && (browserPlaySelections.length > 1 || browserPlayIssue?.code === 'invalid_remembered_source') ? (
                <div className="max-w-2xl rounded-[24px] bg-black/18 p-4 backdrop-blur-[6px]">
                  <label className="mb-2 block text-xs uppercase tracking-[0.18em] text-white/42">Launch source</label>
                  <select
                    value={selectedBrowserSelection?.sourceGame.id ?? selectedBrowserSourceId}
                    onChange={(event) => handleBrowserSourceChange(event.target.value)}
                    className="w-full rounded-[16px] border border-white/10 bg-[#121a27] px-3 py-2.5 text-sm text-mga-text"
                  >
                    {!selectedBrowserSelection ? (
                      <option value="" disabled>
                        Choose a source to enable launch
                      </option>
                    ) : null}
                    {browserPlaySelections.map((selection) => (
                      <option key={selection.sourceGame.id} value={selection.sourceGame.id}>
                        {browserPlaySourceOptionLabel(selection, browserPlaySelections)}
                      </option>
                    ))}
                  </select>
                  <p className="mt-2 text-xs text-white/58">
                    {selectedBrowserSelection
                      ? browserPlaySourceContext(selectedBrowserSelection)
                      : (browserPlayIssue?.message ?? 'Choose the source or version to launch.')}
                  </p>
                </div>
              ) : null}

              {xcloudLaunchOptions.length > 1 ? (
                <div className="max-w-2xl rounded-[24px] bg-black/18 p-4 backdrop-blur-[6px]">
                  <label className="mb-2 block text-xs uppercase tracking-[0.18em] text-white/42">xCloud source</label>
                  <select
                    value={primaryXcloudOption ? launchOptionKey(primaryXcloudOption) : ''}
                    onChange={(event) => setSelectedXcloudOptionKey(event.target.value)}
                    className="w-full rounded-[16px] border border-white/10 bg-[#121a27] px-3 py-2.5 text-sm text-mga-text"
                  >
                    {xcloudLaunchOptions.map((option) => (
                      <option key={launchOptionKey(option)} value={launchOptionKey(option)}>
                        {launchOptionContext(option) || 'xCloud'}
                      </option>
                    ))}
                  </select>
                </div>
              ) : null}

              <div className="flex flex-wrap gap-3">
                {browserPlayable ? (
                  <HeroActionButton
                    type="button"
                    onClick={handleLaunchBrowser}
                    disabled={!browserPlayResolution?.canLaunch}
                    primary
                    className="min-w-[10rem] px-6"
                  >
                    <PlayCircle size={17} />
                    Play
                  </HeroActionButton>
                ) : null}
                {(primaryXcloudOption?.url ?? data.xcloud_url) ? (
                  <a
                    href={primaryXcloudOption?.url ?? data.xcloud_url}
                    target="_blank"
                    rel="noreferrer"
                    onClick={() => handleLaunchXcloud(primaryXcloudOption)}
                    className="inline-flex h-11 items-center justify-center gap-2 rounded-[15px] border border-white/[0.08] bg-[#101620] px-5 text-sm font-medium text-white transition-colors hover:bg-white/[0.06]"
                  >
                    <BrandIcon brand="xcloud" className="h-4 w-4 invert" />
                    Play xCloud
                  </a>
                ) : null}
                <FavoriteActionButton
                  favorite={data.favorite}
                  busy={favoriteBusy}
                  onClick={() => void handleToggleFavorite()}
                />
                <HeroOverflowMenu onRefresh={handleRefreshMetadata} onReclassify={handleReclassify} refreshBusy={refreshBusy} />
              </div>

              {browserSupported && !browserPlayable ? (
                <p className="text-xs text-white/58">
                  {browserPlayIssue?.message ?? 'Browser Play is supported for this platform, but no launchable source file was found yet.'}
                </p>
              ) : null}
              {browserSupported && browserPlayable && !browserPlayResolution?.canLaunch && browserPlayIssue ? (
                <p className="text-xs text-amber-300">{browserPlayIssue.message}</p>
              ) : null}
              {refreshBusy ? (
                <div className="flex max-w-xl items-center gap-3 rounded-[18px] border border-sky-300/20 bg-sky-300/[0.08] px-4 py-3 text-sm text-sky-50 shadow-[0_12px_28px_rgba(0,0,0,0.2)]">
                  <Loader2 size={18} className="shrink-0 animate-spin text-sky-200" />
                  <div className="min-w-0">
                    <p className="font-medium">Refreshing metadata and achievements...</p>
                    <p className="text-xs text-sky-100/72">
                      Updating provider matches, media, and achievement progress. This can take a moment.
                    </p>
                  </div>
                </div>
              ) : null}
              {refreshNotice ? <p className="text-xs text-green-400">{refreshNotice}</p> : null}
              {refreshWarnings.length > 0 ? (
                <div className="max-w-xl rounded-[16px] border border-amber-400/25 bg-amber-400/[0.08] px-4 py-3 text-xs text-amber-100 shadow-[0_8px_20px_rgba(0,0,0,0.18)]">
                  <p className="font-semibold text-amber-200">Some metadata providers were skipped:</p>
                  <ul className="mt-1.5 space-y-1 pl-1">
                    {refreshWarnings.map((w) => (
                      <li key={w} className="flex items-start gap-1.5 text-amber-100/80">
                        <span className="mt-px shrink-0 select-none">⚠</span>
                        <span>{w}</span>
                      </li>
                    ))}
                  </ul>
                </div>
              ) : null}
              {deleteNotice ? <p className="text-xs text-green-400">{deleteNotice}</p> : null}
              {groupingNotice ? <p className="text-xs text-green-400">{groupingNotice}</p> : null}
              {refreshError ? <p className="text-xs text-red-400">{refreshError}</p> : null}
              {groupingError ? <p className="text-xs text-red-400">{groupingError}</p> : null}
            </div>
          </div>
        </div>
      </section>

      <div className="mx-auto max-w-[1540px] space-y-8 px-4 md:px-6 lg:px-8">
        <section className="game-info-bar grid gap-px overflow-hidden rounded-[24px] bg-white/[0.05]">
          <div className="grid gap-px bg-white/[0.05] lg:grid-cols-5">
            <div className="min-w-0 bg-[rgba(10,14,22,0.92)]">
              <HeroFactStripItem label="Released" value={formatDateValue(data.release_date)} />
            </div>
            <div className="min-w-0 bg-[rgba(10,14,22,0.92)]">
              <HeroFactStripItem label="Developer" value={detailValue(data.developer)} />
            </div>
            <div className="min-w-0 bg-[rgba(10,14,22,0.92)]">
              <HeroFactStripItem label="Publisher" value={detailValue(data.publisher)} />
            </div>
            <div className="min-w-0 bg-[rgba(10,14,22,0.92)]">
              <HeroFactStripItem label="Play Mode" value={playModeLabel} />
            </div>
            <div className="min-w-0 bg-[rgba(10,14,22,0.92)]">
              <HeroFactStripItem
                label="Availability"
                value={availabilityLabel}
                detail={data.rating ? `Rating ${data.rating.toFixed(1)}` : undefined}
              />
            </div>
          </div>
        </section>

        <section className="featured-media space-y-3">
          <div className="featured-media__header flex items-center justify-between gap-3 px-1">
            <p className="text-[11px] font-medium uppercase tracking-[0.18em] text-white/42">
              Featured Media
            </p>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => navigate(`/game/${encodeURIComponent(data.id)}/media`, { state: location.state })}
              className="h-9 rounded-[14px] border-white/[0.08] bg-black/18 text-white backdrop-blur-[6px] hover:bg-black/28"
            >
              Open Gallery
            </Button>
          </div>
          {featuredMedia.length > 0 ? (
            <div className="featured-media__rail -mx-1 max-w-full overflow-x-auto px-1 pb-1">
              <div className="featured-media__items flex w-max max-w-none gap-3">
                {featuredMedia.map((media) => (
                  <HeroMediaThumb
                    key={mediaItemKey(media)}
                    media={media}
                    label={mediaTypeLabel(media.type)}
                    onSelect={(item) => setSelectedMedia(item)}
                  />
                ))}
              </div>
            </div>
          ) : null}
        </section>

        <nav className="sticky top-2 z-20 overflow-x-auto rounded-[24px] border border-white/[0.05] bg-[rgba(10,14,22,0.82)] px-3 py-3 backdrop-blur-xl">
          <div className="flex min-w-max gap-2">
            <HeroTabLink href="#details" label="Details" />
            {data.completion_time?.main_story || data.completion_time?.main_extra || data.completion_time?.completionist ? (
              <HeroTabLink href="#howlongtobeat" label="HowLongToBeat" />
            ) : null}
            {achievementSets.length > 0 ? <HeroTabLink href="#achievements" label="Achievements" /> : null}
			{saveDomains.length > 0 ? <HeroTabLink href="#saves" label="Saves" /> : null}
            <HeroTabLink href="#source-records" label="Sources" />
            {externalLinks.length > 0 ? <HeroTabLink href="#external-links" label="Links" /> : null}
          </div>
        </nav>

        <div className="space-y-6">
          <div id="details" className="grid gap-6 xl:grid-cols-[minmax(0,1.15fr)_minmax(0,1fr)_minmax(320px,0.95fr)]">
            <SectionCard
              title="About This Game"
              icon={<FolderOpen size={18} className="text-mga-accent" />}
              description="High-level metadata surfaced for this canonical game."
            >
              <div className="grid gap-3 sm:grid-cols-2">
                <MetaItem
                  label="Genres"
                  value={data.genres && data.genres.length > 0 ? data.genres.join(', ') : 'Unknown'}
                />
                <MetaItem
                  label="Players"
                  value={data.max_players ? `${data.max_players}` : 'Unknown'}
                />
              </div>
            </SectionCard>

            <SectionCard
              title="Game Info"
              icon={<Database size={18} className="text-mga-accent" />}
              description="Core fields kept on the canonical game."
            >
              <div className="grid gap-3 sm:grid-cols-2">
                <MetaItem label="Developer" value={detailValue(data.developer)} />
                <MetaItem label="Publisher" value={detailValue(data.publisher)} />
                <MetaItem label="Release Date" value={formatDateValue(data.release_date)} />
                <MetaItem label="Platform" value={platformLabel(data.platform)} />
                <MetaItem label="Play Mode" value={playModeLabel} />
                <MetaItem label="Rating" value={data.rating ? data.rating.toFixed(1) : 'Unknown'} />
              </div>
            </SectionCard>

            <SectionCard
              id="device-play"
              title="Ways to Play & Copies"
              icon={<Database size={18} className="text-mga-accent" />}
              description="Where this game was found and what is ready to use."
            >
              <div className="space-y-4">
                <div className="flex flex-wrap gap-2">
                  <Badge variant="platform"><PlatformIcon platform={data.platform} showLabel /></Badge>
                  {sources.map((source) => <SourceBadge key={source} source={source} className="bg-white/5 text-white" />)}
                  {data.xcloud_available ? <BrandBadge brand="xcloud" label="xCloud" /> : null}
                  {data.is_game_pass ? <Badge variant="gamepass">Game Pass</Badge> : null}
                  {browserPlayable ? <Badge variant="playable">Browser Play</Badge> : null}
                </div>
                <div className="grid gap-3 sm:grid-cols-2">
                  <MetaItem label="Browser Choices" value={launchableSourceCount} />
                  <MetaItem label="Copies" value={data.source_games.length} />
                  <MetaItem
                    label="Version"
                    value={editionLabel(
                      data.identity?.edition.platform ?? data.platform,
                      data.identity?.edition.region,
                      data.identity?.edition.edition_label,
                    )}
                  />
                  <MetaItem label="Match" value={identityStatusLabel(data.identity?.edition.state)} />
                </div>
				{data.devices?.length ? (
				  <div className="space-y-2 rounded-[18px] border border-white/[0.06] bg-black/10 p-3">
					<div className="flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.16em] text-white/58"><Monitor size={14} /> Play on a device</div>
				{data.devices.map((device) => {
				  const commandHere = activeDeviceCommand?.deviceId === device.device_id ? trackedCommand : undefined
				  const activeHere = Boolean(commandHere && !['succeeded', 'failed', 'rejected', 'canceled', 'expired'].includes(commandHere.status))
				  const downloadPercent = commandHere?.status === 'succeeded'
					? 100
					: commandHere?.progress_stage === 'download' ? (commandHere.progress_stage_percent ?? 0) : commandHere?.progress_stage === 'install' ? 100 : 0
				  const installPercent = commandHere?.status === 'succeeded'
					? 100
					: commandHere?.progress_stage === 'install' ? (commandHere.progress_stage_percent ?? 0) : 0
				  const isInstallCommand = commandHere?.name === 'game.install_archive' || commandHere?.name === 'game.install_gog_inno'
				  const isGogInstallCommand = commandHere?.name === 'game.install_gog_inno'
				  const isGogCommand = isGogInstallCommand || commandHere?.name === 'game.uninstall_gog_inno' || commandHere?.name === 'game.cleanup_gog_inno_failed'
				  const progressText = commandHere ? commandProgressMessage(commandHere) : ''
				  const installerRunning = Boolean(isGogInstallCommand && /installer running/i.test(progressText))
				  const reconciliationState = device.install_state === 'missing' || device.install_state === 'needs_repair'
				  const failureState = Boolean(device.installed && device.install_state && ['attention_required', 'cleanup_required', 'cleanup_running', 'cleanup_failed', 'ignored_failure'].includes(device.install_state))
				  const cleanupAvailable = Boolean(device.cleanup_marker_id && ['cleanup_required', 'cleanup_failed', 'ignored_failure'].includes(device.install_state ?? ''))
				  const ignoredFailure = device.install_state === 'ignored_failure'
				  const retryPackage = gogInnoSources.find(({ source }) => source.id === device.installed_source_id)
				  const existingTargetSource = data.source_games.find((source) => source.kind === 'base_game') ?? data.source_games[0]
					  return (
						<div key={device.device_id} className="rounded-[14px] bg-white/[0.04] px-3 py-2.5">
						  <div className="flex flex-wrap items-center justify-between gap-3">
							<div>
							  <p className="text-sm font-semibold text-white">{device.display_name}</p>
							  <p className="text-xs text-white/48">{device.os_user}{device.free_bytes ? ` · ${formatStorageBytes(device.free_bytes)} free` : ''}</p>
							</div>
							<div className="flex flex-wrap items-center gap-2">
							  <Badge variant={device.status === 'ready_for_setup' || device.status === 'installed' ? 'playable' : device.status === 'offline' ? 'muted' : 'default'}>
								{deviceSetupLabel(device.status)}
							  </Badge>
							  {device.emulator_routes?.filter((route) => route.state === 'ready').map((route) => (
								<Button
								  key={`emulator-${route.emulator_id}-${route.source_game_id}`}
								  size="sm"
								  variant={route.default ? 'default' : 'outline'}
								  disabled={!device.connected || !device.can_play || launchEmulator.isPending || Boolean(activeHere)}
								  title={route.reason || `Play ${route.source_title} with ${route.emulator_name}`}
								  onClick={() => launchEmulator.mutate({ deviceId: device.device_id, sourceGameId: route.source_game_id, emulatorId: route.emulator_id })}
								>
								  <PlayCircle size={14} /> {route.emulator_name}{(device.emulator_routes?.length ?? 0) > 1 ? ` · ${route.source_title}` : ''}
								</Button>
							  ))}
							  {device.installed && device.installed_source_id && !failureState && !reconciliationState ? (
								<>
								  <Button
									size="sm"
									disabled={!device.connected || !device.can_play || !device.launch_supported || !device.launch_target || launchGame.isPending || Boolean(activeHere)}
									onClick={() => launchGame.mutate({ deviceId: device.device_id, sourceGameId: device.installed_source_id! })}
								  >
									<PlayCircle size={14} /> Play
								  </Button>
								  {device.authority_mode !== 'shared_launch' ? <Button
									size="sm"
									variant="outline"
									disabled={!device.connected || !device.can_manage || !device.uninstall_supported || uninstallGame.isPending || Boolean(activeHere)}
									onClick={() => setUninstallChoice({
									  deviceId: device.device_id,
									  deviceName: device.display_name,
									  sourceGameId: device.installed_source_id!,
									  installKind: device.install_kind,
									})}
								  >
									<Trash2 size={14} /> Uninstall
								  </Button> : null}
								</>
							  ) : reconciliationState && device.installed_source_id && device.authority_mode !== 'shared_launch' ? null : failureState && device.installed_source_id ? (
								<>
								  {cleanupAvailable ? (
									<>
									  <Button size="sm" disabled={!device.connected || !device.can_manage || !device.failed_cleanup_supported || Boolean(activeHere)} onClick={() => setCleanupChoice({ deviceId: device.device_id, deviceName: device.display_name, sourceGameId: device.installed_source_id!, installPath: device.install_path ?? '' })}><Trash2 size={14} /> Clean up</Button>
									  {!ignoredFailure ? <Button size="sm" variant="outline" disabled={!device.connected || !device.can_manage || !device.failed_cleanup_supported || !retryPackage || Boolean(activeHere)} onClick={() => retryPackage && setCleanupChoice({ deviceId: device.device_id, deviceName: device.display_name, sourceGameId: device.installed_source_id!, installPath: device.install_path ?? '', retryChoice: { deviceId: device.device_id, deviceName: device.display_name, sourceGameId: device.installed_source_id!, sourceLabel: retryPackage.source.integration_label || retryPackage.source.integration_id, packageName: retryPackage.packageInfo.installerName, installKind: 'gog_inno' } })}>Retry</Button> : null}
									</>
								  ) : null}
								  {ignoredFailure ? (
									<Button size="sm" variant="outline" disabled={!device.can_manage || reopenFailed.isPending} onClick={() => reopenFailed.mutate({ deviceId: device.device_id, sourceGameId: device.installed_source_id! })}>Review cleanup</Button>
								  ) : (
									<Button
									  size="sm"
									  variant="outline"
									  disabled={!device.can_manage || ignoreFailed.isPending || device.install_state === 'cleanup_running'}
									  onClick={() => {
										if (window.confirm('Dismiss this warning? MGA will keep the installation record and will not delete or change any files.')) {
										  ignoreFailed.mutate({ deviceId: device.device_id, sourceGameId: device.installed_source_id! })
										}
									  }}
									>
									  Dismiss warning
									</Button>
								  )}
								</>
							  ) : (
								<>
								  {gogInnoSources.map(({ source, packageInfo }) => (
									<Button
									  key={`gog-inno-${source.id}`}
									  size="sm"
									  disabled={!device.connected || !device.can_manage || !device.gog_inno_install_supported || device.status === 'update_required' || Boolean(activeHere)}
									  onClick={() => {
										setInstallError('')
										setInstallChoice({
										  deviceId: device.device_id,
										  deviceName: device.display_name,
										  sourceGameId: source.id,
										  sourceLabel: source.integration_label || source.integration_id,
										  packageName: `${packageInfo.installerName}${packageInfo.companionCount ? ` + ${packageInfo.companionCount} part${packageInfo.companionCount === 1 ? '' : 's'}` : ''}`,
										  installKind: 'gog_inno',
										})
									  }}
									>
									  <Download size={14} /> Install{gogInnoSources.length > 1 ? ` · ${source.integration_label || source.raw_title}` : ''}
									</Button>
								  ))}
								  {archiveSources.map(({ source, archive }) => (
									<Button
									  key={`archive-${source.id}`}
									  size="sm"
									  disabled={!device.connected || !device.can_manage || !device.archive_install_supported || device.status === 'update_required' || Boolean(activeHere)}
									  onClick={() => {
										setInstallError('')
										setInstallChoice({
										  deviceId: device.device_id,
										  deviceName: device.display_name,
										  sourceGameId: source.id,
										  sourceLabel: source.integration_label || source.integration_id,
										  packageName: fileBasename(archive.path),
										  installKind: 'managed_archive',
										})
									  }}
									>
									  <Download size={14} /> Install{archiveSources.length > 1 ? ` · ${source.integration_label || source.raw_title}` : ''}
									</Button>
								  ))}
								</>
							  )}
							</div>
						  </div>
						  {device.installed && device.installed_source_id && device.launch_candidates && device.launch_candidates.length > 1 ? (
							<label className="mt-2 flex items-center gap-2 text-xs text-white/58">
							  <span>Starts with</span>
							  <select
								className="min-w-0 flex-1 rounded-lg border border-white/10 bg-black/30 px-2 py-1 text-white/80"
								value={device.launch_target ?? ''}
								disabled={!device.can_manage || changeLaunchTarget.isPending || Boolean(activeHere)}
								onChange={(event) => changeLaunchTarget.mutate({ deviceId: device.device_id, sourceGameId: device.installed_source_id!, launchTarget: event.target.value })}
							  >
								<option value="" disabled>Choose a game executable</option>
								{device.launch_candidates.map((candidate) => <option key={candidate} value={candidate}>{candidate}</option>)}
							  </select>
							</label>
						  ) : null}
						  {(!device.installed || (device.authority_mode === 'shared_launch' && reconciliationState)) && device.use_existing_supported && device.existing_installations?.length && existingTargetSource ? (
							<details className="mt-2 rounded-lg border border-white/10 bg-black/20 px-3 py-2 text-xs text-white/58">
							  <summary className="cursor-pointer font-medium text-white/72">Use a game already installed on this Windows user</summary>
							  <p className="mt-2 leading-5">Choose the exact copy. MGA Client will ask locally before giving this server launch-only access.</p>
							  <div className="mt-2 flex flex-wrap gap-2">
								{device.existing_installations.map((existing) => (
								  <Button
									key={existing.local_installation_id}
									size="sm"
									variant="outline"
									disabled={!device.connected || !device.can_manage || useExistingInstallation.isPending || Boolean(activeHere)}
									onClick={() => useExistingInstallation.mutate({ deviceId: device.device_id, sourceGameId: existingTargetSource.id, localInstallationId: existing.local_installation_id })}
								  >
									<FolderOpen size={14} /> {existing.title}
								  </Button>
								))}
							  </div>
							</details>
						  ) : null}
						  {reconciliationState ? (
							<p className="mt-2 rounded-lg border border-amber-300/20 bg-amber-300/10 px-3 py-2 text-xs leading-5 text-amber-100">
							  {device.install_state === 'missing'
								? device.authority_mode === 'shared_launch'
								  ? `MGA Client no longer sees launch permission for this existing installation on ${device.display_name}. Choose the existing copy again below, or use Install above for a separate MGA-managed copy.`
								  : `The installed game folder is no longer on ${device.display_name}. MGA did not remove it.`
								: `This installation is incomplete on ${device.display_name}: ${installationVerificationMessage(device.state_reason)}`}
							</p>
						  ) : failureState && !ignoredFailure ? (
							<div className="mt-2 rounded-lg border border-amber-300/20 bg-amber-300/10 px-3 py-2 text-xs leading-5 text-amber-100">
							  <p>
								{device.install_state === 'cleanup_required'
								  ? `The install didn’t finish on ${device.display_name}. Clean up the game files before trying again.`
								  : device.install_state === 'cleanup_running' ? `Cleaning up the failed install on ${device.display_name}…`
								  : device.install_state === 'cleanup_failed' ? `Cleanup couldn’t finish on ${device.display_name}. Files were preserved.`
								  : attentionRequiredMessage(device.state_reason, device.display_name)}
							  </p>
							  {!cleanupAvailable && device.install_state === 'attention_required' ? (
								<p className="mt-1 text-amber-100/75">
								  This older record has no verified cleanup information, so MGA will not remove files from {device.install_path || device.display_name}. Inspect that location if needed, then dismiss the warning to remove it from Play.
								</p>
							  ) : null}
							</div>
						  ) : ignoredFailure ? <p className="mt-2 text-xs text-white/48">Warning dismissed. MGA did not change game files on {device.display_name}.</p> : null}
						  {commandHere ? (
							<div className="mt-2">
							  <div className="flex justify-between gap-3 text-xs text-white/58"><span>{progressText}</span><span>{installerRunning ? 'In progress' : `${commandHere.progress_percent ?? 0}%`}</span></div>
							  {isInstallCommand ? (
								<div className="mt-2 space-y-2">
								  <div><div className="mb-1 flex justify-between text-[11px] text-sky-200/80"><span>Download</span><span>{downloadPercent}%</span></div><div className="h-1.5 overflow-hidden rounded-full bg-black/30"><div className="h-full rounded-full bg-sky-400 transition-[width]" style={{ width: `${downloadPercent}%` }} /></div></div>
								  <div><div className="mb-1 flex justify-between text-[11px] text-purple-200/80"><span>Install</span><span>{installerRunning ? 'In progress' : `${installPercent}%`}</span></div><div className="h-1.5 overflow-hidden rounded-full bg-black/30">{installerRunning ? <div className="h-full w-full animate-pulse rounded-full bg-purple-500/80" /> : <div className="h-full rounded-full bg-purple-500 transition-[width]" style={{ width: `${installPercent}%` }} />}</div></div>
								</div>
							  ) : (
								<div className="mt-1 h-1.5 overflow-hidden rounded-full bg-black/30"><div className="h-full rounded-full bg-mga-accent transition-[width]" style={{ width: `${commandHere.progress_percent ?? 0}%` }} /></div>
							  )}
							  {commandHere.error_code || commandHere.error_message ? <p className="mt-1 text-xs text-red-300">{isGogCommand ? gogInstallErrorMessage(commandHere.error_code, commandHere.error_message, device.display_name) : (commandHere.error_message || humanizeValue(commandHere.error_code!))}</p> : null}
							  {launchGame.isError ? <p className="mt-1 text-xs text-red-300">{launchGame.error instanceof Error ? launchGame.error.message : 'Could not start the game.'}</p> : null}
							</div>
						  ) : null}
						</div>
					  )
					})}
				  </div>
				) : null}
                <details className="rounded-[18px] border border-white/[0.06] bg-black/10 px-4 py-3 text-xs text-white/58">
                  <summary className="cursor-pointer font-medium text-white/72">Technical details</summary>
                  <div className="mt-3 grid gap-2 break-all sm:grid-cols-2">
                    <span>Game ID: {data.id}</span>
                    <span>Title ID: {data.identity?.title.id ?? 'Not available'}</span>
                    <span>Files: {sourceFileCount}</span>
                    <span>Match evidence: {data.identity?.evidence?.length ?? resolverCount}</span>
                  </div>
                </details>
              </div>
            </SectionCard>
          </div>

        {(data.completion_time?.main_story || data.completion_time?.main_extra || data.completion_time?.completionist) ? (
          <SectionCard
            id="howlongtobeat"
            title="HowLongToBeat"
            icon={<PlayCircle size={18} className="text-mga-accent" />}
            description="Estimated completion times sourced from the current metadata provider."
          >
            <div className="grid gap-3 md:grid-cols-4">
              <HeroStatCard label="Main Story" value={formatHours(data.completion_time?.main_story)} />
              <HeroStatCard label="Main + Extras" value={formatHours(data.completion_time?.main_extra)} />
              <HeroStatCard label="Completionist" value={formatHours(data.completion_time?.completionist)} />
              <HeroStatCard label="Estimate Source" value={data.completion_time?.source ? pluginLabel(data.completion_time.source) : 'Unknown'} />
            </div>
          </SectionCard>
        ) : null}

		{saveDomains.length > 0 ? (
		  <SectionCard
			id="saves"
			title="Saves"
			icon={<Save size={18} className="text-mga-accent" />}
			description="Save handling depends on how and where you play. MGA only connects versions that are proven compatible."
		  >
			<div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
			  {saveDomains.map((domain) => (
				<div key={domain.domain_id} className="rounded-[18px] border border-white/[0.06] bg-black/10 p-4">
				  <div className="flex items-start justify-between gap-3">
					<div className="min-w-0">
					  <p className="text-sm font-semibold text-white">{domain.label}</p>
					  {domain.context ? <p className="mt-0.5 truncate text-xs text-white/45">{domain.context}</p> : null}
					</div>
					<Badge variant={domain.status === 'available' ? 'playable' : domain.status === 'provider_managed' ? 'default' : 'muted'}>
					  {saveDomainStatusLabel(domain)}
					</Badge>
				  </div>
				  <p className="mt-3 text-xs leading-5 text-white/58">{domain.detail}</p>
				  {domain.authority_state === 'reconciliation_required' ? (
					<div className="mt-3 rounded-xl border border-purple-300/20 bg-purple-300/10 p-3">
					  <p className="text-xs leading-5 text-purple-100">This folder belonged to another MGA Server. Choose the copy this server should start with.</p>
					  <div className="mt-2 flex flex-wrap gap-2">
						<Button size="sm" disabled={saveDomainAction.isPending || !saveSyncIntegrationId || !domain.can_reconcile} onClick={() => saveDomainAction.mutate({ action: 'reconcile-local', deviceId: domain.device_id!, sourceGameId: domain.source_game_id!, localSaveDomainId: domain.local_save_domain_id })}>Keep this device</Button>
						{domain.has_backup ? <Button size="sm" variant="outline" disabled={saveDomainAction.isPending || !saveSyncIntegrationId || !domain.can_reconcile} onClick={() => saveDomainAction.mutate({ action: 'reconcile-server', deviceId: domain.device_id!, sourceGameId: domain.source_game_id!, localSaveDomainId: domain.local_save_domain_id })}>Use backup, keep both</Button> : null}
					  </div>
					</div>
				  ) : domain.sync_state === 'conflict' ? (
					<div className="mt-3 rounded-xl border border-amber-300/20 bg-amber-300/10 p-3">
					  <p className="text-xs leading-5 text-amber-100">Both copies changed. MGA will not choose for you.</p>
					  <div className="mt-2 flex flex-wrap gap-2">
						<Button size="sm" disabled={saveDomainAction.isPending || !saveSyncIntegrationId} onClick={() => saveDomainAction.mutate({ action: 'snapshot-force', deviceId: domain.device_id!, sourceGameId: domain.source_game_id!, localSaveDomainId: domain.local_save_domain_id })}>Keep this device</Button>
						<Button size="sm" variant="outline" disabled={saveDomainAction.isPending || !saveSyncIntegrationId || !domain.can_restore} onClick={() => saveDomainAction.mutate({ action: 'restore-preserve', deviceId: domain.device_id!, sourceGameId: domain.source_game_id!, localSaveDomainId: domain.local_save_domain_id })}>Use backup, keep both</Button>
					  </div>
					</div>
				  ) : (
					<div className="mt-3 flex flex-wrap gap-2">
					  {domain.can_claim ? <Button size="sm" disabled={saveDomainAction.isPending} onClick={() => saveDomainAction.mutate({ action: 'claim', deviceId: domain.device_id!, sourceGameId: domain.source_game_id!, localSaveDomainId: domain.local_save_domain_id })}>Set up backup</Button> : null}
					  {domain.can_snapshot ? <Button size="sm" disabled={saveDomainAction.isPending || !saveSyncIntegrationId} onClick={() => saveDomainAction.mutate({ action: 'snapshot', deviceId: domain.device_id!, sourceGameId: domain.source_game_id!, localSaveDomainId: domain.local_save_domain_id })}>Back up now</Button> : null}
					  {domain.can_restore ? <Button size="sm" variant="outline" disabled={saveDomainAction.isPending || !saveSyncIntegrationId} onClick={() => saveDomainAction.mutate({ action: 'restore', deviceId: domain.device_id!, sourceGameId: domain.source_game_id!, localSaveDomainId: domain.local_save_domain_id })}>Restore</Button> : null}
					  {domain.can_release ? <Button size="sm" variant="ghost" disabled={saveDomainAction.isPending} onClick={() => saveDomainAction.mutate({ action: 'release', deviceId: domain.device_id!, sourceGameId: domain.source_game_id!, localSaveDomainId: domain.local_save_domain_id })}>Release access</Button> : null}
					</div>
				  )}
				  {(domain.can_snapshot || domain.can_restore) && !saveSyncIntegrationId ? <p className="mt-2 text-xs text-amber-200">Choose an active Save Sync connection in Settings first.</p> : null}
				</div>
			  ))}
			</div>
			{saveDomainAction.isError ? <p className="mt-3 text-sm text-red-300">{saveDomainAction.error instanceof Error ? saveDomainAction.error.message : 'MGA could not start that save action.'}</p> : null}
		  </SectionCard>
		) : null}

        <SectionCard
          id="achievements"
          title="Achievements"
          icon={<Trophy size={18} className="text-mga-accent" />}
          description="Stored achievement progress grouped by connected achievement system."
        >
          {achievements.isPending ? (
            <p className="text-sm text-white/58">Loading achievements...</p>
          ) : achievements.isError ? (
            <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-4">
              <p className="text-sm text-red-400">{achievements.error.message}</p>
            </div>
          ) : achievementSets.length > 0 ? (
            <div className="space-y-6">
              {achievementSets.map((set) => (
                <div key={`${set.source}:${set.external_game_id}:${set.source_game_id ?? ''}`} className="space-y-4">
                  <div className="grid gap-4 xl:grid-cols-[minmax(210px,240px)_repeat(4,minmax(0,1fr))]">
                    <div className="rounded-[24px] border border-white/8 bg-[linear-gradient(180deg,rgba(72,104,236,0.95),rgba(51,75,171,0.96))] p-5 text-white shadow-[0_18px_40px_rgba(0,0,0,0.22)]">
                      <div className="flex h-full flex-col items-center justify-between gap-5 text-center">
                        <div className="flex flex-col items-center gap-2">
                          <SourceBadge source={set.source} className="border-white/20 bg-white/10 text-white" />
                          <div className="rounded-full border border-white/20 bg-white/14 px-4 py-1 text-xs font-semibold uppercase tracking-[0.22em] text-white">
                            {achievementVersionLabel(set)}
                          </div>
                          <div>
                            <p className="text-sm font-semibold text-white">{achievementSetTitle(set)}</p>
                            {achievementSetContext(set) ? (
                              <p className="mt-1 text-xs leading-5 text-white/70">{achievementSetContext(set)}</p>
                            ) : null}
                          </div>
                        </div>
                        <div className="space-y-1">
                          <p className="text-sm font-medium text-white/84">Achievement summary</p>
                          <p className="text-3xl font-semibold">{set.unlocked_count}/{set.total_count}</p>
                        </div>
                        <div className="flex justify-center">
                          <AchievementProgressRing
                            summary={{
                              source_count: 1,
                              total_count: set.total_count,
                              unlocked_count: set.unlocked_count,
                              total_points: set.total_points,
                              earned_points: set.earned_points,
                            }}
                            size={72}
                            strokeWidth={6}
                            showLabel={false}
                            className="text-white"
                          />
                        </div>
                        <div className="w-full border-t border-white/20 pt-4">
                          <p className="text-sm text-white/84">
                            {set.total_points !== undefined && set.total_points > 0
                              ? `${set.earned_points ?? 0}/${set.total_points} points`
                              : 'Points unavailable'}
                          </p>
                        </div>
                      </div>
                    </div>
                    {set.achievements.slice(0, 4).map((achievement) => (
                      <AchievementPreviewCard key={`${set.source}:${set.external_game_id}:${achievement.external_id}`} achievement={achievement} />
                    ))}
                  </div>
                  {set.achievements.length > 4 ? (
                    <details className="rounded-[22px] border border-white/8 bg-[#101622] px-4 py-3">
                      <summary className="cursor-pointer list-none text-sm font-medium text-white">
                        View all achievements ({set.achievements.length})
                      </summary>
                      <div className="mt-4 space-y-3">
                        {set.achievements.map((achievement) => (
                          <AchievementRow key={`${set.source}:${set.external_game_id}:${achievement.external_id}`} achievement={achievement} />
                        ))}
                      </div>
                    </details>
                  ) : null}
                </div>
              ))}
            </div>
          ) : (
            <p className="text-sm text-white/58">No achievements are available for this game.</p>
          )}
        </SectionCard>

        <SectionCard
          id="source-records"
          title="Source Records"
          icon={<Database size={18} className="text-mga-accent" />}
          description="Provider-specific records, resolver matches, and per-source file details remain available for inspection."
        >
          <div className="space-y-4">
            {data.source_games.length === 0 ? (
              <p className="text-sm text-white/58">No source records are stored for this game.</p>
            ) : (
              data.source_games.map((source) => (
                <SourceRecordCard
                  key={source.id}
                  source={source}
                  canonicalGameId={data.id}
                  onHardDelete={handleRequestHardDelete}
                  onSplit={handleSplitSource}
                  onMerge={handleMergeSource}
                  onClearPin={handleClearSourcePin}
                />
              ))
            )}
          </div>
        </SectionCard>

        {externalLinks.length > 0 ? (
          <SectionCard
            id="external-links"
            title="External Links"
            icon={<ExternalLink size={18} className="text-mga-accent" />}
            description="Known provider pages tied to the current canonical game."
          >
            <div className="space-y-3">
              {externalLinks.map((link) => <ExternalLinkCard key={link.id} link={link} />)}
            </div>
          </SectionCard>
        ) : null}

      </div>

      <div
        className={cn(
          'fixed inset-x-0 bottom-0 z-30 px-4 pb-4 transition-all duration-200 md:px-6 lg:px-8',
          showFloatingActions ? 'translate-y-0 opacity-100' : 'pointer-events-none translate-y-6 opacity-0',
        )}
      >
        <div className="mx-auto flex max-w-[1540px] items-center justify-between gap-4 rounded-[22px] border border-white/[0.05] bg-[rgba(9,12,20,0.92)] px-4 py-3 shadow-[0_20px_44px_rgba(0,0,0,0.34)] backdrop-blur-xl">
          <div className="flex min-w-0 items-center gap-3">
            {coverMedia ? (
              <div className="h-14 w-20 shrink-0 overflow-hidden rounded-[14px] bg-black/30">
                <img src={mediaUrl(coverMedia)} alt="" className="h-full w-full object-cover" />
              </div>
            ) : null}
            <div className="min-w-0">
              <p className="truncate text-sm font-medium text-white">{data.title}</p>
              <p className="truncate text-xs text-white/52">
                {browserPlayable ? 'Ready to play' : (primaryXcloudOption?.url ?? data.xcloud_url) ? 'Available in xCloud' : `${data.source_games.length} source record${data.source_games.length === 1 ? '' : 's'}`}
              </p>
            </div>
          </div>
          <div className="flex shrink-0 flex-wrap items-center gap-3">
            {browserPlayable ? (
              <HeroActionButton
                type="button"
                primary
                onClick={handleLaunchBrowser}
                disabled={!browserPlayResolution?.canLaunch}
                aria-label="Play"
                title="Play"
                className="h-12 w-12 rounded-full px-0"
              >
                <PlayCircle size={20} />
              </HeroActionButton>
            ) : null}
            {(primaryXcloudOption?.url ?? data.xcloud_url) ? (
              <a
                href={primaryXcloudOption?.url ?? data.xcloud_url}
                target="_blank"
                rel="noreferrer"
                onClick={() => handleLaunchXcloud(primaryXcloudOption)}
                className="inline-flex h-12 items-center justify-center gap-2 rounded-[16px] border border-white/[0.08] bg-[#101620] px-5 text-sm font-medium text-white transition-colors hover:bg-white/[0.06]"
              >
                <BrandIcon brand="xcloud" className="h-4 w-4 invert" />
                Play xCloud
              </a>
            ) : null}
            <FavoriteActionButton
              favorite={data.favorite}
              busy={favoriteBusy}
              onClick={() => void handleToggleFavorite()}
            />
            <HeroOverflowMenu direction="up" onRefresh={handleRefreshMetadata} onReclassify={handleReclassify} refreshBusy={refreshBusy} />
          </div>
        </div>
      </div>
      {refreshBusy ? (
        <div className="fixed inset-x-0 bottom-24 z-30 px-4 md:px-6 lg:px-8">
          <div className="mx-auto flex max-w-[1540px] items-center gap-3 rounded-[18px] border border-sky-300/20 bg-[rgba(9,18,30,0.94)] px-4 py-3 text-sm text-sky-50 shadow-[0_20px_44px_rgba(0,0,0,0.34)] backdrop-blur-xl">
            <Loader2 size={18} className="shrink-0 animate-spin text-sky-200" />
            <span>Refreshing metadata and achievements...</span>
          </div>
        </div>
      ) : null}
      </div>

      <MediaViewerDialog media={selectedMedia} onClose={() => setSelectedMedia(null)} />
      <DeviceInstallDialog
        gameId={id}
        choice={installChoice}
        busy={installGame.isPending}
        error={installError}
        onClose={() => setInstallChoice(null)}
        onInstall={(root) => {
          if (installChoice) installGame.mutate({ choice: installChoice, root })
        }}
      />
      <Dialog
        open={Boolean(uninstallChoice)}
        onClose={() => {
          if (!uninstallGame.isPending) setUninstallChoice(null)
        }}
        title="Uninstall game"
      >
        <div className="space-y-4">
          <p className="text-sm leading-6 text-mga-muted">
            Remove <span className="font-semibold text-mga-text">{data.title}</span> from{' '}
            <span className="font-semibold text-mga-text">{uninstallChoice?.deviceName}</span>?
            {uninstallChoice?.installKind === 'gog_inno'
              ? ' The game’s installer will remove the game. Saves and settings may remain.'
              : ' MGA will remove only the managed installation folder recorded by the client.'}
          </p>
          {uninstallGame.isError ? (
            <p className="text-sm text-red-300">
              {uninstallGame.error instanceof Error ? uninstallGame.error.message : 'Could not start uninstall.'}
            </p>
          ) : null}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" disabled={uninstallGame.isPending} onClick={() => setUninstallChoice(null)}>
              Cancel
            </Button>
            <Button
              type="button"
              disabled={uninstallGame.isPending}
              onClick={() => {
                if (uninstallChoice) uninstallGame.mutate({ deviceId: uninstallChoice.deviceId, sourceGameId: uninstallChoice.sourceGameId })
              }}
            >
              {uninstallGame.isPending ? <Loader2 size={16} className="animate-spin" /> : <Trash2 size={16} />}
              Uninstall
            </Button>
          </div>
        </div>
      </Dialog>
      <Dialog
        open={Boolean(cleanupChoice)}
        onClose={() => {
          if (!cleanupFailed.isPending) setCleanupChoice(null)
        }}
        title={cleanupChoice?.retryChoice ? 'Clean up before retrying' : 'Clean up failed install'}
      >
        <div className="space-y-4">
          <p className="text-sm leading-6 text-mga-muted">
            MGA will remove the files in this failed game folder on{' '}
            <span className="font-semibold text-mga-text">{cleanupChoice?.deviceName}</span>:
          </p>
          <p className="break-all rounded-xl border border-amber-300/20 bg-amber-300/10 px-3 py-2 font-mono text-xs leading-5 text-amber-100">
            {cleanupChoice?.installPath}
          </p>
          <p className="text-sm leading-6 text-mga-muted">
            The game’s uninstaller will run first when available. Windows components, saves, and settings outside this folder may remain.
          </p>
          {cleanupFailed.isError ? (
            <p className="text-sm text-red-300">
              {cleanupFailed.error instanceof Error ? cleanupFailed.error.message : 'Could not start cleanup.'}
            </p>
          ) : null}
          <div className="flex justify-end gap-2">
            <Button type="button" variant="outline" disabled={cleanupFailed.isPending} onClick={() => setCleanupChoice(null)}>
              Cancel
            </Button>
            <Button
              type="button"
              disabled={cleanupFailed.isPending || !cleanupChoice}
              onClick={() => {
                if (cleanupChoice) cleanupFailed.mutate(cleanupChoice)
              }}
            >
              {cleanupFailed.isPending ? <Loader2 size={16} className="animate-spin" /> : <Trash2 size={16} />}
              Clean up
            </Button>
          </div>
        </div>
      </Dialog>
      <MergeCanonicalDialog
        source={mergeTarget}
        currentCanonicalId={game.data?.id ?? ''}
        busy={groupingBusy}
        error={groupingError}
        onClose={() => setMergeTarget(null)}
        onConfirm={handleConfirmMergeSource}
      />
      <SourceGameHardDeleteDialog
        canonicalGameId={game.data?.id ?? null}
        source={deleteTarget}
        sourceLabel={sourceRecordLabel}
        onClose={() => setDeleteTarget(null)}
        onDeleted={handleHardDeleteCompleted}
      />
    </div>
  )
}
