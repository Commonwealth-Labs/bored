---
name: bored-review
description: Review a ticket that is waiting in review on the bored kanban board. Checks the work against each acceptance criterion and recommends done or back to todo with specific notes. Use when the user asks to review a ticket, asks what is waiting for review, or says /bored-review.
argument-hint: [id | slug]
allowed-tools: Bash(bored *) Bash(git status *) Bash(git log *) Bash(git diff *) Bash(git show *) Bash(git branch *) Bash(go test *) Bash(npm test *) Bash(make test *) Read Grep Glob
---

# Review a ticket

Argument: `$ARGUMENTS` (an id or slug; may be empty).

## 1. Pick

- With an argument: `bored show <ref> --json`. It must be in review; if not,
  say what state it is in and stop.
- Without one: `bored list --status review --json`. Nothing there means nothing
  is waiting; say so. Several means list them and ask which, unless the user
  said to review them all, in which case take them in id order.

## 2. Gather the evidence

- Read the ticket: acceptance criteria, plan, and log. The last "-> review"
  line in the log is the handover summary and usually names the branch and
  commit.
- Repo ticket: `bored repo detect --json` must match the ticket's repo; if it
  does not, say which repo to open and stop. Then, from the `branch` field or
  the log: `git log <default>..<branch> --oneline`, `git diff <default>...<branch>`,
  and run the tests the log mentions.
- Repo-less ticket: the deliverable is in the ticket itself, in the Plan
  section and the log. Read it as the decision or design it claims to be.

## 3. Judge each criterion

For every acceptance criterion give one of met, not met, or cannot tell, with a
one-line reason tied to a file, a test result, or a paragraph. Then note
anything the change does that the ticket did not ask for, and anything risky
(data loss, security, missing tests for new behaviour).

## 4. Verdict

- All met and nothing alarming: recommend done. Ask the user; on yes run
  `bored done <id> -m "reviewed: <one line>"`. If the user said to
  auto-approve, skip the question.
- Otherwise send it back with notes a future session can act on directly:
  `bored move <id> todo -m "needs: <criterion N: what would satisfy it; ...>"`.
  Put longer notes in `bored log <id> "..."`.

Do not fix the work yourself during a review. A returned ticket gets fixed by
`/bored-work <id>`.
