/**
 * What to show when no synced Google Drive folder could be found.
 *
 * Finding nothing is the expected answer on Linux, where Google ships no Drive
 * client at all, so this is a normal branch rather than an error path. The
 * console offers a way to create the folder instead of an empty picker.
 *
 * Everything here is text to copy and paste, never a file to download. On
 * Windows the execution policy blocks a downloaded .ps1 by default, so handing
 * someone a script file is handing them something they cannot run; text pasted
 * into a terminal always works, on every platform.
 */

export type MountPlatform = 'windows' | 'macos' | 'linux'

export type MountInstructions = {
  platform: MountPlatform
  heading: string
  /** Why this platform needs anything at all. */
  summary: string
  /** Shell to paste. Empty when the answer is to install an app instead. */
  script: string
  /** Where to get the official client, when there is one. */
  download?: { label: string; url: string }
}

const RCLONE_STEPS = `# 1. Install rclone (https://rclone.org/install/)
#    macOS:  brew install rclone
#    Linux:  sudo -v ; curl https://rclone.org/install.sh | sudo bash

# 2. Connect your Google account. Choose "drive" and accept the defaults;
#    pick "scope 2" (read-only) unless you want MGA to write to Drive.
rclone config

# 3. Mount it somewhere MGA can read. Replace "gdrive" if you named the
#    remote something else.
mkdir -p "$HOME/GoogleDrive"
rclone mount gdrive: "$HOME/GoogleDrive" --vfs-cache-mode full --daemon

# 4. Check it is there, then paste that path into MGA.
ls "$HOME/GoogleDrive"`

const LINUX_SERVICE = `
# Optional: keep it mounted across reboots.
mkdir -p ~/.config/systemd/user
cat > ~/.config/systemd/user/rclone-gdrive.service <<'UNIT'
[Unit]
Description=Google Drive (rclone)
AfterExecStart=network-online.target

[Service]
Type=notify
ExecStart=%h/.local/bin/rclone mount gdrive: %h/GoogleDrive --vfs-cache-mode full
ExecStop=/bin/fusermount -u %h/GoogleDrive
Restart=on-failure

[Install]
WantedBy=default.target
UNIT
systemctl --user daemon-reload
systemctl --user enable --now rclone-gdrive.service`

export function mountInstructions(platform: MountPlatform): MountInstructions {
  switch (platform) {
    case 'windows':
      return {
        platform,
        heading: 'Install Google Drive for Desktop',
        summary:
          'Google Drive for Desktop gives this machine a drive letter, such as G:, holding your Drive. Once it is signed in, come back here and MGA will find it.',
        script: '',
        download: { label: 'Download Google Drive for Desktop', url: 'https://www.google.com/drive/download/' },
      }
    case 'macos':
      return {
        platform,
        heading: 'Install Google Drive for Desktop',
        summary:
          'Google Drive for Desktop puts your Drive under Library/CloudStorage. Once it is signed in, come back here and MGA will find it. If you would rather not install it, rclone works too.',
        script: RCLONE_STEPS,
        download: { label: 'Download Google Drive for Desktop', url: 'https://www.google.com/drive/download/' },
      }
    case 'linux':
    default:
      return {
        platform: 'linux',
        heading: 'Mount your Drive with rclone',
        summary:
          'Google does not make a Drive client for Linux, so nothing is mounted by default. rclone is the usual way to turn Drive into a folder this server can read. Paste this into a terminal on the machine running MGA.',
        script: `${RCLONE_STEPS}\n${LINUX_SERVICE}`,
      }
  }
}

/** Best guess at what the person is running, so the right instructions are on
 *  top. They can still switch: a server is often not the machine you browse
 *  from, which is exactly when the guess is wrong. */
export function guessPlatform(userAgent: string): MountPlatform {
  const agent = userAgent.toLowerCase()
  if (agent.includes('windows')) return 'windows'
  if (agent.includes('mac os') || agent.includes('macintosh')) return 'macos'
  return 'linux'
}

export const MOUNT_PLATFORMS: { id: MountPlatform; label: string }[] = [
  { id: 'windows', label: 'Windows' },
  { id: 'macos', label: 'macOS' },
  { id: 'linux', label: 'Linux' },
]
