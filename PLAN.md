# Plan: `bored` — a cross-project kanban for agentic development

## Context

Andrew wants a kanban board that he and Claude Code share, so that planning sessions break goals into doable tickets, agents can claim tickets and move them through states, and "what's next" is answered for him when he's distracted. It is explicitly **not** a per-repo artifact: one central board structures several initiatives that may span several git repos, or none yet.

Prior art researched: **Backlog.md** (markdown-per-task, frontmatter, terminal board, `--json`, human review gates) and **beads** (dependency graph, `bd ready`, atomic claim, `bd prime`). We build from scratch, borrowing the file format and board from the first and the agent loop from the second.

Decisions made with Andrew:
- Build from scratch. Go + Charm stack (Bubble Tea v2).
- One markdown file per ticket, YAML frontmatter, git-tracked.
- Central store, its own git repo. Default `~/.bored/`, override with `BORED_HOME`. Tool source lives at `~/src/bored` (empty today, not yet a git repo).
- Five states: `backlog → todo → doing → review → done`. `backlog` = raw idea; `todo` = refined with acceptance criteria, agent may claim. "Blocked" is derived from dependencies, never stored.
- Agents drive a CLI (`bored ... --json`) from Claude Code skills. No MCP server.
- **There is one construct: the ticket.** No projects, no epics, no types. Hierarchy comes from an optional `parent`. A ticket with children is a container, which is derived, not declared. A root ticket (no parent) is an initiative like "the IAM build" and can carry a short `slug` so it can be named in commands. Repos are registered paths; a ticket names a repo only once it is concrete code work.

## How we'd use it

**One-time setup.** `bored init` creates the store. `bored repo add h1v3 ~/src/h1v3` for repos that already exist. `bored install-skills` puts the four skills into `~/.claude/skills/`. Optionally a SessionStart hook so every Claude session opens knowing the board.

**You, capturing an idea.** From any shell: `bored new "Rate-limit the webhook endpoint" --parent iam`. Or `n` in the TUI. It lands in `backlog`. No ceremony, no acceptance criteria, no repo needed. With no parent it's a new root, which is also fine.

**Us, planning at the top.** From any directory, say "let's think about next steps for the IAM build". If there's no root for it yet, I create one: `bored new "The IAM build" --slug iam`. I load what's already under it (`bored tree iam`) so we build on the board rather than restart. Then it's a conversation: I ask a few targeted questions about goals, constraints and what's already decided, propose a split into conceptual chunks with rough ordering and dependencies, and you shape it. Nothing here mentions a path. The chunks become child tickets in `backlog`, and the reasoning goes into the root's body: the goal and constraints in Description, the options considered and the chosen split in Plan.

**Us, refining a chunk.** Later, `/bored-plan BRD-10` picks one of those chunks and does exactly the same thing one level down: break it into children with acceptance criteria, sized so one fits one session. Some children are still not code: "decide between Keycloak and Zitadel", "write the token lifecycle design". Those get acceptance criteria like "decision and reasoning recorded in this ticket" and no repo. Others are concrete, and that's the moment a repo gets attached: `--repo h1v3` at creation or `bored set 14 --repo h1v3` later. If the repo doesn't exist yet, a child ticket creates it and registers it. When a repo is attached and exists, I look at the code before proposing the split. The chunk BRD-10 now has children, so it drops off the board as a card and shows in `bored tree` with a `0/5` progress count instead.

**You, glancing.** `bored` opens the TUI board. It shows workable tickets, meaning those with no open children, across everything. `p` narrows to one root. `t` flips to the tree view to see structure and progress. `bored board` and `bored tree` print the same to the terminal. `bored next` or `/bored-next` says what to pick up and why, and flags anything sitting in `review` waiting for you.

**You, handing me work.** `/bored-work` (takes the next ready ticket) or `/bored-work 14`. If the ticket names a repo, I check I'm in it and refuse politely if not. If it doesn't, it's thinking work and I do it right here. I claim it, which marks it `doing` with me as assignee, write a short plan into the ticket if there isn't one, do the work (on a branch, for code), log progress and tick acceptance criteria as I go, and move it to `review` with a summary. For a decision ticket the summary is the decision. I never mark anything `done`. If I hit a blocker I log why and release it back to `todo`. If I find extra work, I file a new backlog ticket under the same parent rather than widening the one I'm on.

