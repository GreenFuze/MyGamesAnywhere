# MyGamesAnywhere (MGA)

![MyGamesAnywhere](docs/branding/title-text.png)

## Every game you have. Every way you can play it.

**MyGamesAnywhere is a self-hosted home game library for Windows households.** Browse Steam, Xbox and Game Pass, ROMs, installers, and NAS games from any browser at home; keep every version clear; and send supported installs or launches to the Windows user and PC you choose.

`Windows` · `Pre-1.0` · `Trusted home network`

## 🎮 [Visit the MGA website →](https://greenfuze.github.io/MyGamesAnywhere/)

See the real interface, check what works today, and find the setup that fits your home.

**[See MGA in action](https://greenfuze.github.io/MyGamesAnywhere/screenshots.html)** · [Install MGA Server](https://greenfuze.github.io/MyGamesAnywhere/install.html) · [Check compatibility](https://greenfuze.github.io/MyGamesAnywhere/integrations.html)

[![MGA showing a privacy-safe home game library](docs/screenshots/library-current.png)](https://greenfuze.github.io/MyGamesAnywhere/screenshots.html)

## Why MGA?

- **Browse from any screen at home.** The library lives on your MGA Server and opens in a browser on a PC, TV, phone, or tablet.
- **Know what you have and where it can play.** Steam, Xbox, regional ROMs, remasters, and other editions can appear together without becoming indistinguishable.
- **Choose the right gaming PC.** The MGA Client can install or launch supported games for a specific Windows user and device.

## What works today

- Library discovery from Steam, Xbox/PC Game Pass, Google Drive, and SMB/NAS folders; Epic support is experimental.
- Xbox-backed Game Pass and xCloud availability, plus Steam, Xbox, and RetroAchievements progress.
- Installed, storefront, configured browser-emulator, and xCloud play choices where the connection supports them.
- Separate MGA profiles, store accounts, favorites, achievements, and library state.
- Device-aware launching, stopping, storage checks, and install progress through the per-user Windows Client.
- Managed installation from ZIP, 7z, and RAR archives, plus a bounded signed-GOG installer flow.
- Browser play through configured EmulatorJS, js-dos, and ScummVM runtimes.

MGA does not replace Steam, Xbox, or other stores, and it is not a general PC game-streaming service. Storefronts still own purchases, DRM, many installs, and updates.

## Quick start

**Install the Server once. Install the Client only on Windows accounts that will install or launch games.**

1. Open the [latest release](https://github.com/GreenFuze/MyGamesAnywhere/releases/latest) and download **MGA Server Setup** (`mga-v…-windows-amd64-installer.exe`).
2. Run it and choose **For me only** for the simplest setup, or **All users** for an always-on household server.
3. Open MGA, create the first player, add one connection, and scan.
4. When a Windows account needs local installs or launches, use the Client control in MGA to download and pair **MGA Client Setup**.

Phones, tablets, and TVs only need a browser to browse MGA. Read the [Windows installation guide](https://greenfuze.github.io/MyGamesAnywhere/install.html) for LAN, portable, and Client setup.

## Before you try it

- MGA is Windows-first, pre-1.0 software intended for one PC or a trusted home LAN.
- Do not expose the current server directly to the public internet.
- MGA provides no games, ROMs, firmware, store entitlements, or DRM bypasses.
- Some connections need provider credentials, and some browser or emulator routes need setup.
- Keep a backup of the MGA data directory while the project is pre-1.0.

[MGA website](https://greenfuze.github.io/MyGamesAnywhere/) · [Screenshots](https://greenfuze.github.io/MyGamesAnywhere/screenshots.html) · [Compatibility](https://greenfuze.github.io/MyGamesAnywhere/integrations.html) · [Compare](https://greenfuze.github.io/MyGamesAnywhere/comparison.html) · [FAQ](https://greenfuze.github.io/MyGamesAnywhere/faq.html) · [Releases](https://github.com/GreenFuze/MyGamesAnywhere/releases)

<details>
<summary><strong>Building or contributing</strong></summary>

MGA contains a Go server, React web interface, plugin integrations, and the per-user Windows MGA Client. Start with [`AGENTS.md`](AGENTS.md) and [`docs/agent-bootstrap.md`](docs/agent-bootstrap.md).

Current product and architecture guidance lives in the [MGA Confluence space](https://greenfuzer.atlassian.net/wiki/spaces/MG/overview). Open work is tracked in [MGA Jira](https://greenfuzer.atlassian.net/jira/software/c/projects/MGA/boards/69/backlog).

</details>

MGA is pre-1.0 software under active development. See [`VERSION`](VERSION) and [`LICENSE.md`](LICENSE.md).
