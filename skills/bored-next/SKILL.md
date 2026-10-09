---
name: bored-next
description: Answer "what should I do next" from the bored kanban board, with the reason, what is waiting on the user, and what Claude could take in parallel. Use when the user asks what is next, what to pick up, or where things stand. Read-only; changes nothing.
argument-hint: [id | slug to scope under]
allowed-tools: Bash(bored *)
---

# What's next

Read-only. If `$ARGUMENTS` is given, add `--under $ARGUMENTS` to every command
below. Run them all, then answer.

```
bored next --json                 # the pick for the user (config actor)
bored next --as claude --json     # the pick for Claude
bored list --status review --json # waiting on the user
bored list --status doing --json  # in flight, by whom, since when
bored ready --json                # the queue, best first (exit 5 = empty)
```

Answer in three to six lines, in this order:

1. What is waiting on the user. Review items come first; finishing them
   unblocks the most.
2. The pick for the user, with the `reason` field in plain words, and what
   it unblocks.
3. What Claude could take in parallel if `next --as claude` differs.
4. Anything stale: `doing` for more than a day, or a `todo` ticket whose
   blocker is itself sitting in `doing` or `review`.

If nothing is ready anywhere, say so and suggest `/bored-plan` on the root
with the most backlog (`bored tree -d 2` shows the counts).

No mutations. Do not claim, move, or create anything from this skill.
