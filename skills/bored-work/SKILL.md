---
name: bored-work
description: Work a ticket from the bored kanban board as the agent "claude": claim it, do the work in the right repo, log progress, tick acceptance criteria, and hand it to review. Invoke with /bored-work [id] or just /bored-work to take the next ready ticket.
argument-hint: [id | slug]
disable-model-invocation: true
allowed-tools: Bash(bored *) Bash(git *) Bash(go *) Bash(npm *) Bash(make *) Read Edit Write Grep Glob
---

# Work a ticket

You act on the board as `claude`: pass `--as claude` to every mutating
`bored` command. Session: `${CLAUDE_SESSION_ID}`.

Argument: `$ARGUMENTS` (an id or slug; may be empty).

## 1. Pick

- With an argument: `bored show $ARGUMENTS --json`.
- Without one: `bored next --as claude --json`. Exit 5 means nothing is ready:
  say so, print `bored board`, and stop. The `reason` field explains the pick;
  "already in progress" means you are resuming your own earlier work, so read
  the log before touching anything.

## 2. Check you are in the right place

- `workable` false means the ticket has open children. Stop and say which
  children to work instead (`bored tree <id>`).
- If the ticket has a `repo`: run `bored repo detect --json`. If that fails or
  names a different repo, print the expected path from `bored repo list` and
  stop. Never `cd` into another repo; the user starts the session where the
  work belongs.
- If it has no `repo`: this is thinking work (a decision, a design, a spike).
  Do it here. The deliverable is text that ends up in the ticket.

## 3. Claim

`bored claim <id> --as claude`. Exit 4 means blocked, assigned to someone
else, or not in todo: report the reason and stop. Do not use `--force` unless
the user asks for it.

## 4. Plan, then work

- Read Description, Acceptance Criteria, Plan and Log. Earlier attempts leave
  notes in the log; a ticket returned from review has a "needs:" line saying
  exactly what was missing.
- If Plan is empty, write a short one before coding:
  `bored set <id> --plan "<3 to 8 lines: what changes, where, how you verify>"`.
- Code work: branch from the repo's default branch, named after the ticket
  (`git checkout -b brd-12-oauth-login`), small commits, tests as you go. Do not
  push unless the user has asked for pushes.
- Log at meaningful checkpoints, not every step:
  `bored log <id> "<what changed, what you learned>" --as claude`.
- Tick criteria only when verifiably met: `bored ac <id> check N`.
- Found extra work? File it instead of doing it:
  `bored new "<title>" --parent <same parent as this ticket> --status backlog --description "Found while working <id>: ..."`.

## 5. Hand over

When every criterion is ticked and the tests pass:

- Code: commit, then record the branch: `bored set <id> --branch <branch>`.
- Thinking work: put the decision and reasoning in the ticket:
  `bored set <id> --plan "<the decision, the options rejected, and why>"`.
- Move it: `bored move <id> review -m "<two lines on what was done; branch X; commit Y>" --as claude`.
- Tell the user it is waiting for review and what to look at first.

Never run `bored done`. Marking work done is the human's call.

## Blocked or failing

If you cannot finish: `bored log <id> "blocked: <why, and what would unblock it>" --as claude`,
then release it: `bored move <id> todo -m "released: <why>" --as claude`. If a
specific ticket must land first, `bored dep add <id> <blocker>`.
