import { SourceMoveJobsPanel } from '@/components/settings/SourceMoveJobsPanel'
import { useProfiles } from '@/hooks/useProfiles'

// The default install-folder control belonged to the retired MGA Client, which
// owned device-local placement. Frontend integrations now choose where content
// lands on the playing device, so only server-side source management remains.
export function MySettingsTab() {
  const { currentProfile } = useProfiles()
  if (!currentProfile) return null
  return (
    <div className="space-y-6">
      <SourceMoveJobsPanel />
    </div>
  )
}
