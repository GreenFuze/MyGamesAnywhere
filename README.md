# MyGamesAnywhere (MGA)

**MyGamesAnywhere** is a **local-first game library** for people who play across **PC installs, consoles, emulators, and cloud**. It runs a small **Go server** on your machine (Windows tray app today), scans **sources** you connect (Steam, Xbox, Epic, SMB shares, Google Drive, etc.), enriches titles with **metadata plugins** (IGDB, RAWG, LaunchBox, HLTB, …), and exposes everything through a **REST API** and a **web UI** (Phase 1 scaffold).

**Tagline ideas (pick one or riff):**  
*Your games, one shelf.* · *Play anywhere — know what you have.* · *The shelf that follows your library.*

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

- **Primary feel:** deep **charcoal / midnight** with a **confident accent** (electric blue, teal, or violet-gold — see our **Midnight** theme in the app: `#0f1115` bg, `#4c8dff` accent).
- Provide versions for **dark UI** (default) and optional **light / monochrome** for print or light headers.
- **Tray / favicon:** often **single-color or high-contrast silhouette** reads better than full gradients at 16×16.

### What to deliver (file checklist)

| Asset | Suggested spec | Where to put it (after you export) |
|--------|----------------|-------------------------------------|
| **Master logo** | PNG, **transparent**, ~**1024×1024** or vector master → export | Keep in your repo or design folder; we use downscaled copies below |
| **Web / UI logo** | PNG **256–512px** wide, transparent | `server/frontend/public/logo.png` (then wire in About / shell — optional for now) |
| **Favicon** | **32×32** and **192×192** PNG *or* replace `public/favicon.svg` | `server/frontend/public/` — see Vite [public assets](https://vitejs.dev/guide/assets.html#the-public-directory) |
| **Windows tray `.ico`** | **Multi-size ICO**: **16×16**, **32×32**, **48×48** (some tools add 256×256) | Replace **`server/cmd/server/mga.ico`** (embedded at compile time — rebuild server after swap) |

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

- **Logo & tray icon:** *Your name here after you drop the assets.*

When your PNGs are in place, open a PR or ping the maintainer to hook **`logo.png`** into the About page and swap **`mga.ico`** — paths above are the contract.
