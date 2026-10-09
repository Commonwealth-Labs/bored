---
name: bored-plan
description: Plan work on the bored kanban board by breaking an initiative, chunk, or topic into child tickets with acceptance criteria and dependencies. Use when the user wants to plan, decompose, break down, refine, or think about next steps for a project or a ticket. Proposes a table first, creates tickets only after the user confirms, and never starts work.
argument-hint: [id | slug | topic words]
allowed-tools: Bash(bored *) Bash(git status *) Bash(git log *) Bash(git ls-files *) Read Grep Glob
---

# Plan work on the board

You are decomposing one ticket into children. The target may be a root (an
initiative like "the IAM build"), a chunk under one, or a leaf being refined.
The procedure is the same at every depth. Nothing here needs a repo or a path:
planning is about the work, and a repo gets attached to a ticket only once it
is concrete code.

Argument: `$ARGUMENTS` (an id, a slug, or free text naming a topic; may be empty).

## 1. Resolve the target

- Id or slug: `bored show <ref> --json`. Exit 3 means it doesn't exist; say so and stop.
- Free text or empty: run `bored tree -d 2` and ask whether this is a new
  initiative or belongs under an existing ticket. For a new initiative create a
  root: `bored new "<title>" --slug <slug> --description "<one paragraph>"`.
  Propose a short lowercase slug and confirm it before creating.
- If `bored repo detect --json` succeeds you are inside a registered repo;
  mention which initiatives have work there (`bored prime`), but do not assume
  the target from the directory.

## 2. Load context before proposing anything

- `bored tree <target> -l` for what already exists underneath. Build on it;
  never recreate tickets that are there.
- `bored show <target> --json` for the description, plan and log. The log holds
  earlier decisions.
- If the target or an ancestor names a repo that exists on disk and the work
  is code, spend a few minutes in that repo (layout, the relevant files, how
  tests run) so the split is grounded in reality. If no repo is named, do not
  go looking for one.
- Ask only the questions whose answers change the split: goal, hard
  constraints, what is already decided, what is out of scope. Two or three
  questions, not an interrogation. If the user has said "just propose", skip
  the questions and state your assumptions in the table.

## 3. Propose the split as a table

| # | Title | Pri | Depends on | Repo | Acceptance criteria |
|---|-------|-----|------------|------|---------------------|

Rules of thumb:

- 3 to 8 children. If the work needs more, propose intermediate chunks and plan
  those later with `/bored-plan <chunk>`.
- At the top of a tree (root to chunks) the children are conceptual. They get
  no acceptance criteria and go to backlog.
- Lower down, each child should fit one Claude session and one PR: 2 to 5
  acceptance criteria, each one checkable by reading a file, running a test, or
  reading a paragraph the ticket will contain.
- Thinking work (decide, design, spike, research) is a legitimate ticket. Its
  criteria look like "decision and reasoning recorded in this ticket". It gets
  no repo.
- Code work gets a repo. If that repo does not exist yet, add a child "Create
  repo X at <path> and register it with `bored repo add`" and make the code
  tickets depend on it.
- `depends_on` is for real ordering constraints only, not for preference.
- Priority 1 to 4; 3 is the default. Reserve 1 for things that unblock others.

Wait for the user to confirm or edit. Iterate on the table, not on the board.

## 4. Create

Create children with `--no-commit`, then commit once:

```
bored new "<title>" --parent <target> --priority N [--repo r] [--depends-on ID] [--ac "..."]... --no-commit
...
bored sync -m "plan: <target> <short summary>"
```

`--ac` puts a ticket in todo; without it the ticket goes to backlog. For
dependencies between new siblings, create the blocker first and use the id it
prints. `--repo` defaults to the parent's repo; pass `--repo none` for a
thinking ticket under a code chunk.

Record the reasoning on the target itself so it survives:

```
bored set <target> --description "<goal and constraints>"
bored set <target> --plan "<options considered and the chosen split, a few lines>"
```

Finish by printing `bored tree <target> -l`.

## Never

- Claim, move, or start work on anything. Planning ends at the board.
- Edit files under the store directory directly; only use the `bored` CLI.
- Create tickets the user has not seen in the table.