**You, reviewing.** Either look at the branch yourself and `bored done 14`, or `/bored-review 14` and I check the diff, or the written decision, against each acceptance criterion and recommend done or back-to-todo with notes. You make the final call. When the last child of BRD-10 is done, BRD-10 reappears on the board as workable, so "integrate and verify the chunk" is a natural last step rather than a special case.

**Me, in any session.** `bored prime` on startup tells me which repo this directory is, which roots have tickets pointing at it, what's in flight, what's ready, and what's next. I only touch tickets through the CLI, never by editing files, so every change is locked, validated and committed. `bored next` is also how I answer "what should I be doing" when you ask mid-session.

**Later, unattended.** A runner loops `bored next` into headless Claude sessions per ticket, each in its own worktree, and stops when nothing is ready. You come back to a `review` column.

## Verified environment facts (2026-10-09)

- Go 1.26.1 at `/usr/local/go/bin/go`. `gh`, `git`, `jq` present. `~/.claude/skills/` exists (only `synced/` inside; never name a skill `synced`). Andrew's `~/.claude/settings.json` has `defaultMode: "plan"`, so skills must declare `allowed-tools` for `bored` to run unprompted.
- Charm v2 moved to the `charm.land` module path (the `github.com/charmbracelet/*/v2` paths fail with a module-path mismatch). Pin:
  - `charm.land/bubbletea/v2` v2.1.0, `charm.land/lipgloss/v2` v2.0.6, `charm.land/bubbles/v2` v2.2.1, `charm.land/huh/v2` v2.0.3, `charm.land/glamour/v2` v2.0.1
- Bubble Tea v2 API deltas: `View() tea.View` (build with `tea.NewView(s)`; set `v.AltScreen = true`, `v.WindowTitle`); key events are `tea.KeyPressMsg` with `msg.String()` giving `"j"`, `"ctrl+c"`, `"enter"`, `"esc"`, `"space"`; `tea.WindowSizeMsg`; `tea.RequestBackgroundColor()` → `tea.BackgroundColorMsg{}.IsDark()`; `tea.ExecProcess(*exec.Cmd, func(error) tea.Msg)` for `$EDITOR`; `tea.WithAltScreen()` no longer exists.
- huh v2: `huh.NewForm(huh.NewGroup(fields...))`; `form.Update` returns `(huh.Model, tea.Cmd)` (type-assert back to `*huh.Form`); completion via `form.State == huh.StateCompleted | huh.StateAborted`.
- glamour v2 has **no `WithAutoStyle`**. Use `glamour.WithStyles(styles.DarkStyleConfig|styles.LightStyleConfig)` from `charm.land/glamour/v2/styles`, picking by `lipgloss.HasDarkBackground(os.Stdin, os.Stdout)` (CLI) or `BackgroundColorMsg.IsDark()` (TUI).
- YAML: `go.yaml.in/yaml/v3` (maintained successor of `gopkg.in/yaml.v3`, same API).
- Claude Code skills: `~/.claude/skills/<name>/SKILL.md`, frontmatter `name`, `description`, `argument-hint`, `allowed-tools`, `disable-model-invocation`; `$ARGUMENTS`/`$0`. SessionStart hook stdout is injected as context (10k char cap).

## 1. Store layout (`$BORED_HOME`, default `~/.bored/`)

```
~/.bored/
├── .git/
├── .gitignore        # .lock, *.tmp
├── .lock             # flock target
├── config.yaml       # id_prefix: BRD, actor: andrew, auto_commit: true, done_window_days: 7, editor: ""
├── repos.yaml        # repos: {h1v3: {path: /Users/andrewgibson/src/h1v3, branch: main}, ...}
└── tickets/BRD-1.md, BRD-2.md, ...
```

- Repos are just named paths. Which tickets belong to a repo is stored on the tickets. `bored repo detect` maps the current directory to a repo name.
- **IDs are sequential `BRD-<n>`.** The store is one directory on one machine, so there is no branch divergence to protect against with hashes; same-filesystem races are handled by `O_EXCL` under the store lock. Humans say "do 12". Input normalisation: `12`, `brd-12`, `BRD-12` all resolve. Anywhere an ID is accepted, a slug is accepted too.
- **Filename `tickets/<ID>.md` never changes.** Title and status live in frontmatter only. No per-status directories (git rename churn, unstable paths). Writes go to `tickets/.<ID>.md.tmp` then `os.Rename`. `List()` only accepts `^BRD-\d+\.md$`.
- No `delete` command in v1 (prevents ID reuse). `archive` later moves files, never removes.

