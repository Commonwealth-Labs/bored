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
