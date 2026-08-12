---
name: pp-pagasa
description: "Every PAGASA public weather surface as one agent-native CLI — synopsis, city forecast, and live cyclone signals, with a local history no PAGASA page keeps. Trigger phrases: `philippine weather`, `pagasa forecast`, `is there a storm`, `weather signal in my area`, `use pagasa`, `run pagasa`."
version: "1.4.0"
author: "Nestor G Pestelos Jr"
license: "Apache-2.0"
argument-hint: "<command> [args] | install cli|mcp"
allowed-tools: "Read Bash"
metadata:
  openclaw:
    requires:
      bins:
        - pagasa-pp-cli
    install:
      - kind: go
        bins: [pagasa-pp-cli]
        module: github.com/ph-commons/pagasa-pp-cli/cmd/pagasa-pp-cli
---

# PAGASA — Printing Press CLI

Structured JSON over PAGASA HTML (synopsis, city 5-day, cyclone bulletins) + local SQLite history. **No auth.** Read-only — never use for create/update/delete/publish/send.

## Install / verify

```bash
which pagasa-pp-cli || true
pagasa-pp-cli --version
```

Missing binary (Go ≥1.26.5; `GOTOOLCHAIN=auto` if older):

```bash
go install github.com/ph-commons/pagasa-pp-cli/cmd/pagasa-pp-cli@latest
# or prebuilt: curl -fsSL https://raw.githubusercontent.com/ph-commons/pagasa-pp-cli/main/scripts/install.sh | bash
```

Ensure `$GOPATH/bin`, `$HOME/go/bin`, or `~/.local/bin` is on `PATH`. **Do not run skill commands until `--version` works.**

MCP: `go install .../pagasa-pp-mcp@latest` → `claude mcp add pagasa-pp-mcp -- pagasa-pp-mcp` → `claude mcp list`. Relocate via host env `PAGASA_HOME` (MCP does not inherit CLI flags).

## Anti-triggers

| Need | Use instead |
|------|-------------|
| Global weather | open-meteo |
| Official TenDay JSON API | PAGASA token this CLI does **not** have |
| PH AWS station latest / local series | this CLI `obs` / `obs history` (not open-meteo) |

## Core commands

| Job | Command |
|-----|---------|
| Morning briefing | `pagasa-pp-cli digest --city "Metro Manila" --json --select synopsis,city_forecast` |
| Active cyclone (position, intensity, movement, forecast, signals) | `pagasa-pp-cli storm --json` |
| Distance to home | `pagasa-pp-cli approach --location 14.58,121.03 --json` |
| Locality signal state | `pagasa-pp-cli watch --area Mandaluyong --json` |
| Storm evolution | `pagasa-pp-cli history --limit 10 --json` |
| Forecast change | `pagasa-pp-cli drift --city "Metro Manila" --json` |
| Station observations (AWS latest) | `pagasa-pp-cli obs --station 98 --json` or `--station "Science Garden"` |
| Capture station row for local series | `pagasa-pp-cli obs --capture --agent` (cron; empty history until this runs) |
| Local AWS series | `pagasa-pp-cli obs history --station 98 --limit 24 --json` |
| NL → command | `pagasa-pp-cli which "<capability>"` (exit 0 = match, 2 = none → `--help`) |
| Health | `pagasa-pp-cli doctor` · `doctor --fail-on warn` |

**Grain note:** `history` = synopsis/cyclone snapshots. `obs history` = station AWS series. PAGASA AWS page is latest-only; series = scheduled `--capture`.

### Link scrapes (when needed)

- `climate` — 10-day climate forecast links
- `tropical-cyclone` — severe-weather-bulletin links
- `weather list-weather` · `weather list-weather-outlook-selected-philippine-cities`

### Family-safety: signal **number** (hard rule)

**`watch` empty / `signal_matched:false` means "not confirmed," not "confirmed clear."** Area names in the bulletin may not match the query string.