## 2. Ticket schema

```markdown
---
id: BRD-12
title: Add OAuth login to admin panel
status: todo            # backlog | todo | doing | review | done
parent: BRD-10          # omitempty; absent = root ticket (an initiative)
slug: ""                # omitempty; short name usable wherever an ID is, unique across the store
repo: h1v3              # omitempty; registered repo name, set only when this is concrete code work
priority: 2             # 1 (highest) .. 4; default 3
depends_on: [BRD-11]    # blockers; blocked = any blocker not done
labels: [auth, backend] # free-form, multi-valued; use for bug, spike, decision, tech-debt, etc.
assignee: ""            # "" | andrew | claude
created: 2026-10-09T10:30:00Z
updated: 2026-10-09T12:00:00Z
claimed_at: 2026-10-09T12:00:00Z   # omitempty; cleared on move back to todo
branch: ""              # omitempty; set by agent on move to review
---

## Description

## Acceptance Criteria
- [ ] Login redirects to provider and back
- [x] Session cookie set with 7d expiry

## Plan
(optional; written by bored-plan / bored-work)

## Log
- 2026-10-09 12:00 claude: claimed
- 2026-10-09 12:41 claude: -> review: implemented handler; branch feat/oauth; commit abc123
```

Rules: frontmatter round-trips through a fixed-order Go struct (`omitempty` on optional fields). Body is stored **verbatim**. Only two structured body operations: `AppendLog(body, ts, actor, msg)` (creates `## Log` at end if missing; minute-resolution local time) and `CountAC(body)` / `CheckAC(body, n)` over the `## Acceptance Criteria` section.

Derived, never stored:
- **children** of a ticket, and whether it **has open children** (any child not `done`).
- **workable** = has no open children. Only workable tickets appear on the board or are offered by `next`/`ready`. A container becomes workable again when its last child is done, so "integrate and verify" falls out naturally.
- **blocked** = any `depends_on` not done.
- **scope** `--under X` = the subtree rooted at X (ID or slug), X included.
- **repo scope** = for a repo R, the roots of all tickets naming R; `next --repo R` offers tickets naming R plus repo-less tickets under those roots (thinking work in the same initiative).
- **progress** `done/total` over direct children, shown in `tree` and on the board.

## 3. Go package layout

Module `github.com/Commonwealth-Labs/bored`, Go 1.26.

```
cmd/bored/main.go              os.Exit(cli.Execute())
embed.go                       package bored; //go:embed skills   (root so embed can reach skills/)
internal/model/
  ticket.go                    Ticket, Status, consts, Validate(), ParseStatus()
  repo.go                      Repo{Name, Path, Branch}
  graph.go                     Index: NewIndex, Resolve(idOrSlug), Children, HasOpenChildren, Subtree, RootOf,
                               BlockedBy, IsBlocked, Workable, Ready, Next, WouldCycle, Progress
  transitions.go               CanClaim, CanMove, ApplyMove (pure)
internal/store/
  store.go                     Store{Root, Cfg}, Open(), Init(), resolveID()
  config.go  repos.go          yaml load/save; DetectRepo(cwd) via git toplevel / --git-common-dir
  ticket_io.go                 ParseTicket, RenderTicket, Load, Save (temp+rename), List
  body.go                      AppendLog, CountAC, CheckAC, EnsureSections
  ids.go                       allocateID() = max existing + 1, O_EXCL create, retry
  lock.go                      syscall.Flock on .lock, LOCK_NB retry 50ms up to 5s
  mutate.go                    Mutate(commitMsg, func(*Tx) error): lock → fn → save dirty → auto-commit → unlock
internal/git/git.go            IsRepo, Init, Commit(dir,msg), Push via os/exec
internal/render/               board.go (text columns, shared by CLI board/prime/TUI), tree.go, markdown.go (glamour factory), table.go
internal/cli/                  cobra: root.go, output.go (emit JSON|text, ExitError), actor.go, one file per command
internal/tui/                  app.go, board.go, tree.go, detail.go, form.go, keys.go, styles.go, msgs.go
skills/bored-plan|bored-work|bored-review|bored-next/SKILL.md
scripts/smoke.sh   testdata/   Makefile (build, install, test, smoke, install-skills)   README.md
```

