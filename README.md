# bored

A cross-project kanban board for you and your coding agents. One markdown
file per ticket in a central, git-tracked store. Tickets form a tree; a
ticket with open children is a container and stays off the board until its
children are done. Every command takes `--json` for agents.

See `PLAN.md` for the design and the phased plan.

## Install

```
make install        # go install ./cmd/bored into ~/go/bin
bored init          # creates ~/.bored (override with BORED_HOME)
```

## Shape of the data

```
~/.bored/
├── config.yaml     # id_prefix, actor, auto_commit, done_window_days, editor
├── repos.yaml      # named paths on disk; tickets reference them by name
└── tickets/BRD-1.md ...
```

A ticket:

```markdown
---
id: BRD-12
title: Add OAuth login
status: todo            # backlog | todo | doing | review | done
parent: BRD-10          # optional; absent = a root (an initiative)
slug: ""                # optional; usable wherever an id is
repo: h1v3              # optional; set only when this is concrete code work
priority: 2             # 1 (highest) .. 4
depends_on: [BRD-11]
labels: [auth]
assignee: ""
---

## Description
## Acceptance Criteria
- [ ] ...
## Plan
## Log
- 2026-10-09 12:00 claude: claimed
```

## Containers

A ticket with children is a container. While any child is open:

- its status is **derived** from the children: backlog until one is ready,
  todo once one is, doing once any has started or finished. The stored
  status is ignored and `move`/`done` on it are refused.
- it stays off the board; the tree shows it with a done/total count.

When the last child is done, what happens to the parent depends on what it
says about itself. Still in backlog, it was never touched: it closes
automatically if it has no acceptance criteria (it was only a grouping, and
the same check runs on its parent) or becomes a todo ticket on the board if
it has (that is its own work, now unblocked). Both leave a log line. If you
had moved it to todo, doing or review yourself, which is only possible while
it has no open children, it stays where you put it and is simply workable
again; closing it is your call.

## Stuck tickets

When an agent gives up on a ticket, or a headless session ends without
handing over, the ticket goes back to todo with a `stuck` label and the
reason in its log. Stuck tickets are skipped by `next` and `ready`, so no
agent retries one until a human has looked. On the board the title is
amber, the column header counts them, and the details line shows the reason.
Clear it with `bored label rm <id> stuck`, or `bored claim <id> --force`,
which clears it because claiming means you've read it.

## Everyday commands

```
bored new "The IAM build" --slug iam                 # a root
bored new "Add OAuth login" --parent iam --ac "..."  # a child, todo because it has AC
bored tree iam                                       # hierarchy with progress
bored board                                          # workable tickets by status
bored next --as claude                               # the one thing to do, with a reason
bored claim 12 --as claude
bored log 12 "implemented handler" --as claude
bored ac 12 check 1
bored move 12 review -m "summary; branch feat/oauth"
bored done 12 -m "reviewed"                          # the human gate
bored prime                                          # context block for an agent session
```

Exit codes: `0` ok, `1` failure, `2` usage, `3` not found, `4` conflict
(bad transition, already claimed, cycle, open children), `5` nothing to do.

## Develop

```
make test     # unit tests and a subprocess test of the CLI
make smoke    # end-to-end script against a throwaway store
```

## Claude Code skills

Four skills drive the board from inside a Claude Code session. They are
embedded in the binary:

```
bored install-skills                                   # copy into ~/.claude/skills
bored install-skills --symlink --repo-dir ~/src/bored  # or link to a checkout for live editing
```

| Skill | What it does | Mutates? |
|---|---|---|
| `/bored-plan [id\|slug\|topic]` | Decomposes one ticket (or a new topic) into children. Proposes a table, waits for confirmation, then creates. | yes, after confirmation |
| `/bored-work [id]` | Claims a ticket (or `bored next`), checks it's in the right repo, works it, logs, ticks criteria, moves to review. Never marks done. | yes |
| `/bored-review [id]` | Checks a review ticket against each criterion, recommends done or back to todo. Asks before `done`. | yes, after confirmation |
| `/bored-next [id\|slug]` | Answers "what should I do next": what's waiting on you, the pick and why, what Claude could take in parallel. | no |

Each skill declares `allowed-tools: Bash(bored *) ...`, which pre-approves
those commands for the invoking turn. The pattern matches commands that start
with the bare word `bored`, so the binary must be on PATH; `~/go/bin/bored`
would prompt.

