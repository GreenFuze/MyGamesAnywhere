export type BrowserPlayIssueCode =
  | 'unsupported_platform'
  | 'missing_launch_source'
  | 'ambiguous_launch_source'
  | 'invalid_requested_source'
  | 'invalid_remembered_source'
  | 'missing_root_file'
  | 'missing_runtime_core'
  | 'missing_source_files'
  | 'missing_scummvm_files'

export type BrowserPlayIssueAction = {
  label: string
  href: string
}

export type BrowserPlaySelectionIssue = {
  code: BrowserPlayIssueCode
  title?: string
  message: string
  action?: BrowserPlayIssueAction
}

export type BrowserPlayDiagnosticSource = {
  integrationId?: string | null
  integrationLabel?: string | null
  rawTitle?: string | null
  rootPath?: string | null
  files: Array<{ path: string }>
}

const SAVE_OR_SUPPORT_EXTENSIONS = new Set([
  '.srm',
  '.sav',
  '.state',
  '.rtc',
  '.cht',
  '.ips',
  '.bps',
  '.ups',
])

function fileExtension(path: string): string {
  const normalized = path.replaceAll('\\', '/')
  const name = normalized.slice(normalized.lastIndexOf('/') + 1)
  const dot = name.lastIndexOf('.')
  return dot >= 0 ? name.slice(dot).toLowerCase() : ''
}

function quoted(value: string): string {
  return value ? `"${value}"` : 'this game'
}

export class BrowserPlayIssueResolver {
  private readonly gameTitle: string

  private readonly source?: BrowserPlayDiagnosticSource

  constructor(
    gameTitle: string,
    source?: BrowserPlayDiagnosticSource,
  ) {
    this.gameTitle = gameTitle
    this.source = source
  }

  missingRootFile(runtimeLabel: string, declaredRootFileId = ''): BrowserPlaySelectionIssue {
    const source = this.source
    const title = source?.rawTitle?.trim() || this.gameTitle.trim()
    const connection = source?.integrationLabel?.trim() || 'this connection'
    const location = source?.rootPath?.trim()
    const locationHint = location ? ` under "${location}"` : ''
    const action = this.connectionAction()

    if (declaredRootFileId) {
      return {
        code: 'missing_root_file',
        title: 'Connection data is out of date',
        message: `${runtimeLabel} cannot find the game file that MGA previously selected for ${quoted(title)}. Rescan ${connection} to refresh its files.`,
        action,
      }
    }

    const files = source?.files ?? []
    if (files.length === 0) {
      return {
        code: 'missing_root_file',
        title: 'Game files not found',
        message: `No game files are attached to ${quoted(title)}. Restore the playable game file${locationHint}, then rescan ${connection}.`,
        action,
      }
    }

    if (files.every((file) => SAVE_OR_SUPPORT_EXTENSIONS.has(fileExtension(file.path)))) {
      return {
        code: 'missing_root_file',
        title: 'Playable game file is missing',
        message: `MGA found only save or support files for ${quoted(title)}. Restore the ROM or game archive${locationHint}, then rescan ${connection}.`,
        action,
      }
    }

    return {
      code: 'missing_root_file',
      title: 'Playable game file not selected',
      message: `MGA can see files for ${quoted(title)}, but none can start the game. Make sure the main ROM, archive, disc image, or executable is present${locationHint}, then rescan ${connection}.`,
      action,
    }
  }

  private connectionAction(): BrowserPlayIssueAction {
    const integrationId = this.source?.integrationId?.trim()
    const query = new URLSearchParams({ tab: 'connections' })
    if (integrationId) query.set('integration', integrationId)
    return {
      label: integrationId ? 'Open connection' : 'Open connections',
      href: `/settings?${query.toString()}`,
    }
  }
}
