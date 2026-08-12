# PAGASA CLI

**Every PAGASA public weather surface as one agent-native CLI — synopsis, city forecast, and live cyclone signals, with a local history no PAGASA page keeps.**

PAGASA serves only the latest forecast as server-rendered HTML. `pagasa-pp-cli` extracts the synopsis, per-city 5-day forecasts, and active tropical-cyclone bulletins into structured JSON, links the bulletin PDFs and wind-signal maps from pubfiles, and mirrors each reading into a local SQLite store so you can query history and forecast drift offline. Table output in a terminal, JSON when piped — built for both humans and agents.

Created by [Nestor G Pestelos Jr](https://npestelos.com).

> **Unofficial.** This is an independent, community-built tool and is **not affiliated with, endorsed by, or supported by PAGASA or DOST**. It extracts publicly available data from the PAGASA website's HTML; the site's structure can change at any time and break extraction without notice. For official, authoritative forecasts and warnings, always consult [pagasa.dost.gov.ph](https://www.pagasa.dost.gov.ph) directly — **never rely on this tool for life-safety decisions**. Review PAGASA's terms of use before deploying automated access, and use a courteous request rate. The official [PAGASA TenDay JSON API](https://tenday.pagasa.dost.gov.ph) requires a token issued by PAGASA and is **not** used by this tool.

## Install

Requires [Go 1.26.5 or newer](https://go.dev/dl/):

```bash
go install github.com/ph-commons/pagasa-pp-cli/cmd/pagasa-pp-cli@latest
```

The binary installs to `$(go env GOPATH)/bin` (usually `~/go/bin`); make sure that's on your `PATH`.

Or use the one-shot installer (idempotent; verifies release `checksums.txt` SHA256 before extract; `go install` fallback retries the module checksum-DB fetch; installs to `~/.local/bin`):

```bash
curl -fsSL https://raw.githubusercontent.com/ph-commons/pagasa-pp-cli/main/scripts/install.sh | bash
```

An MCP server binary is also available for IDE/desktop agents:

```bash
go install github.com/ph-commons/pagasa-pp-cli/cmd/pagasa-pp-mcp@latest
```

Default transport is **stdio** (no listening socket). Streamable HTTP is
opt-in and fail-closed:

```bash
# loopback only; bearer required
export PP_MCP_TOKEN="$(openssl rand -hex 32)"
pagasa-pp-mcp --transport http --token "$PP_MCP_TOKEN"
# endpoint: http://127.0.0.1:7777/mcp
# clients must send: Authorization: Bearer $PP_MCP_TOKEN

# non-loopback bind (LAN/container) still needs a token
pagasa-pp-mcp --transport http --addr 0.0.0.0:7777 --allow-remote --token "$PP_MCP_TOKEN"
```

Without `--token` / `PP_MCP_TOKEN`, HTTP mode exits before listen. Without
`--allow-remote`, non-loopback `--addr` (including bare `:7777`) is rejected.

## Quick Start

```bash
# current synopsis + active storm (KIYAPO, position, movement)
pagasa-pp-cli now

# 5-day forecast for one city (temp range + rain chance + condition)
pagasa-pp-cli forecast --city "Metro Manila"

# active cyclone: synopsis, center position, intensity, movement, forecast track, wind signals by locality
pagasa-pp-cli storm --json

# how far is the storm from a point? (Mandaluyong shown)
pagasa-pp-cli approach --location 14.58,121.03

# is a wind signal in effect for my locality?
pagasa-pp-cli watch --area Mandaluyong
```

Add `--json` (or `--agent`) to any command for machine output; output is auto-JSON when piped.

## Commands

| Command | What it does |
|---------|--------------|
| `now` | Current synopsis and active tropical cyclone (name, category, issuance time). |
| `forecast --city <name>` | 5-day forecast for a selected city. `--list-cities` shows the 18 valid names; `--city` matches on a case-insensitive substring. |
| `storm` | Active cyclone: synopsis, center position/coordinates, intensity, movement, forecast track, bulletin PDF links, and per-locality wind-signal breakdown. Reports `active:false` when none is tracked. |
| `approach --location "lat,lon"` | Great-circle distance from a fixed point to the storm center parsed from the synopsis. |
| `watch --area <name>` | Wind-signal relevance for a locality: matches the bulletin HTML `wind_signals` table when possible (`signal` + `signal_matched`), otherwise honest "not confirmed" plus signal map / PDF links. Empty match ≠ confirmed clear. |
| `digest --city <name>` | Synopsis + a city's forecast + active-storm bulletins in one payload (pages fetched in parallel). Persists a local snapshot. |
| `history` | Past synopsis/cyclone snapshots recorded locally (PAGASA serves only the latest). Empty until you've run `digest`/`now`/`storm` over time. |
| `drift --city <name>` | How a city's forecast changed across recorded snapshots. Needs at least two snapshots for the city. |
| `obs` | Latest **Automated Weather Stations** table (temp, humidity, wind, precip, pressure, solar). Live read; optional `--station` filter; optional `--limit` for display count only. Host: `bagong.pagasa.dost.gov.ph`. |
| `obs --capture` | Same scrape **plus** write to local `aws_obs` and prune rows older than 14 days. Use from cron for a local series. `--station` still filters; **do not pass `--limit`** (rejected — capture stores the full matched set). |
| `obs history` | Local AWS series (empty until captures exist). Distinct from synopsis `history`. `--limit` / `--station` apply here for query. |

Run `pagasa-pp-cli --help` for the full command tree, including the raw page-extraction commands (`weather`, `climate`, `tropical-cyclone`) and the local-store utilities.

### AWS station series (local)

PAGASA's AWS page publishes **only the latest snapshot** per station (typically stamps every ~5–10 minutes). This CLI does not invent multi-hour history from a single request. To build a series:

```bash
# every 10–15 minutes (launchd/systemd/cron — not installed by install.sh)
pagasa-pp-cli obs --capture --agent
# optional: restrict which stations are scraped/stored
# pagasa-pp-cli obs --capture --station 98 --agent
pagasa-pp-cli obs history --station 98 --limit 48 --json
```

**`--limit` vs `--capture`:** `--limit` is for **live** `obs` display and for **`obs history`** queries only. Combining `obs --capture --limit N` is a hard error — capture must persist the full matched station set (prune remains table-wide). Put row caps on `obs history --limit`, not on capture.

Empty `obs history` after upgrade is expected until the first capture. Prune is irreversible (14-day default, capture path only). Global/non-PAGASA weather remains out of scope — use open-meteo.

## How it works

- **`www.pagasa.dost.gov.ph`** — server-rendered HTML. The synopsis lives in a `panel-body`; each of 18 cities is an accordion panel with a day-by-day forecast table; the severe-weather-bulletin page links the current cyclone's artifacts. The `internal/pagasa` package parses these into typed Go values (with tests).
- **`bagong.pagasa.dost.gov.ph`** — Automated Weather Stations table (`obs`). Separate host from the default BaseURL; typed `aws_obs` rows, not synopsis snapshots.
- **`pubfiles.pagasa.dost.gov.ph`** — the bulletin PDFs and wind-signal PNGs, served over plain HTTP.
- **Local SQLite mirror** — `now`, `storm`, and `digest` persist synopsis snapshots (`history` / `drift`). `obs --capture` persists station rows in `aws_obs`. Schema additions for `aws_obs` are additive (no one-way stamp bump). Nothing leaves your machine.

HTML extraction is inherently brittle: if PAGASA restructures a page, the matching extractor can break until updated. That's the trade-off for a site with no official public JSON API.

## Output & flags

Agent-native flags on every command: `--json`, `--select <fields>`, `--compact`, `--csv`, `--quiet`, `--dry-run`, `--agent`. Typed exit codes let agents self-correct without parsing error text.

```bash
pagasa-pp-cli forecast --city manila --json --select city,days
pagasa-pp-cli now --agent          # JSON + non-interactive in one flag
pagasa-pp-cli storm --dry-run      # show what would be fetched, no request
```

## Changelog

User-visible changes are tracked in **[CHANGELOG.md](CHANGELOG.md)** (Keep a Changelog). Unreleased work sits under `[Unreleased]` until the next `vX.Y.Z` tag; that section is also the source text for GitHub Release notes.

## Development

```bash
go build ./...
go test ./...
go vet ./...
```

CI (build, vet, test, govulncheck) runs on every push and PR. Dependabot keeps Go modules and Actions current.

## Sources & inspiration

Prior art in the Philippine-weather ecosystem: [pagasa-parser](https://github.com/pagasa-parser) (TCB PDF → JSON), [bagyo-api](https://github.com/edwardguevarra/bagyo-api) (cyclone REST API), and [ph-municipalities](https://github.com/ciatph/ph-municipalities) (municipality lists from PAGASA 10-day files). Scaffolded with the [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press).

## License

Apache-2.0. See [LICENSE](LICENSE).
