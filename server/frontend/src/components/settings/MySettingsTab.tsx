import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { FolderCog, RotateCcw, Save } from 'lucide-react'
import { getProfileInstallPreference, setProfileInstallPreference } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { useProfiles } from '@/hooks/useProfiles'

const defaultInstallRoot = '%USERPROFILE%\\Games'

export function MySettingsTab() {
  const { currentProfile } = useProfiles()
  const queryClient = useQueryClient()
  const [root, setRoot] = useState(defaultInstallRoot)
  const preference = useQuery({
    queryKey: ['profile-install-preference', currentProfile?.id],
    queryFn: getProfileInstallPreference,
    enabled: Boolean(currentProfile),
  })
  useEffect(() => {
    if (preference.data) setRoot(preference.data.profile_root)
  }, [preference.data])

  const save = useMutation({
    mutationFn: (value: string) => setProfileInstallPreference(value),
    onSuccess: async (saved) => {
      setRoot(saved.profile_root)
      queryClient.setQueryData(['profile-install-preference', currentProfile?.id], saved)
      await queryClient.invalidateQueries({ queryKey: ['endpoint-install-preference'] })
    },
  })

  if (!currentProfile) return null
  return (
    <section className="rounded-mga border border-mga-border bg-mga-surface p-5 shadow-lg">
      <div className="flex items-start gap-3">
        <div className="grid h-10 w-10 shrink-0 place-items-center rounded-mga bg-mga-accent/10 text-mga-accent">
          <FolderCog className="h-5 w-5" />
        </div>
        <div>
          <h2 className="text-lg font-black text-mga-text">Where to install games</h2>
          <p className="mt-1 max-w-2xl text-sm leading-6 text-mga-muted">
            This is your starting folder on devices that do not have their own choice.
          </p>
        </div>
      </div>
      <div className="mt-5 max-w-2xl">
        <Input
          label="Default install folder"
          value={root}
          onChange={(event) => setRoot(event.target.value)}
          disabled={preference.isLoading || save.isPending}
          placeholder={defaultInstallRoot}
        />
        <p className="mt-2 text-xs leading-5 text-mga-muted">
          Values such as <code className="text-mga-text">%USERPROFILE%</code> are filled in by MGA Client for the Windows user on the selected device—not by MGA Server.
        </p>
        <div className="mt-4 flex flex-wrap gap-2">
          <Button onClick={() => save.mutate(root.trim())} disabled={!root.trim() || save.isPending || root.trim() === preference.data?.profile_root}>
            <Save className="h-4 w-4" /> Save
          </Button>
          <Button variant="outline" onClick={() => save.mutate('')} disabled={save.isPending || preference.data?.profile_root === defaultInstallRoot}>
            <RotateCcw className="h-4 w-4" /> Use the standard folder
          </Button>
        </div>
        {preference.error || save.error ? (
          <p className="mt-3 text-sm text-red-400">{(preference.error || save.error) instanceof Error ? (preference.error || save.error as Error).message : 'Could not save the install folder.'}</p>
        ) : null}
      </div>
    </section>
  )
}
