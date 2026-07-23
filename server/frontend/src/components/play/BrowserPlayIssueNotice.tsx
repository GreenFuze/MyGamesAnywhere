import { ExternalLink } from 'lucide-react'
import { Link } from 'react-router-dom'
import type { BrowserPlaySelectionIssue } from '@/lib/browserPlayDiagnostics'
import { buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'

export function BrowserPlayIssueNotice({
  issue,
  className,
}: {
  issue: BrowserPlaySelectionIssue
  className?: string
}) {
  return (
    <div className={cn('space-y-2', className)}>
      {issue.title ? <p className="font-medium text-current">{issue.title}</p> : null}
      <p>{issue.message}</p>
      {issue.action ? (
        <Link
          to={issue.action.href}
          className={buttonVariants({ variant: 'outline', size: 'sm' })}
        >
          {issue.action.label}
          <ExternalLink size={13} />
        </Link>
      ) : null}
    </div>
  )
}
