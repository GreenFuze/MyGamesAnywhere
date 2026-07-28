# Current screenshot evidence

The `*-current.png` files are the current MGA-44 capture set. They were captured
on 28 July 2026 from source commit `cb4018b8` using the isolated
`server/cmd/publicdemo` fixture. The Library, game hero, and copies/files images
are published in the public gallery.

The fixture contains one fictional player, eight fictional games, and original
privacy-safe artwork committed with the generator. It contains no personal
accounts, real connection credentials, or commercial game art. The fixture
never fakes Client or installed-game state.

The current Play image is diagnostic evidence, not a public marketing image. It
shows a real device/user grant but a disconnected Client because a second
`mga://` invocation is stopped by the single-instance mutex before forwarding
the request. That behavior is tracked by MGA-71. The image remains out of the
gallery until a packaged Client proves the handoff and shows the connected
state honestly.

Verification before capture:

```text
go test ./cmd/publicdemo
go test ./...
npm run test:unit
npm run build
git diff --check
```

Capture surfaces:

- `library-current.png`: library shelves, game artwork, source/play/save badges.
- `play-current.png`: current Play diagnostic, intentionally withheld from the
  public gallery pending MGA-71.
- `game-current.png`: game hero and primary play route.
- `game-copies-current.png`: two independent store copies, player ownership,
  save handling, and connection actions.

The Library image is cropped only above the page navigation so the public image
focuses on the library and does not advertise a disconnected Client state that
is unrelated to the library proof. No product content, status, or game data was
altered.

SHA-256:

```text
630C1D21FE471A7477E77DEF71F7075A4025126770263CA19CF530A7D5442E1E  library-current.png
BAA9C3AD0EB3491D540768A09C27F780DB50ADCA853C6C57B51A40C9472D975F  play-current.png
319ACA6E6740A7B090D5E9C324B94073186529DC772867866934ACD3CBD1D226  game-current.png
85BEDBCEE36D943DAB8AAD83C024072683E65ADA851DC6D20BD5C3FD1421586D  game-copies-current.png
```

`NO_MIGRATION_NEEDED`: these files are static public evidence and fixture-only
artwork. They do not change production persistence, configuration, identity, or
protocol behavior.