Deps: `github.com/spf13/cobra` (≈20 commands with nested groups and a persistent `--json`; a hand-rolled dispatcher would be worse and the Charm deps dwarf it), `go.yaml.in/yaml/v3`, the five Charm v2 modules. Everything else stdlib. Git via `os/exec`, not go-git.

Core types:

```go
type Ticket struct {
    ID string `yaml:"id"`; Title string `yaml:"title"`; Status Status `yaml:"status"`
    Parent string `yaml:"parent,omitempty"`; Slug string `yaml:"slug,omitempty"`; Repo string `yaml:"repo,omitempty"`
    Priority int `yaml:"priority"`; DependsOn []string `yaml:"depends_on"`
    Labels []string `yaml:"labels"`; Assignee string `yaml:"assignee"`
    Created, Updated time.Time; ClaimedAt *time.Time `yaml:"claimed_at,omitempty"`
    Branch string `yaml:"branch,omitempty"`; Body string `yaml:"-"`; Path string `yaml:"-"`
}
type Scope struct{ Under string; Repo string; Labels []string }   // Under is an ID or slug
func (ix *Index) Workable(id string) bool                          // no open children
func (ix *Index) Ready(sc Scope, actor string) []*Ticket           // todo && workable && !blocked && (unassigned || actor)
func (ix *Index) Next(sc Scope, actor string) (*Ticket, string /*reason*/)
func (s *Store) Mutate(commitMsg string, fn func(tx *Tx) error) error
```

`Next` algorithm: if actor has a `doing` ticket → return it, reason "already in progress since …". Else candidates = `Ready(scope, actor)`; sort by priority asc, then number of dependents desc (unblocks most), then created asc. Reason like `P1, unblocked, unblocks 2 tickets, in todo 2d, this repo`.

## 4. CLI

Global flags: `--json`, `--home <dir>` (overrides `BORED_HOME`), `--as <actor>` on mutating commands (fallback `BORED_ACTOR` env, then `config.actor`). Scope flags `--under <id|slug>`, `--repo <name>`, `--label <l>` are accepted by every listing command. Wherever `<id>` appears, a slug is accepted.

```
bored                                    TUI on a TTY, else same as `bored board`
bored init [--prefix BRD]                mkdir, config, repos.yaml, .gitignore, git init, first commit
bored repo add <name> <path> [--branch main]
bored repo set <name> [--path p] [--branch b]
bored repo list | detect                 detect: registered repo containing $PWD (git toplevel or --git-common-dir for worktrees)
bored new <title> [--parent id] [--slug s] [--repo r] [--priority 1-4] [--depends-on id,id] [--label l]...
          [--status backlog|todo] [--description ..] [--ac ".."]... [--body-from-stdin] [--edit]
                                         prints new ID; --status defaults to backlog, or todo when any --ac given;
                                         --repo defaults to the parent's repo
bored set <id> [--title t] [--parent id|none] [--slug s] [--repo r|none] [--priority n]   field edits without $EDITOR
bored list [--status s] [--under x] [--repo r] [--label l] [--assignee a] [--all] [--json [--full]]
bored tree [x]                           subtree (default: all roots), indented, status + done/total per container
bored show <id> [--json] [--plain]       glamour on TTY, raw markdown otherwise; header shows parent chain and children
bored move <id> <status> [-m note]       any transition; ->todo clears assignee/claimed_at; ->done with open children = exit 4 unless --force
bored claim <id> [--as a] [--force]      todo->doing on a workable ticket, sets assignee+claimed_at, logs "claimed"; exit 4 otherwise
bored done <id> [-m note] [--force]      review->done (human gate); --force allows from doing
bored next [--under x] [--repo r] [--as a]   one ticket + reason; exit 5 if none
bored ready [--under x] [--repo r]       all ready tickets, sorted as next
bored log <id> <message> [--as a]
bored ac <id> check|uncheck <n>
bored edit <id>                          $EDITOR; re-parse + validate; abort save if invalid
bored dep add|rm <id> <blocker-id>       add does cycle check -> exit 4
bored dep list <id>
bored board [--under x] [--repo r] [--all-levels]   workable tickets in columns; done windowed by done_window_days
bored prime                              workflow instructions + board summary + next, for agent context; detects repo from $PWD
bored sync [-m msg] [--push]             commit outstanding store changes (hand edits)
bored install-skills [--dir ~/.claude/skills] [--symlink]
bored version
```

