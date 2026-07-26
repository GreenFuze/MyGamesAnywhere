# Public screenshot fixture

`publicdemo` creates a privacy-safe MGA database for public screenshots. It uses
one fictional profile, fictional games, generated SVG covers, and no external
credentials.

The command fails if the database or server config already exists. Always use a
new temporary directory; never point it at a real MGA data directory.

Example from `server/`:

```powershell
$demo = Join-Path $env:TEMP "mga-public-demo"

go run ./cmd/publicdemo `
  --db "$demo\data\db.sqlite" `
  --covers-dir "$demo\covers" `
  --server-config "$demo\config.json" `
  --app-dir ".\bin" `
  --port 8911

python -m http.server 8766 --directory "$demo\covers" --bind 127.0.0.1
.\bin\mga_server.exe --config "$demo\config.json" --app-dir ".\bin" --data-dir "$demo" --runtime-mode user --no-tray
```

Open `http://127.0.0.1:8911`, choose **Demo Player**, and use the bootstrap
password `changeme`. The first sign-in requires replacing it.

The cover server must be running before MGA starts so the media cache can fetch
the generated artwork.
