import { Navigate, useSearchParams } from 'react-router'
import { LibraryCopiesReview } from '@/components/library/LibraryCopiesReview'
import { UndetectedGamesTab } from '@/components/settings/UndetectedGamesTab'
import { Tabs, type Tab } from '@/components/ui/tabs'
import { useProfiles } from '@/hooks/useProfiles'

const REVIEW_TABS: Tab[] = [
  { id: 'identify', label: 'Games to identify' },
  { id: 'copies', label: 'Copies and versions' },
]

export function LibraryReviewPage() {
  const { currentProfile } = useProfiles()
  const [searchParams, setSearchParams] = useSearchParams()
  const requestedTab = searchParams.get('tab')
  const activeTab = REVIEW_TABS.some((tab) => tab.id === requestedTab) ? requestedTab! : 'identify'

  if (currentProfile?.role !== 'admin_player') {
    return <Navigate to="/library" replace />
  }

  const handleTabChange = (tab: string) => {
    const next = new URLSearchParams(searchParams)
    next.set('tab', tab)
    setSearchParams(next, { replace: true })
  }

  return (
    <div className="space-y-5">
      <header>
        <h1 className="text-2xl font-bold text-mga-text">Library Review</h1>
        <p className="mt-1 max-w-3xl text-sm leading-6 text-mga-muted">
          Help MGA identify uncertain games and keep every version where it belongs.
        </p>
      </header>

      <Tabs tabs={REVIEW_TABS} active={activeTab} onChange={handleTabChange} />

      {activeTab === 'copies' ? <LibraryCopiesReview /> : <UndetectedGamesTab />}
    </div>
  )
}
