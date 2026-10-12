---
name: update-docs
description: Create or update this project's documentation (README.md + docs/). If no docs exist it writes the full doc set from the code; otherwise it finds the source files that changed since the last docs sync and updates only the affected sections, then verifies every env var and HTTP route is documented. Use when the user runs /update-docs or asks to create, refresh, sync or audit the project docs.
argument-hint: "[full | readme | architecture | configuration | http-api | protocols | operations | development]"
disable-model-invocation: true
allowed-tools: Read, Grep, Glob, Write, Edit, Bash(bash .claude/skills/update-docs/scripts/inventory.sh:*), Bash(git diff:*), Bash(git log:*), Bash(git status:*), Bash(git show:*)
---

# Update project docs

Keep `README.md` and `docs/` accurate to the code of this Go matching engine (FIX acceptor, order books, ITCH feed, admin console, benchmarks).

Arguments passed: $ARGUMENTS

- *(empty)*: auto mode. Create the docs if they are missing, otherwise update what changed.
- `full`: re-audit every doc against the code, including files the manifest says are unchanged.
- a doc name from the table below: work on that doc only.

## Ground rules

1. **The code is the source of truth.** Every name, default, port, path, route, tag and message type in the docs must come from code you read during this run or from the inventory output. Never guess and never rely on memory of earlier runs. If the code doesn't settle a question, leave it out of the docs and list it under *Open questions* in the report.
2. **Never put secrets in docs.**
   - Do not open `me.env*`, `*.bak*`, `*.log` or the compiled binaries.
   - Never copy entries from the credential and broker stores (`fix_brokers.json`, `itch_brokers.json`, `fix_allow.json`, `broker_meta.json`, `brokers.tsv`). Describe each format with a made-up example instead.
   - The inventory prints some env var defaults as `<redacted>`. Document only the variable's name and the fact that a built-in default exists. List these variables under *Security notes* in `configuration.md` as "must be overridden in production", without the value.
3. **Hosts and IPs.** You may document loopback defaults and port numbers as they are. Some defaults are real public or private IPs or hostnames, such as the one for `ME_PUB_IP`. For those, write "deployment-specific default, see `<file>`". In examples, use placeholders such as `<me-public-ip>`.
4. **Preserve human writing.** When updating, edit the smallest section that is wrong. Keep accurate prose, wording and ordering. Don't delete a section unless you can tie it to code that was removed. Leave content the doc set doesn't define alone, such as extra files in `docs/` or extra sections, except to fix facts that are now wrong.
5. **Reference code by file and symbol**, such as `engine.go` and `(*Engine).process`, never by line number, because line numbers go stale.
6. **Touch only `README.md` and `docs/**`.** Don't change code and don't commit; the user reviews the diff.

## Doc set

| Doc | Covers | Primary sources |
|---|---|---|
| `README.md` | What the engine is, its feature list, a quick start (build, run, open the UI), the ports at a glance, and a link to every doc | `main.go`, `go.mod`, `config.go` |
| `docs/architecture.md` | Components and how they connect, shown in a Mermaid diagram. Also: the order lifecycle (FIX/HTTP order in → execution report and market data out), matching rules (priority, order types, TIF), trading sessions, RMS checks, bots, persistence, and the instrument universe with auto-listing | `engine.go`, `book.go`, `bus.go`, `bots.go`, `rms.go`, `persist.go`, `instruments.go`, `loader.go` |
| `docs/configuration.md` | Every env var (auto table), every data and config file with its format, the deployment directory layout, and security notes | `config.go`, every `envOr`/`os.Getenv` site, `*.json`, `brokers.tsv` |
| `docs/http-api.md` | Every HTTP route (auto table). Per route: method, auth, parameters, response shape and a curl example. Also the WebSocket streams, the SSE `/events` stream, and admin console login | `main.go`, `admin.go`, `instruments.go`, `itchgw.go`, `web.go`, `benchweb.go`, `bench_export.go` |
| `docs/protocols.md` | **FIX acceptor:** session settings, BeginString/ApplVerID, supported message types and key tags, reject reasons, auth, IP allowlist, message log. **ITCH feed:** SoupBinTCP login, message types, sequencing, auth | `fixacceptor.go`, `fixmsglog.go`, `itchfeed.go`, `itchauth.go`, `brokermeta.go`, `spec/` |
| `docs/operations.md` | Running and deploying: the env file, nginx TLS and the FIXS gateway (`nginx-443.patch`, `regen-fixs-gateway.sh`), admin console tasks (broker onboarding, allowlist), benchmark and live load tests, log and audit files, health checks, and troubleshooting | `admin.go`, `bench.go`, `live.go`, `latencyproxy.go`, `*.sh`, `*.patch` |
| `docs/development.md` | The source file index (auto table), build and test commands, code conventions, and how-tos for adding an env var, route, FIX message or instrument | all `*.go`, `*_test.go`, `go.mod` |

If a new source file doesn't fit any row, put it in the closest doc for what it does and say so in the report. If a section would have no code behind it (for example, there is no systemd unit in the repo), leave the section out rather than invent one.

## Auto-managed tables

These tables must always match the code exactly. Wrap each one in its markers:

