export type InstallationValidationStatusView = {
  state: string
  eligible_count: number
  last_finished_at?: string
}

export function installationReasonLabel(reason?: string): string {
  switch (reason) {
    case 'install_path_missing': return 'The game folder is no longer on this device.'
    case 'manifest_missing': return 'MGA’s installation record is missing from the game folder.'
    case 'manifest_invalid':
    case 'manifest_identity_mismatch':
    case 'manifest_schema_unsupported': return 'MGA could not verify this game folder.'
    case 'launch_target_missing': return 'The executable used to start this game is missing.'
    case 'uninstall_target_missing': return 'The game’s uninstaller is missing.'
    case 'registered_program_missing': return 'Windows no longer lists this game as installed.'
    case 'files_missing_registration_present': return 'Windows lists this game, but its files are missing.'
    case 'unsafe_reparse_point': return 'The game folder redirects somewhere MGA cannot verify safely.'
    default: return reason ? reason.replace(/[_-]+/g, ' ') : 'This installation needs attention.'
  }
}

export function validationStatusLabel(status?: InstallationValidationStatusView, formatDate = (value: string) => new Date(value).toLocaleString()): string {
  if (!status) return 'Not checked yet'
  if (status.state === 'running') return 'Checking now…'
  if (status.state === 'waiting') return 'Waiting for device'
  if (status.state === 'disabled') return 'Automatic checks paused'
  if (status.last_finished_at) return `Last checked ${formatDate(status.last_finished_at)}`
  return status.eligible_count ? 'Check scheduled' : 'No managed games to check'
}
