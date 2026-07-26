import type { DeviceEndpoint } from '@/api/client'

export type ClientStatusPresentation = {
  label: string
  dot: string
  border: string
  text: string
}

export function resolveClientStatusPresentation(
  authorized: boolean,
  endpoint: DeviceEndpoint | undefined,
  connected: boolean,
): ClientStatusPresentation {
  if (!authorized) return { label: 'Client unavailable', dot: 'bg-slate-500', border: 'border-slate-500/30', text: 'text-mga-muted' }
  if (!endpoint || !connected) return { label: 'Connect client', dot: 'bg-red-400', border: 'border-red-500/35', text: 'text-red-300' }
  if (endpoint.status === 'update_required') return { label: 'Client needs update', dot: 'bg-purple-400', border: 'border-purple-500/40', text: 'text-purple-300' }
  if (endpoint.status === 'error') return { label: 'Client error', dot: 'bg-red-400', border: 'border-red-500/40', text: 'text-red-300' }
  if (endpoint.status === 'busy') return { label: 'Client busy', dot: 'bg-amber-400', border: 'border-amber-500/40', text: 'text-amber-300' }
  if (endpoint.execution_mode === 'elevated') return { label: 'Client elevated', dot: 'bg-emerald-400', border: 'border-emerald-500/35', text: 'text-emerald-300' }
  return { label: 'Client ready', dot: 'bg-emerald-400', border: 'border-emerald-500/35', text: 'text-emerald-300' }
}