| Markers | Doc | Columns |
|---|---|---|
| `<!-- AUTO:env-vars -->` … `<!-- /AUTO:env-vars -->` | configuration.md | Variable · Default · Purpose · Read in |
| `<!-- AUTO:config-files -->` … `<!-- /AUTO:config-files -->` | configuration.md | File · Env override · Format · Purpose |
| `<!-- AUTO:routes -->` … `<!-- /AUTO:routes -->` | http-api.md | Method · Path · Auth · Purpose |
| `<!-- AUTO:source-index -->` … `<!-- /AUTO:source-index -->` | development.md | File · Responsibility |

Rebuild these tables from the inventory on every run:
- Add rows for new items and remove rows for items no longer in the code.
- Fix changed defaults and locations.
- Keep the existing Purpose text for rows whose code didn't change.
- For the Method column, read the handler's `r.Method` checks. If the handler accepts any method, write `any`.

## Procedure

### 1. Inventory

From the repo root, run:

```bash
bash .claude/skills/update-docs/scripts/inventory.sh
```

The output lists:
- the existing docs and their headings
- source files changed since the last sync (compared with `docs/.sync-manifest`)
- each Go file's header comment and symbols
- env vars with their defaults (secret defaults redacted)
- HTTP routes with their auth
- listeners and outbound calls
- file paths referenced in the code
- the shape of each data/config file
- the tests

### 2. Pick a mode

- **Create**: there is no `README.md` and no `docs/*.md`. Go to step 3.
- **Adopt**: docs exist but `docs/.sync-manifest` doesn't, so the docs were written by hand or by another tool.
  1. Read all the existing docs first.
  2. Fit their content into the doc set. Move text rather than rewrite it, keep the author's wording, and add what is missing.
  3. Run step 4 treating every source file as CHANGED.
- **Update**: the manifest exists. Go to step 4. If the argument is `full`, treat every source file as CHANGED.
- **Already in sync**: the inventory reports `(no source changes)` and no argument was given. Run `--check` (step 5). If it passes, report that the docs are in sync and stop without rewriting the manifest.

### 3. Create

1. **Read before writing.** For each doc, read every file in its *Primary sources*. Read large files in chunks.
   - `admin_html.go`, `benchweb.go` and `web.go` are mostly embedded HTML. Skim them only for the APIs each page calls.
   - Read `go.mod` and the script headers.
   - For `spec/`, note only the file names; don't read the XML.
2. **Write the docs in this order:** configuration, http-api, protocols, architecture, operations, development, then README. README comes last so it can summarise and link to what exists.
3. **Shape each doc:** an H1 title, then one paragraph on what the doc covers, then the sections. Be concrete: give real ports, paths and command lines. Examples must run against the defaults, for example `curl -s localhost:8010/api/status`, and use only routes and parameters that exist in the code.
4. **Draw the architecture diagram.** `architecture.md` gets a Mermaid flowchart of the components and the calls that actually happen between them: FIX acceptor, HTTP API, engine, order books, bots, RMS, SSE bus, ITCH feed, ITCH WebSocket gateway, persister/Postgres, and the external OMS services.

### 4. Update

For each NEW, CHANGED or DELETED file in the inventory:

1. **See what changed.**
   - Run `git diff <manifest commit> -- <file>`, which covers both committed and uncommitted changes.
   - If the file is new or untracked, or the manifest commit is `none` or unknown to git, read the whole file.
   - If the file was DELETED, use `git show <commit>:<file>` when you need to know what it provided.
2. **Find the dependent doc text.** Grep `README.md` and `docs/` for the file name and for every symbol, env var, route, tag and path the diff adds or removes.
3. **Edit those sections.** One change can affect several docs. For example, a new env var in `fixacceptor.go` belongs in the `configuration.md` table and in the `protocols.md` prose.
4. **Handle removals and renames.** Delete or rewrite text about removed things, and apply renames everywhere.
5. **Rebuild the auto-managed tables** from the inventory.

Changes with no doc impact, such as refactors or comment edits, need no edit; list them as "no doc impact" in the report. If a doc from the doc set is missing entirely, create it using the step 3 rules and link it from README.

When the argument names a single doc, change only that doc. Fix only the `--check` gaps that belong to it, and don't write the manifest unless `--check` passes for the whole set.

### 5. Verify

1. Run `bash .claude/skills/update-docs/scripts/inventory.sh --check` and fix every `MISSING` item.
   - For each `POSSIBLY STALE` item, check the code. Remove it from the docs if it no longer exists. Keep it if it isn't an env var or route of this service, such as a FIX field or a path on another service; those are expected noise.
2. Re-read each section you edited against the code: names, defaults, ports, methods and auth.
3. Check that every relative link in `README.md` and `docs/` points to a file and heading that exist.
4. Grep the docs for anything that looks like a secret: `password=`, the redacted defaults, or entries from the credential files. There must be none.

Repeat until `--check` prints `RESULT: PASS`.

### 6. Record the sync

Only after `--check` passes, run:

```bash
bash .claude/skills/update-docs/scripts/inventory.sh --write-manifest
```

This records a hash of every source file in `docs/.sync-manifest`, which is the baseline the next run compares against. Commit it with the docs.

### 7. Report

Reply with:
- **Mode:** create, adopt, update, or already in sync.
- **Files** created or updated, with one line each on what changed.
- **No doc impact:** code changes that needed no doc change.
- **Open questions:** behaviour the code didn't make clear, so the user can answer and run the skill again.
- **Security notes:** hardcoded credential defaults and similar issues found, by name and file only, never values.
