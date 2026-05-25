# MGA Desktop

Cross-platform Avalonia desktop client for [MyGamesAnywhere](../README.md) — a self-hosted game library manager.

## Stack

| Layer | Technology |
|-------|-----------|
| UI framework | [Avalonia UI](https://avaloniaui.net/) 12.0 (GPU-accelerated, cross-platform) |
| MVVM | [CommunityToolkit.Mvvm](https://learn.microsoft.com/en-us/dotnet/communitytoolkit/mvvm/) 8.4 — `[ObservableProperty]`, `[RelayCommand]` source generators |
| Reactive | System.Reactive 6 — SSE event streams, observable navigation |
| HTTP / SSE | Custom `MgaApiService` + `SseClient` in `MGA.Api` project |
| Theme | Avalonia `ResourceDictionary` swap at runtime — Midnight (dark) + Daylight (light) |
| Runtime | .NET 9 |

## Structure

```
desktop/
├── MGA.Desktop.sln
├── scripts/
│   ├── build.ps1                  # Build / run / publish helper
│   └── generate-api-client.ps1    # NSwag → MGA.Api/Generated/MgaApiClient.g.cs
└── src/
    ├── MGA.Api/                   # HTTP + SSE client (no Avalonia dependency)
    │   ├── Models.cs              # C# record types matching server JSON
    │   ├── MgaApiService.cs       # Typed API facade — throws MgaApiException on errors
    │   ├── MgaApiException.cs
    │   ├── SseClient.cs           # RAII SSE reader with auto-reconnect
    │   ├── SseEventBus.cs         # Rx Subject<SseMessage> router
    │   └── SseMessage.cs
    └── MGA.Desktop/               # Avalonia application
        ├── App.axaml(.cs)         # Manual service construction (no DI container)
        ├── Controls/              # Reusable UserControls
        │   ├── TitleBar           # Custom chrome: logo, drag region, min/max/close
        │   ├── Sidebar            # Collapsible nav with animated width
        │   ├── ToastHost          # Stacked toast notifications
        │   └── LoadingSpinner     # Animated arc spinner
        ├── Services/
        │   ├── AppConfigService   # %APPDATA%/MGA/config.json RAII reader/writer
        │   ├── ServerConnectionService  # RAII: HttpClient + SSE per server
        │   ├── ThemeService       # Swaps ResourceDictionary at runtime
        │   ├── NavigationService  # CurrentPage observable, NavigateTo()
        │   └── ToastService       # Fire-and-forget toast queue
        ├── Themes/
        │   ├── ThemeBase.axaml    # All brush/style resources (DynamicResource only)
        │   ├── Midnight.axaml     # Dark color tokens
        │   └── Daylight.axaml     # Light color tokens
        ├── ViewModels/            # MVVM ViewModels (one per page / component)
        └── Views/                 # AXAML views (auto-resolved by ViewLocator)
```

## Building

```powershell
# Build
dotnet build desktop/MGA.Desktop.sln

# Run
dotnet run --project desktop/src/MGA.Desktop

# Using the helper script
.\desktop\scripts\build.ps1 -Run
.\desktop\scripts\build.ps1 -Configuration Release -Publish
```

## First run

On first launch the app shows the **Onboarding** screen. Enter the URL of your running MGA server (e.g. `http://localhost:8900` or `http://tv2:8900`) and click **Connect**. The app tests connectivity before saving.

Config is stored at:
- Windows: `%APPDATA%\MGA\config.json`
- macOS: `~/Library/Application Support/MGA/config.json`
- Linux: `~/.config/mga/config.json`

## Architecture decisions

**No DI container** — `App.axaml.cs` manually constructs the service graph bottom-up (RAII order). Every service/ViewModel receives only what it needs via constructor injection.

**RAII throughout** — `SseClient` starts streaming in its constructor and stops on `DisposeAsync()`. `ServerConnectionService` owns the `HttpClient` and SSE subscription; both are released on `Dispose()`. `ViewModelBase` owns a `CompositeDisposable`; page ViewModels add their Rx subscriptions to it and it's cleaned up when navigating away.

**Theme system** — `Midnight.axaml` / `Daylight.axaml` define `Color` tokens. `ThemeBase.axaml` derives `SolidColorBrush` resources and all control styles from those tokens using `DynamicResource`. Switching themes replaces `App.Resources.MergedDictionaries[0]`; all `DynamicResource` bindings repaint automatically.

**ViewLocator** — Avalonia's `IDataTemplate` convention maps `FooViewModel` → `FooView` by namespace replacement. Every page ViewModel has a matching View; no manual registration needed.

## API client generation

The typed API client is generated from `server/openapi.yaml` via NSwag:

```powershell
.\desktop\scripts\generate-api-client.ps1
```

The generated file (`src/MGA.Api/Generated/MgaApiClient.g.cs`) is committed so the build never requires NSwag at CI time.

## Developer tooling — SuitCode

[SuitCode](https://suitcode.dev) is a **CLI tool** (not an MCP server) that provides authoritative repository intelligence — real import graphs and symbol resolution across the whole monorepo.

### Install / update

```powershell
winget install SuitCode.SuitCode    # Windows
brew install suitcode               # macOS / Linux
```

### Warm up at the start of every session

SuitCode indexes the repo and builds a symbol graph. Always run this first — other commands give shallow results without it:

```powershell
suitcode . warmup
```

### Useful commands for this project

| Goal | Command |
|---|---|
| Understand a file | `suitcode . explain-file --file desktop/src/MGA.Desktop/Services/ServerConnectionService.cs` |
| Find all callers of a symbol | `suitcode . references --symbol SwitchServer` |
| Pull context for a set of files | `suitcode . context --files "desktop/src/MGA.Api/MgaApiService.cs,desktop/src/MGA.Api/Models.cs" --budget 4000 --format text` |
| Import graph for a ViewModel | `suitcode . imports --file desktop/src/MGA.Desktop/ViewModels/MainWindowViewModel.cs` |

### Known limitations

| Area | Symptom | Workaround |
|---|---|---|
| Go files | `Symbols` is null even after warmup (`LspEnhanced: false`) | Use `explain-file` for Go source navigation |
| TypeScript imports | Cross-file import resolution is incomplete | Seed related files explicitly with `--files` |
| PowerShell pipe | Progress lines appear before JSON, breaking `ConvertFrom-Json` | Pipe to `Out-File` and then read the file, or use the Read tool directly on source files |
