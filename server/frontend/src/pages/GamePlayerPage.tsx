import { useEffect, useMemo, useRef, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, ExternalLink, PlayCircle } from 'lucide-react'
import { useLocation, useNavigate, useParams } from 'react-router-dom'
import { ApiError, getGame } from '@/api/client'
import { BrandBadge } from '@/components/ui/brand-icon'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { PlatformIcon } from '@/components/ui/platform-icon'
import { useRecentPlayed } from '@/hooks/useRecentPlayed'
import {
  browserPlayRuntimeLabel,
  buildBrowserPlaySession,
  buildBrowserPlayerUrl,
  clearBrowserPlaySession,
  persistBrowserPlaySession,
  selectBrowserPlaySelection,
  type BrowserPlaySession,
} from '@/lib/browserPlay'
import { hasBrowserPlaySupport } from '@/lib/gameUtils'

function LaunchFrame({ src, title }: { src: string; title: string }) {
  return (
    <iframe
      src={src}
      title={title}
      allow="autoplay; fullscreen; gamepad"
      className="h-full w-full border-0 bg-black"
    />
  )
}

export function GamePlayerPage() {
  const navigate = useNavigate()
  const location = useLocation()
  const { id = '' } = useParams()
  const { recordLaunch } = useRecentPlayed()
  const [sessionToken, setSessionToken] = useState<string | null>(null)
  const recordedRef = useRef<string | null>(null)
  const tokenRef = useRef<string | null>(null)

  const game = useQuery({
    queryKey: ['game', id],
    queryFn: async () => {
      try {
        return await getGame(id)
      } catch (error) {
        if (error instanceof ApiError && error.status === 404) {
          return getGame(id)
        }
        throw error
      }
    },
    enabled: id.length > 0,
  })

  const selection = useMemo(
    () => (game.data ? selectBrowserPlaySelection(game.data) : null),
    [game.data],
  )
  const session = useMemo<BrowserPlaySession | null>(
    () => (game.data && selection ? buildBrowserPlaySession(game.data, selection) : null),
    [game.data, selection],
  )
  const playerUrl = useMemo(() => {
    if (!sessionToken || !session) return null
    return buildBrowserPlayerUrl(session.runtime, sessionToken)
  }, [session, sessionToken])

  useEffect(() => {
    if (tokenRef.current) {
      clearBrowserPlaySession(tokenRef.current)
      tokenRef.current = null
    }

    if (!session) {
      setSessionToken(null)
      return
    }

    const nextToken = persistBrowserPlaySession(session)
    tokenRef.current = nextToken
    setSessionToken(nextToken)

    return () => {
      clearBrowserPlaySession(nextToken)
      if (tokenRef.current === nextToken) {
        tokenRef.current = null
      }
    }
  }, [session])

  useEffect(() => {
    if (!game.data || !session || !playerUrl) return
    if (recordedRef.current === playerUrl) return
    recordedRef.current = playerUrl
    recordLaunch({
      gameId: game.data.id,
      title: game.data.title,
      platform: game.data.platform,
      launchKind: 'browser',
      launchUrl: `/game/${encodeURIComponent(game.data.id)}/play`,
    })
  }, [game.data, playerUrl, recordLaunch, session])

  const handleBack = () => {
    navigate(`/game/${encodeURIComponent(id)}`, { state: location.state })
  }

  if (game.isPending) {
    return (
      <div className="min-h-screen bg-mga-bg text-mga-text">
        <div className="mx-auto flex min-h-screen max-w-7xl flex-col gap-6 p-4 md:p-6">
          <Button variant="outline" size="sm" onClick={handleBack} className="w-fit">
            <ArrowLeft size={14} />
            Back to Game
          </Button>
          <div className="rounded-mga border border-mga-border bg-mga-surface p-6 text-sm text-mga-muted">
            Loading browser player...
          </div>
        </div>
      </div>
    )
  }

  if (game.isError || !game.data) {
    return (
      <div className="min-h-screen bg-mga-bg text-mga-text">
        <div className="mx-auto flex min-h-screen max-w-7xl flex-col gap-6 p-4 md:p-6">
          <Button variant="outline" size="sm" onClick={handleBack} className="w-fit">
            <ArrowLeft size={14} />
            Back to Game
          </Button>
          <div className="rounded-mga border border-red-500/30 bg-red-500/10 p-6 text-sm text-red-300">
            {game.isError ? game.error.message : 'Game not found.'}
          </div>
        </div>
      </div>
    )
  }

  const data = game.data
  const runtimeLabel = selection ? browserPlayRuntimeLabel(selection.runtime) : null

  return (
    <div className="min-h-screen bg-mga-bg text-mga-text">
      <div className="mx-auto flex min-h-screen max-w-[1600px] flex-col gap-4 p-4 md:p-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Button variant="outline" size="sm" onClick={handleBack}>
            <ArrowLeft size={14} />
            Back to Game
          </Button>
          {data.xcloud_url && (
            <a href={data.xcloud_url} target="_blank" rel="noreferrer">
              <Button variant="outline" size="sm">
                <ExternalLink size={14} />
                Play on xCloud
              </Button>
            </a>
          )}
        </div>

        <section className="rounded-mga border border-mga-border bg-mga-surface p-4 md:p-5">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant="platform">
              <PlatformIcon platform={data.platform} showLabel />
            </Badge>
            {runtimeLabel && <Badge variant="playable">{runtimeLabel}</Badge>}
            {data.xcloud_available && <BrandBadge brand="xcloud" label="xCloud" />}
          </div>
          <div className="mt-3">
            <h1 className="text-2xl font-semibold tracking-tight md:text-3xl">{data.title}</h1>
            <p className="mt-2 text-sm text-mga-muted">
              Dedicated browser player route for fullscreen play, runtime lifecycle, and local save-state UX.
            </p>
          </div>
        </section>

        {!data.play?.platform_supported ? (
          <section className="rounded-mga border border-mga-border bg-mga-surface p-6 text-sm text-mga-muted">
            This platform is not part of the supported browser-play set for Phase 6.
          </section>
        ) : !data.play?.available || !selection ? (
          <section className="rounded-mga border border-mga-border bg-mga-surface p-6 text-sm text-mga-muted">
            Browser Play is supported for this platform, but no launchable source file was found for this game yet.
          </section>
        ) : !session || !playerUrl ? (
          <section className="rounded-mga border border-red-500/30 bg-red-500/10 p-6 text-sm text-red-200">
            Failed to assemble a browser-play launch session for this game.
          </section>
        ) : (
          <section className="flex min-h-[70vh] flex-1 flex-col overflow-hidden rounded-[1.25rem] border border-mga-border bg-black shadow-lg shadow-black/25">
            <div className="flex items-center justify-between border-b border-white/10 bg-black/80 px-4 py-3 text-sm text-white/80">
              <div className="flex items-center gap-2">
                <PlayCircle size={16} />
                <span>{data.title}</span>
              </div>
              <span className="text-xs uppercase tracking-wide text-white/50">{runtimeLabel}</span>
            </div>
            <div className="flex-1">
              <LaunchFrame src={playerUrl} title={`${data.title} browser player`} />
            </div>
          </section>
        )}

        {hasBrowserPlaySupport(data) && data.xcloud_url && (
          <p className="text-xs text-mga-muted">
            xCloud stays external in Phase 6. Browser Play and xCloud are separate launch paths.
          </p>
        )}
      </div>
    </div>
  )
}
