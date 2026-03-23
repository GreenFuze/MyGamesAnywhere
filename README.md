# MyGamesAnywhere (MGA)

![MyGamesAnywhere — title banner](docs/branding/title-text.png)

**MyGamesAnywhere** is a **local-first game library** for people who play across **PC installs, consoles, emulators, and cloud**. It runs a small **Go server** on your machine (Windows tray app today), scans **sources** you connect (Steam, Xbox, Epic, SMB shares, Google Drive, etc.), enriches titles with **metadata plugins** (IGDB, RAWG, LaunchBox, HLTB, …), and exposes everything through a **REST API** and a **web UI** (Phase 1 scaffold).

**Slogan:** *The shelf that follows your library. Play anywhere. Know what you have.*

---

## Who it’s for

- Collectors who want **one catalog** across Steam, console libraries, ROMs, and physical rips.
- Anyone tired of **remembering which PC or drive** had which game.
- People who want **achievements, time-to-beat, and cover art** without giving everything to a single storefront.

**Tone:** capable, calm, **privacy-respecting** (data stays local unless *you* sync), slightly playful but **not** childish — think “serious tool for people who love games,” not startup hype.

---

## Brand & logo brief (for your PNG work)

### What the mark should communicate

1. **Library / collection** — shelves, catalog, “everything in one place” (abstract is fine; avoid literal clipart shelves unless it’s exceptional).
2. **Play / anywhere** — optional nod to **controller, play glyph, cloud arc, or multi-device** — subtle; don’t overcrowd.
3. **Trust** — readable at small sizes; works on **dark and light** backgrounds.

### Name usage

- Full name: **MyGamesAnywhere**
- Short: **MGA** (tray, compact UI, icon)

### Color direction (not strict — your taste wins)

- **Primary feel:** deep **purple-charcoal / midnight** with **electric blue** accents (aligned with logo art — see **Midnight** in the app: ~`#0c0a10` bg, ~`#3db8ff` accent).
- Provide versions for **dark UI** (default) and optional **light / monochrome** for print or light headers.
- **Tray / favicon:** often **single-color or high-contrast silhouette** reads better than full gradients at 16×16.

### What to deliver (file checklist)

| Asset | Suggested spec | In this repo |
|--------|----------------|--------------|
| **README / marketing banner** | Wide PNG with title + slogan | [`docs/branding/title-text.png`](docs/branding/title-text.png) |
| **Web title strip** | Wide hero / wordmark (use **dark** banner; avoid solid magenta fill — that was a separate export) | [`server/frontend/public/title.png`](server/frontend/public/title.png) — Home & About (kept in sync with [`docs/branding/title-text.png`](docs/branding/title-text.png)) |
| **Optional empty bar hero** | Wide PNG with blank bar for custom text | [`server/frontend/public/title-bar.png`](server/frontend/public/title-bar.png) (unused by default) |
| **UI logo** | PNG **transparent** (emblem) | [`server/frontend/public/logo.png`](server/frontend/public/logo.png) — shell (home link), About |
| **Favicon** | Multi-size **ICO** (or PNGs) | [`server/frontend/public/favicon.ico`](server/frontend/public/favicon.ico) — see Vite [public assets](https://vitejs.dev/guide/assets.html#the-public-directory) |
| **Windows tray `.ico`** | **16×16**, **32×32**, **48×48**, … | [`server/cmd/server/mga.ico`](server/cmd/server/mga.ico) — **rebuild server** after changing |

**Converting PNG → ICO (examples):**  
[icoconvert.com](https://icoconvert.com), or ImageMagick:  
`magick convert icon-16.png icon-32.png icon-48.png mga.ico`

### Pitfalls to avoid

- Tiny text spelling “MyGamesAnywhere” in the **tray icon** — unreadable; use **symbol + “MGA”** or symbol only.
- Overly thin strokes — **blur at 16×16**.
- **TM/®** in the favicon — skip unless legally required in that context.

---

## Tech snapshot (for context only)

- **Server:** Go, SQLite, Chi, plugin **IPC** (length-prefixed JSON).
- **Frontend:** Vite + React + TypeScript under `server/frontend/`.
- **Build:** `server/build.ps1` → output under `server/bin/` (includes SPA in `bin/frontend/dist` when Node is available).

More detail: [`server/README.md`](server/README.md), [`server/frontend/README.md`](server/frontend/README.md), [`roadmap.md`](roadmap.md).

---

## License & contributing

*(Add your license and contribution notes when ready.)*

---

## Credits

- **Logo & tray icon:** Brand pack in `docs/branding/` and `server/frontend/public/` (see table above).

Branding is wired into the **web shell** (sidebar / mobile header), **Home** hero, **About**, and **favicon**; the tray uses the embedded **`mga.ico`**. Drop new exports into the same paths and rebuild.