`storm --json` and `watch --json` expose `wind_signals` from the bulletin HTML Wind Signal table: `[{"signal": 3, "affected_areas": "Luzon: Cagayan, ..."}]`. Prefer `watch --area X --json`: when `signal_matched` is true, use `signal`. When false but `storm_active`, grep `wind_signals[].affected_areas` for the locality/province, or fall back to the highest-numbered `TCB#N_*.pdf` in `bulletin_pdfs` (page 2 TCWS table). Empty `wind_signals` can mean no cyclone **or** active system with "No Tropical Cyclone Wind Signal" — **never** treat that alone as confirmed clear. Family-safety: never stop at a bare "clear" note without checking `storm` / PDF when a cyclone is active.

## Agent mode

`--agent` ≡ `--json --compact --no-input --no-color --yes`. Prefer for agents.

- stdout JSON, stderr errors
- `--select path,nested` trims verbose payloads
- `--dry-run` preview; multi-page novel commands fetch pages in parallel
- Offline: local SQLite when available

**Envelope** (store/API reads):

```json
{"meta": {"source": "live|local", "synced_at": "...", "reason": "..."}, "results": <data>}
```

Parse `.results`; trust `.meta.source`. Human “N results (live)” only on TTY without machine-format flags.

## Paths

| Lever | Role |
|-------|------|
| `--home <dir>` | Single invocation |
| `PAGASA_HOME` | All four kinds under one root (fleet durable) |
| `PAGASA_{CONFIG,DATA,STATE,CACHE}_DIR` | Per-kind override |

Order: per-kind env → `--home` → `PAGASA_HOME` → XDG → defaults.  
Per-kind env **beats** `--home` for that kind. Clearing `PAGASA_HOME` does not move files — relocate manually or `doctor` misses credentials.

Kinds: `config` (config.toml, profiles) · `data` (credentials.toml, data.db) · `state` (jobs, teach.log) · `cache` (HTTP).  
`agent-context` → schema v4 `paths`.

## Learning loop (judgment only)

CLI journals itself. Agent: **recall → act → teach/amend**. Never hand-record failures.

1. **`recall "<question>" --agent`** before discovery (skip rest of session if store empty: no learnings/playbooks/candidates).
2. Order: `candidates` → `playbook` → `notes` → `results[0]` → warnings.
   - **Candidates:** try-then-confirm. Trial first, then `learnings confirm <id>` only after verified; `learnings reject <id>` if wrong. NEVER re-teach a candidate as a duplicate teach.
   - **Playbook:** read `notes` first; replay `steps` with `slots_resolved`; budget = `expected_tool_calls`.
   - **Exact match** conf ≥2 → skip discovery, fetch resource IDs. **Partial** → hint only.
3. **`teach ... &`** after cold-start resolution (background). Leaf resource IDs; strip PII from queries.
4. Optional: playbook flags on `teach` / `teach-playbook`; `playbook amend --add-note` for observed CLI/API gotchas only (no per-user answers, no PII/paths/keys).
5. Disable: `--no-learn` or `PAGASA_NO_LEARN=true`. Stats: `learnings stats`.

Older binary without `learnings confirm` → ignore candidates branch.

## Feedback / deliver / profiles

```bash
pagasa-pp-cli feedback "one-line surprise"   # local feedback.jsonl unless endpoint + --send
pagasa-pp-cli climate --deliver file:/tmp/out.json
pagasa-pp-cli profile save briefing --json
pagasa-pp-cli --profile briefing climate    # explicit flags > profile > defaults
```

Deliver sinks: `stdout` · `file:<path>` · `webhook:<url>`.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | OK |
| 2 | Usage |
| 3 | Not found |
| 5 | Upstream API |
| 7 | Rate limit |
| 10 | Config |

## Argument routing

| `$ARGUMENTS` | Action |
|--------------|--------|
| empty / help / `--help` | `pagasa-pp-cli --help` |
| `install` … `mcp` | MCP install above |
| `install` … | Prerequisites |
| else | `pagasa-pp-cli <args> --agent` · ambiguous → `<cmd> --help` |

HTTP: Chrome-compatible client; no resident browser required.