### Agents act as `claude`

Skills pass `--as claude` on mutating commands. `bored next --as claude` will
hand back a ticket Claude already has in `doing` before offering anything new.

### SessionStart hook (optional)

To have every session open knowing the board, add to `~/.claude/settings.json`:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume|clear|compact",
        "hooks": [
          { "type": "command", "command": "command -v bored >/dev/null && bored prime 2>/dev/null || true" }
        ]
      }
    ]
  }
}
```

`bored prime` prints the workflow rules, the board scoped to the repo you are
in, and the next pick. It always exits 0 and stays well under the 10,000
character hook cap.

### Plan mode and headless runs

If `~/.claude/settings.json` sets `"defaultMode": "plan"`, sessions start in
plan mode and the mutating skills will stop at a written plan instead of
touching the board. Interactively, leave plan mode (shift+tab) before running
`/bored-work` or confirming a `/bored-plan` table. Headlessly, pass the mode
explicitly:

```
claude -p "/bored-work 12" --permission-mode acceptEdits
claude -p "/bored-next"                                  # read-only, works in plan mode
```

Measured on 2026-10-09: `/bored-next` runs cost about $0.40, a `/bored-plan`
that creates eight tickets about $1.30, a `/bored-work` on a thinking ticket
about $0.70, a `/bored-review` that runs the test suite about $1.00.

## The TUI

`bored` with no arguments opens the interactive board when stdout is a
terminal (otherwise it prints the static board, so it is safe in scripts).

Two views. The **board** shows workable tickets in five columns, one line
per ticket (id and title; blocked ones in red). The line at the bottom
describes the selected ticket: priority, where, assignee, criteria,
blockers. Anything with open children is a container and only appears in
the **tree** view, with a done/total count, until its children are done.
Press `t` to switch.

```
j/k/h/l  move          enter  open the ticket       t    board / tree
[ ]      move status   c      claim                 d    done (asks first)
n        new ticket    m      append a log line     e    open in $EDITOR
/        text filter   p      cycle root filter     r    reload
?        help          q      quit
```

In the tree, `n` creates a child of the selected ticket; on the board it
creates a sibling. The board reloads itself every three seconds if any
ticket file changed, so agents moving cards while you watch show up.
Colours adapt to light and dark terminals.

## Headless runs: `bored run`

`bored run` hands ready tickets to headless Claude Code sessions, one at a
time, and stops when nothing is ready. Each ticket gets

```
claude -p "/bored-work <id>" --output-format json --permission-mode acceptEdits --max-turns 60
```

run in the right place: a per-ticket git worktree at
`<store>/worktrees/<repo>/<id>` on branch `<id>` for tickets that name a
repo (so your checkout is never touched and runs can't collide), or the
current directory for thinking work. Afterwards the ticket is reloaded. In
review means the agent handed over. Still in doing means the session ended
mid-way, so the runner releases it back to todo with a log line saying so.
Every session's output lands under `<store>/runs/<timestamp>-<id>-work/`
(`stdout.json`, `stderr.log`, `meta.json`), ignored by the store's git.

```
bored run                         # one ticket, then stop
bored run --max 0 --budget-usd 3  # until nothing is ready; cap each session at $3
bored run --under iam --review    # only that initiative; add a review pass per ticket
bored run --dry-run               # show the pick and where it would run
bored run --no-worktree           # run in the repo path itself
```

`--review` spawns a second session (`/bored-review <id>`, told to recommend
only) and appends its verdict to the ticket's log, so `bored show` and the
TUI carry the opinion next to the work. Marking done stays with you.

The loop stops after two consecutive failures so a broken setup can't burn
budget. Exit codes: 5 when nothing was ready, 1 when it stopped on failures.

**Permission mode.** `acceptEdits` is the default: file edits are allowed,
and Bash commands are allowed only where the skill's `allowed-tools` lists
them (`bored`, `git`, `go`, `npm`, `make`). Anything else is denied rather
than prompted, so a session can stall but can't do something unexpected.
Pass `--permission-mode bypassPermissions` if you decide you want the agent
unconstrained.

Measured on 2026-10-09: a thinking ticket took 15 turns and about $1.25; a
small code ticket in a worktree took 15 turns and about $1.00, plus roughly
$1.00 for the review pass.