Exit codes: `0` ok · `1` I/O/git failure · `2` usage/validation (including duplicate slug) · `3` not found · `4` conflict (bad transition, already claimed, cycle, open children, not workable) · `5` nothing to do (`next`/`ready` empty, so a runner loop stops cleanly).

JSON: tickets serialise as the frontmatter fields plus derived `blocked`, `blocked_by`, `workable`, `children`, `progress: {done, total}`, `ac: {done, total}`, `root`, `path`, and `body` on `show`/`list --full`. `next` → `{"ticket": {...}|null, "reason": "..."}`. `board` → `{"backlog": [...], ..., "done": [...]}`. `tree` → nested `{ticket, children: [...]}`. Errors with `--json` go to stderr as `{"error": "...", "code": N}`.

`prime` output (well under the 10k hook cap):

```
# bored — cross-project kanban (store: ~/.bored)
This directory: repo h1v3 (/Users/andrewgibson/src/h1v3). Initiatives with work here: iam (BRD-1)
Workflow: backlog -> todo (has AC, agent-ready) -> doing (claim) -> review (human gate) -> done
Use the CLI, not direct file edits: bored next --json | bored claim ID --as claude | bored log ID ".." | bored move ID review -m ".."
Skills: /bored-next /bored-work /bored-review /bored-plan
## Board (under iam; * = this repo)
doing:  BRD-12* (claude) Add OAuth login
review: BRD-9* Fix flaky test
todo (ready): BRD-14* P2 ..., BRD-15 P3 Decide token lifetimes   | blocked: BRD-16* (by BRD-14)
Next: BRD-14 — P2, unblocked, unblocks 1 ticket, this repo
```

## 5. Concurrency and git

- `Mutate`: flock `.lock` → run fn → write dirty tickets temp+rename (sets `updated`) → if `auto_commit` and `git status --porcelain` non-empty: `git -C root add -A && git commit -q -m msg` → unlock. Holding the lock across the commit avoids racing git's own `index.lock` between two agents.
- Commit messages: `bored: new BRD-13 "Add OAuth login"`, `bored: move BRD-12 todo->doing (claude)`, `bored: log BRD-12`, `bored: dep BRD-16 blocked-by BRD-14`, `bored: sync`.
- `--no-commit` on mutating commands for batch creation in `bored-plan`, followed by one `bored sync -m "plan: BRD-10"`.
- Slug uniqueness is checked inside the same transaction as the write.

## 6. Claude Code skills (`skills/*/SKILL.md`)

Common frontmatter: `name`, `description` (lead with trigger phrases), `allowed-tools: Bash(bored *) Bash(git *) Read Grep Glob`, `argument-hint`.

- **`bored-plan` `[id | slug | topic]`** — Works from any directory. One procedure at any depth: *decompose this ticket into children*.
  1. Resolve the target. An ID or slug → that ticket. Free text → ask whether it's a new initiative (then `bored new "<title>" --slug <s>` as a root) or belongs under an existing one (`bored tree` to choose).
  2. Load context: `bored show <target>` and `bored tree <target>`, so planning builds on what's already there. If the target or its ancestors name a repo that exists on disk, explore it briefly. Otherwise ask a few targeted questions about goals, constraints and what's already decided.
  3. Propose 3–8 children with priority and `depends_on` between them. At the top of a tree they are conceptual chunks without acceptance criteria, destined for `backlog`. Lower down they get 2–5 acceptance criteria each and go to `todo`, sized "one ticket = one context window = one PR". Thinking work (decide, design, spike) gets AC like "decision recorded in this ticket" and no repo. Code work gets `--repo`. If a needed repo doesn't exist, add a child "create repo X at path and `bored repo add` it".
  4. Show the proposal as a table and wait for confirmation. Create via `bored new ... --parent <target> --no-commit`, then `bored sync`. Write the reasoning (goal, constraints, options considered, chosen split) into the target's Description and Plan via `bored edit` or `bored log`.
  5. Never claims or starts work.
