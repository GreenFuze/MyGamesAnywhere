export type InstallationValidationStatusView = {
  state: string
  eligible_count: number
  last_finished_at?: string
}

export function installationReasonLabel(reason?: string): string {
  switch (reason) {
	case 'installer_exit_nonzero': return 'The installer reported an error before MGA could verify the installation.'
	case 'install_validation_failed': return 'The installer finished, but MGA could not verify a complete installation.'
	case 'uninstaller_missing': return 'The installer finished, but MGA could not find a safe uninstaller.'
	case 'uac_declined': return 'Windows permission was declined before installation finished.'
	case 'installer_timeout':
	case 'installer_still_running': return 'MGA could not confirm whether the installer finished. Check the device.'
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
	default: return reason ? `MGA recorded: ${reason.replace(/[_-]+/g, ' ')}.` : 'MGA needs you to review this installation.'
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
