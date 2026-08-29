import { useQuery } from '@tanstack/react-query'
import { Boxes, Scale, ShieldAlert } from 'lucide-react'
import { listRuntimeArtifacts } from '@/api/client'
import { MetricCard, PageIntro, QueryFeedback, SectionCard, StatusPill, formatCount, formatDate } from '@/components/management/ManagementPrimitives'

export function ArtifactsPage() {
  const query = useQuery({ queryKey: ['management', 'runtime-artifacts'], queryFn: listRuntimeArtifacts })
  const artifacts = query.data ?? []
  const approved = artifacts.filter((artifact) => artifact.compliance_state === 'approved').length
  const emulators = artifacts.filter((artifact) => artifact.category === 'emulator').length
  return <div className="mga-page-enter space-y-7">
    <PageIntro eyebrow="Runtime supply" title="Emulators and runtime artifacts" description="Catalog versioned emulators and runtimes with explicit license, integrity, acquisition, compatibility, and redistribution evidence before MGA can serve their bytes." />
    <div className="grid gap-4 sm:grid-cols-3"><MetricCard label="Artifacts" value={formatCount(artifacts.length)} detail="Versioned runtime supply records" icon={<Boxes className="h-4 w-4" />} /><MetricCard label="Emulators" value={formatCount(emulators)} detail="Emulator packages in the registry" icon={<Scale className="h-4 w-4" />} /><MetricCard label="Approved" value={formatCount(approved)} detail={`${Math.max(artifacts.length - approved, 0)} blocked or awaiting compliance evidence`} tone={approved === artifacts.length ? 'good' : 'attention'} icon={<ShieldAlert className="h-4 w-4" />} /></div>
    <SectionCard title="Artifact registry" description="Unknown or blocked compliance fails closed. Only approved, redistributable, integrity-verified content is deliverable.">
      <QueryFeedback pending={query.isPending} error={query.error} empty={!query.isPending && artifacts.length === 0} emptyTitle="No runtime artifacts registered" emptyDescription="Register lawful emulator and runtime packages with their SPDX license and integrity evidence before frontend clients request them." />
      {artifacts.length > 0 && <div className="grid gap-3 xl:grid-cols-2">{artifacts.map((artifact) => <article key={artifact.id} className="rounded-lg border border-mga-border bg-mga-elevated/40 p-4"><div className="flex items-start justify-between gap-4"><div><p className="text-sm font-semibold text-mga-text">{artifact.display_name}</p><p className="mt-1 text-xs text-mga-muted">{artifact.version} · {artifact.os}/{artifact.architecture} · {artifact.channel}</p></div><StatusPill label={artifact.compliance_state} tone={artifact.compliance_state === 'approved' ? 'good' : artifact.compliance_state === 'blocked' ? 'danger' : 'attention'} /></div><div className="mt-4 grid grid-cols-2 gap-3 text-xs"><div><span className="text-mga-muted">License</span><p className="mt-1 text-mga-text">{artifact.license_spdx}</p></div><div><span className="text-mga-muted">Acquisition</span><p className="mt-1 capitalize text-mga-text">{artifact.acquisition_mode.replace('_', ' ')}</p></div><div><span className="text-mga-muted">Delivery</span><p className="mt-1 text-mga-text">{artifact.redistributable ? 'Redistributable' : 'Reference only'}</p></div><div><span className="text-mga-muted">Updated</span><p className="mt-1 text-mga-text">{formatDate(artifact.updated_at)}</p></div></div></article>)}</div>}
    </SectionCard>
  </div>
}