- **`bored-work` `[id]`**, `disable-model-invocation: true` — `ID=$0` or `bored next --as claude --json` (stop on exit 5). `bored show ID --json`. If the ticket has a `repo`, verify `bored repo detect` matches it; on mismatch print the expected path and stop, never `cd` elsewhere. If it has no repo, it's thinking work: do it in place and record the output in the ticket via `bored log` and the Plan section. `bored claim ID --as claude` (exit 4 → report why, stop). Write a Plan if empty. Work; `bored log` at checkpoints; `bored ac check` as criteria pass. On completion: for code, tests pass, commit on a branch, `bored move ID review -m "summary; branch X; commit Y"`; for a decision, `bored move ID review -m "decision: .."`. Never moves to `done`. On blocker: log reason and `bored move ID todo -m ".."`. Extra work found: `bored new ... --parent <same parent> --status backlog`.
- **`bored-review` `[id]`** — `bored list --status review --json` or `$0`. For a repo ticket, `git diff main...<branch>` in that repo; for a thinking ticket, read the recorded decision. Check each AC. Verdict: `bored done ID -m "reviewed: .."` or `bored move ID todo -m "needs: .."`. Asks before `done` unless told auto-approve.
- **`bored-next` `[id | slug]`** — `bored next --json`, `bored ready --json`, `bored list --status doing --json`, `bored list --status review --json`, scoped with `--under` if given. Answers in 3–6 lines: the pick, why, what it unblocks, what is in doing/review that may need attention first. No mutations.

SessionStart hook (documented in README, optional):

```json
{"hooks": {"SessionStart": [{"matcher": "startup|resume|clear|compact",
  "hooks": [{"type": "command", "command": "command -v bored >/dev/null && bored prime 2>/dev/null || true"}]}]}}
```

## 7. TUI (Bubble Tea v2)

Two views. **Board**: five columns sized from `tea.WindowSizeMsg`, showing workable tickets; card = `BRD-12 P2 iam › h1v3` (root slug, repo if set), wrapped title, `AC 2/5`, `blocked` badge. **Tree**: indented hierarchy with status and `done/total` per container, cursor on any node. Status bar; `?` help.

