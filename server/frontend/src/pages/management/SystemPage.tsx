import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Download, KeyRound, ListChecks, Plus, RefreshCw, ServerCog, Trash2 } from 'lucide-react'
import {
  createFrontendAPIClient,
  getAboutInfo,
  getLegacyClientDataReport,
  listFrontendAPIClients,
  revokeFrontendAPIClient,
  rotateFrontendAPIClient,
  type FrontendAPIClient,
  type IssuedFrontendAPIClient,
} from '@/api/client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  ActionError, ConfirmDialog, FormDialog, RestrictedNotice, ShowOnceSecret,
} from '@/components/management/ManagementActions'
import {
  MetricCard, PageIntro, QueryFeedback, SectionCard, StatusPill, formatCount, formatDate,
} from '@/components/management/ManagementPrimitives'
import { useProfiles } from '@/hooks/useProfiles'
import { DESTRUCTIVE_ACTIONS, ManagementPolicy } from '@/lib/managementPolicy'

export function SystemPage() {
  const { currentProfile } = useProfiles()
  const policy = new ManagementPolicy(currentProfile)
  const admin = policy.can('apiClient.issue')
  const queryClient = useQueryClient()

  const about = useQuery({ queryKey: ['management', 'about'], queryFn: getAboutInfo })
  const clients = useQuery({
    queryKey: ['management', 'frontend-clients'],
    queryFn: listFrontendAPIClients,
    enabled: admin,
  })

  const [creating, setCreating] = useState(false)
  const [revoking, setRevoking] = useState<FrontendAPIClient | null>(null)
  const [issued, setIssued] = useState<IssuedFrontendAPIClient | null>(null)

  const activeClients = clients.data?.clients.filter((client) => !client.revoked_at).length ?? 0
  const invalidateClients = () => queryClient.invalidateQueries({ queryKey: ['management', 'frontend-clients'] })

  const rotate = useMutation({
    mutationFn: (clientId: string) => rotateFrontendAPIClient(clientId),
    onSuccess: async (result) => { setIssued(result); await invalidateClients() },
  })
  const revoke = useMutation({
    mutationFn: (clientId: string) => revokeFrontendAPIClient(clientId),
    onSuccess: async () => { setRevoking(null); await invalidateClients() },
  })

  return (
    <div className="mga-page-enter space-y-7">
      <PageIntro
        eyebrow="Server"
        title="Server and app access"
        description="Server details, and the access keys you give to apps that connect to MGA."
      />

      <div className="grid gap-4 sm:grid-cols-3">
        <MetricCard label="Version" value={about.isPending ? '…' : about.data?.version || 'Development'} detail="Version running right now" icon={<ServerCog className="h-4 w-4" />} />
        <MetricCard label="Connected apps" value={admin ? formatCount(activeClients) : 'Restricted'} detail={admin ? 'Holding a key to this profile' : 'Administrator role required'} icon={<KeyRound className="h-4 w-4" />} />
        <MetricCard label="Permissions" value={admin ? formatCount(clients.data?.supported_scopes.length) : 'Restricted'} detail="Permissions you can grant to an app" icon={<ListChecks className="h-4 w-4" />} />
      </div>

      {issued && (
        <SectionCard title="Your new access key" description="Copy this into the app now. It will not be shown again.">
          <ShowOnceSecret
            label={`Token for ${issued.name}`}
            value={issued.token}
            warning={issued.transport_warning || clients.data?.transport_warning}
          />
          <p className="mt-3 text-xs text-mga-muted">Scopes: {issued.scopes.join(' · ')}</p>
          <Button variant="outline" size="sm" className="mt-3" onClick={() => setIssued(null)}>Done</Button>
        </SectionCard>
      )}

      <div className="grid gap-5 xl:grid-cols-2">
        <SectionCard title="MGA server" description="What this server is running.">
          <QueryFeedback pending={about.isPending} error={about.error} empty={false} emptyTitle="" emptyDescription="" />
          {about.data && (
            <dl className="grid grid-cols-[8rem_1fr] gap-x-4 gap-y-3 text-xs">
              <dt className="text-mga-muted">Version</dt><dd className="font-mono text-mga-text">{about.data.version || 'development'}</dd>
              <dt className="text-mga-muted">Commit</dt><dd className="truncate font-mono text-mga-text">{about.data.commit || 'working tree'}</dd>
              <dt className="text-mga-muted">Build date</dt><dd className="text-mga-text">{about.data.build_date || 'development build'}</dd>
                          </dl>
          )}
        </SectionCard>

        <SectionCard title="Connected apps" description="Each key works for one profile only, grants just what you choose, can be revoked at any time, and is shown once.">
          {!admin ? (
            <RestrictedNotice>
              Switch to an administrator profile to issue or revoke external frontend clients.
            </RestrictedNotice>
          ) : (
            <>
              <div className="mb-4">
                <Button size="sm" onClick={() => { setIssued(null); setCreating(true) }}>
                  <Plus className="h-3.5 w-3.5" /> Issue client
                </Button>
              </div>
              <QueryFeedback
                pending={clients.isPending}
                error={clients.error}
                empty={!clients.isPending && (clients.data?.clients.length ?? 0) === 0}
                emptyTitle="No apps connected yet"
                emptyDescription="Create a key when you want to connect an app such as Playnite."
              />
              {(clients.data?.clients.length ?? 0) > 0 && (
                <div className="space-y-2">
                  {clients.data?.clients.map((client) => (
                    <div key={client.id} className="rounded-lg border border-mga-border bg-mga-elevated/40 p-3">
                      <div className="flex items-center justify-between gap-3">
                        <p className="truncate text-sm font-medium text-mga-text">{client.name}</p>
                        <StatusPill label={client.revoked_at ? 'Revoked' : 'Active'} tone={client.revoked_at ? 'danger' : 'good'} />
                      </div>
                      <p className="mt-2 text-xs text-mga-muted">{client.scopes.join(' · ')}</p>
                      <p className="mt-1 text-[0.68rem] text-mga-muted">Last used {formatDate(client.last_used_at)}</p>
                      {!client.revoked_at && (
                        <div className="mt-3 flex flex-wrap gap-2">
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={rotate.isPending}
                            onClick={() => rotate.mutate(client.id)}
                          >
                            <RefreshCw className="h-3.5 w-3.5" />
                            {rotate.isPending && rotate.variables === client.id ? 'Rotating…' : 'Rotate token'}
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-rose-300 hover:bg-rose-500/10"
                            onClick={() => { revoke.reset(); setRevoking(client) }}
                          >
                            <Trash2 className="h-3.5 w-3.5" /> Revoke
                          </Button>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
              <ActionError error={rotate.error} className="mt-3" />
            </>
          )}
        </SectionCard>
      </div>

      {admin && <LegacyRetirementExport />}

      {creating && (
        <IssueClientDialog
          scopes={clients.data?.supported_scopes ?? []}
          transportWarning={clients.data?.transport_warning}
          onClose={() => setCreating(false)}
          onIssued={async (client) => {
            setCreating(false)
            setIssued(client)
            await invalidateClients()
          }}
        />
      )}

      <ConfirmDialog
        open={revoking !== null}
        title={`Revoke ${revoking?.name ?? 'client'}?`}
        confirmLabel="Revoke client"
        submitting={revoke.isPending}
        error={revoke.error}
        onClose={() => setRevoking(null)}
        onConfirm={() => revoking && revoke.mutate(revoking.id)}
        consequences={DESTRUCTIVE_ACTIONS['apiClient.revoke'].consequences}
        preserves={DESTRUCTIVE_ACTIONS['apiClient.revoke'].preserves}
      />
    </div>
  )
}

function IssueClientDialog({
  scopes, transportWarning, onClose, onIssued,
}: {
  scopes: string[]
  transportWarning?: string
  onClose: () => void
  onIssued: (client: IssuedFrontendAPIClient) => Promise<void>
}) {
  const [name, setName] = useState('')
  const [selected, setSelected] = useState<string[]>([])

  const create = useMutation({
    mutationFn: () => createFrontendAPIClient({ name: name.trim(), scopes: selected }),
    onSuccess: (client) => void onIssued(client),
  })

  const toggle = (scope: string) => setSelected((current) =>
    current.includes(scope) ? current.filter((item) => item !== scope) : [...current, scope])

  return (
    <FormDialog
      open
      onClose={onClose}
      title="Connect an app"
      description="Grant only what the app needs. The key is shown once, so copy it now."
      submitLabel="Issue client"
      submitting={create.isPending}
      error={create.error}
      disabled={name.trim() === '' || selected.length === 0}
      onSubmit={() => create.mutate()}
    >
      <Input label="Client name" value={name} onChange={(event) => setName(event.target.value)} autoFocus />
      <fieldset className="space-y-2">
        <legend className="text-sm font-medium text-mga-text">Scopes</legend>
        {scopes.length === 0 && <p className="text-xs text-mga-muted">No scopes were reported by the server.</p>}
        {scopes.map((scope) => (
          <label key={scope} className="flex items-center gap-2 text-xs text-mga-text">
            <input
              type="checkbox"
              checked={selected.includes(scope)}
              onChange={() => toggle(scope)}
              className="h-4 w-4 accent-[color:var(--mga-accent,#7c5cff)]"
            />
            <span className="font-mono">{scope}</span>
          </label>
        ))}
      </fieldset>
      {transportWarning && (
        <p className="rounded-lg border border-amber-400/25 bg-amber-400/5 p-3 text-xs leading-5 text-amber-200">
          {transportWarning}
        </p>
      )}
    </FormDialog>
  )
}

/** Downloads the admin-only retirement evidence as a file rather than only
 * naming the endpoint. The report contains recovery paths, so it is treated as
 * sensitive and never rendered inline. */
function LegacyRetirementExport() {
  const download = useMutation({
    mutationFn: async () => {
      const report = await getLegacyClientDataReport()
      const blob = new Blob([JSON.stringify(report, null, 2)], { type: 'application/json' })
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `mga-legacy-client-report-${new Date().toISOString().slice(0, 10)}.json`
      anchor.click()
      URL.revokeObjectURL(url)
    },
  })

  return (
    <SectionCard
      title="Old MGA client data"
      description="A record of the old device and install features, kept for reference. No passwords or keys are included."
    >
      <div className="flex flex-col gap-3 rounded-lg border border-mga-border bg-mga-elevated/40 p-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <p className="text-sm font-medium text-mga-text">Installation and endpoint report</p>
          <p className="mt-1 text-xs leading-5 text-mga-muted">
            Contains install, runtime, save-domain, and prepared-copy paths for owner recovery. Treat the
            downloaded file as sensitive.
          </p>
        </div>
        <Button variant="outline" disabled={download.isPending} onClick={() => download.mutate()}>
          <Download className="h-4 w-4" /> {download.isPending ? 'Preparing…' : 'Download report'}
        </Button>
      </div>
      <ActionError error={download.error} className="mt-3" />
    </SectionCard>
  )
}