Keys: `j/k` row, `h/l` column, `g/G`, `enter` detail, `esc`/`q` back/quit, `t` toggle board/tree, `[`/`]` move ticket left/right (confirm only for →done), `n` new (huh form: title, parent, repo, priority, description; parent defaults to the current root filter or the cursor's parent), `c` claim as `config.actor`, `d` done (from review), `e` `$EDITOR` via `tea.ExecProcess`, `m` log message prompt, `/` filter, `p` cycle root filter (all → each root by slug or title), `r` reload.

Model: `App{store, tickets, index, view (board|tree), cols [5][]*Ticket, treeRows []*node, col, row, mode (nav|detail|form|filter|prompt), scope Scope, detail{id, viewport}, form *huh.Form, input textinput, glam, dark, width, height, status, lastMod}`. `Init` batches `loadTickets`, `tea.RequestBackgroundColor`, and a 3s `tea.Tick` that stats `tickets/` and reloads only if the newest mtime changed (agents mutate while the board is open). Every mutation runs in a `tea.Cmd` and re-lists from disk afterwards. `View()` returns `tea.NewView(content)` with `AltScreen = true`. Detail = lipgloss header (parent chain, children with status) + glamour body in a `viewport`. No mouse, no drag, no multi-select in v1.

## 8. Phases (each shippable)

1. **Scaffold + store + CLI.** `git init` the tool repo, `go mod init`, Makefile (`install` → `go install ./cmd/bored` into `~/go/bin`). Build in order: model → store (config, repos, parse/render, ids, lock, mutate) → git → commands: `init`, `repo`, `new`, `set`, `list`, `tree`, `show` (plain first, glamour second), `move`, `claim`, `done`, `log`, `dep`, `ready`, `next`, `board`, `edit`, `sync`, `ac`. Dogfood at once: `bored new "bored" --slug bored`, `bored repo add bored ~/src/bored`, and file phases 2–3 as children.
2. **Agent integration.** `prime`, the four skills, `embed.go`, `install-skills` (`--symlink` for live editing), README with the hook. Test each skill in a real session against a scratch repo, including a root-level planning conversation with no repo at all.
3. **TUI.** Board and tree views per §7; `bored` with no args launches it on a TTY. Add `archive` and the done window.
4. **Headless runner (future, not in this plan).** `bored run [--under x] [--repo r] [--max N] [--budget-usd X]`: loop `bored next --as claude --json` (exit 5 stops) → `cd repo.path` if the ticket has one → `claude -p "/bored-work BRD-N" --output-format json --allowedTools "Bash(bored *),Read,Edit,Write,Bash(git *)" --permission-mode acceptEdits --max-turns 60 --max-budget-usd X`, optionally in a per-ticket `git worktree`; if the ticket is not in `review` afterwards, log and move to `todo`. Needs decisions on permission mode and worktrees.

## 9. Verification

- `go test ./...` with `t.TempDir()` as `BORED_HOME`:
  - store: parse/render round-trip against golden files in `testdata/` (body byte-identical, field order stable); missing optional fields; `AppendLog` creates section; `CountAC`; concurrent `allocateID` (50 goroutines → 50 unique IDs); atomic save leaves no `.tmp`; `Mutate` commits (skip if git absent); duplicate slug rejected.
  - model: `Resolve` by ID and slug; `Children`/`HasOpenChildren`/`Workable`; `Subtree` and `--under` scoping; repo scope includes repo-less tickets under the same roots; `BlockedBy`; `Ready` excludes blocked, non-workable, and assigned-to-others; `Next` prefers in-progress then priority/dependents/age; `WouldCycle` for both `depends_on` and `parent`; transitions (claim only from todo and only when workable; done from review; →done refused with open children; →todo clears assignee).
  - cli: cobra command tests on a temp store asserting JSON output and exit codes (`next` → 5, `claim` twice → 4, claim a container → 4, unknown id → 3, duplicate slug → 2).
- `scripts/smoke.sh` (`make smoke`): fresh `BORED_HOME`; `init`; `repo add` pointing at a temp git repo; root with slug; two chunks under it; three leaves under one chunk with `--ac` and `--depends-on`, one with `--repo` and one without; assert `board` hides the chunk with children and `tree` shows `0/3`; `repo detect` from inside the temp repo; assert `ready` excludes the blocked one; `next --json | jq .ticket.id`; `claim --as claude`; `log`; `move review`; `done`; assert dependent becomes ready; finish all three leaves and assert the chunk is now workable and `done` on it succeeds; `prime`; commit count in `git -C $BORED_HOME log` matches mutation count; exit codes 2/3/4/5.
- Skills: run `/bored-plan` on a fresh topic (no repo), then on a chunk with a repo; `/bored-work` on a thinking ticket and on a code ticket; `/bored-review`; `/bored-next`. Confirm `allowed-tools` lets `bored` run without prompts under `defaultMode: plan`.
- TUI manual checklist: resize, navigation across empty columns, `t` toggles tree, `[`/`]` updates file and commits, `n` creates under the right parent, `c` claims, `/` filter, `p` cycles roots, `e` round-trips the editor, an external `bored move` is picked up by the poll, light and dark terminals.

## 10. Open questions and risks

- **Charm v2 is fresh** (bubbletea v2.1.0 released 2026-10-08). Pin exact versions; keep TUI code thin. huh v2.0.3 declares bubbletea v2.0.2 but MVS will select v2.1.0; run the huh example once to confirm.
- **Container status is stored, not derived.** A root in `backlog` with children in `doing` is a little odd. Deliberately left alone in v1: containers don't show on the board, and the human moves a root to `doing` when they like. If it annoys, derive a display status from children later.
- **A perpetual stream** like "maintenance" is a never-closing root. Harmless; it just never leaves `doing`.
- **Several Claude sessions all `--as claude`** could be handed the same "already in progress" ticket by `next`. Option for phase 2: actor `claude@<short-session-id>` when `$CLAUDE_SESSION_ID` is available.
- **`next` across roots** may bounce between initiatives on pure priority. Start with `--under` filtering; add a `focus` config only if it bites.
- **Direct file edits** bypass `updated` and auto-commit. Skills say "prefer CLI"; `bored sync` and the TUI poll tolerate it.
- **Worktrees**: `repo detect` must compare `git rev-parse --git-common-dir`, not just `$PWD`.
- **Reparenting** (`bored set --parent`) can create a cycle; same check as `dep add`.
- **Root-level thinking may outgrow a ticket body.** If planning conversations produce long documents, add a `notes/` directory in the store that tickets link to. Not in v1.
