#!/usr/bin/env bash
# End-to-end smoke test against a throwaway store. Run via `make smoke`.
set -u
BIN="${BORED_BIN:-bored}"
TMP="$(mktemp -d)"
export BORED_HOME="$TMP/store"
REPO="$TMP/repo"
fail=0
pass() { echo "ok   $1"; }
bad()  { echo "FAIL $1"; fail=1; }
expect_exit() { # expect_exit <code> <desc> <cmd...>
  local want="$1" desc="$2"; shift 2
  "$@" >/dev/null 2>&1; local got=$?
  if [ "$got" = "$want" ]; then pass "$desc (exit $got)"; else bad "$desc: want exit $want got $got"; fi
}
b() { "$BIN" "$@"; }

echo "store: $BORED_HOME"
expect_exit 0 "init" "$BIN" init
expect_exit 4 "init twice conflicts" "$BIN" init

git -C "$TMP" init -q "$REPO" && git -C "$REPO" commit -q --allow-empty -m init
expect_exit 0 "repo add" "$BIN" repo add demo "$REPO"
expect_exit 3 "repo detect outside any repo" "$BIN" repo detect
( cd "$REPO" && "$BIN" repo detect --json | grep -q '"name":"demo"' ) && pass "repo detect inside repo" || bad "repo detect inside repo"

b new "The IAM build" --slug iam >/dev/null                                  || bad "new root"
b new "Authentication" --parent iam >/dev/null                               || bad "new chunk"
b new "Decide IdP" --parent 2 --priority 1 --ac "decision recorded" >/dev/null || bad "new leaf 3"
b new "Add OAuth login" --parent 2 --priority 2 --repo demo --ac "redirects" --ac "cookie" --depends-on 3 >/dev/null || bad "new leaf 4"
b new "Write design" --parent 2 >/dev/null                                   || bad "new leaf 5"
expect_exit 2 "duplicate slug" "$BIN" new "dup" --slug iam
expect_exit 3 "unknown parent" "$BIN" new "orphan" --parent 99
expect_exit 3 "unknown repo" "$BIN" new "x" --repo nope

b board | grep -q "BRD-2" && bad "board shows container" || pass "board hides container with open children"
b board --all-levels | grep -q "BRD-2" && pass "board --all-levels shows container" || bad "board --all-levels"
b tree iam | grep -q "0/3" && pass "tree shows progress 0/3" || bad "tree progress"
b tree iam | grep -q "BRD-4.*\[blocked\]" && pass "tree marks blocked" || bad "tree blocked marker"

b ready --as claude | grep -q "BRD-4" && bad "ready includes blocked" || pass "ready excludes blocked"
[ "$(b next --as claude --json | jq -r .ticket.id)" = "BRD-3" ] && pass "next picks P1" || bad "next"
expect_exit 4 "claim blocked" "$BIN" claim 4 --as claude
expect_exit 4 "claim container" "$BIN" claim 2 --as claude --force
expect_exit 0 "claim" "$BIN" claim 3 --as claude
expect_exit 4 "claim twice" "$BIN" claim 3 --as claude
[ "$(b next --as claude --json | jq -r .reason | cut -c1-19)" = "already in progress" ] && pass "next returns in-progress" || bad "next in-progress"
expect_exit 0 "log" "$BIN" log 3 "looked at both" --as claude
expect_exit 0 "ac check" "$BIN" ac 3 check 1
expect_exit 3 "ac check missing" "$BIN" ac 3 check 9
expect_exit 0 "move review" "$BIN" move 3 review -m "decision: X" --as claude
expect_exit 4 "done before review" "$BIN" done 4
expect_exit 0 "done" "$BIN" done 3 -m agreed
b ready --as claude | grep -q "BRD-4" && pass "dependent becomes ready" || bad "dependent ready"
expect_exit 4 "dep cycle" "$BIN" dep add 3 4
expect_exit 0 "set repo via set" "$BIN" set 5 --repo demo
expect_exit 2 "set nothing" "$BIN" set 5
expect_exit 4 "reparent cycle" "$BIN" set iam --parent 5
( cd "$REPO" && [ "$(b next --as claude --repo demo --json | jq -r .ticket.id)" = "BRD-4" ] ) && pass "next --repo" || bad "next --repo"

# Finish the chunk: container becomes workable, then done.
b claim 4 --as claude >/dev/null && b move 4 review >/dev/null && b done 4 >/dev/null
b move 5 todo >/dev/null && b claim 5 --as claude >/dev/null && b move 5 review >/dev/null && b done 5 >/dev/null
b board --under iam | grep -q "BRD-2" && pass "container workable after children done" || bad "container reappears"
expect_exit 4 "move root to done with open children" "$BIN" move iam done
b move 2 todo >/dev/null && b claim 2 >/dev/null && b move 2 review >/dev/null
expect_exit 0 "done container" "$BIN" done 2
expect_exit 5 "next with nothing ready" "$BIN" next --under iam --as nobody
expect_exit 3 "show unknown" "$BIN" show 99
expect_exit 2 "bad status" "$BIN" move 1 nowhere
expect_exit 2 "unknown flag" "$BIN" list --bogus
expect_exit 0 "prime" "$BIN" prime
( cd "$REPO" && "$BIN" prime | grep -q "repo demo" ) && pass "prime detects repo" || bad "prime repo"
expect_exit 0 "sync no-op" "$BIN" sync
b --no-commit new "uncommitted" >/dev/null
[ "$(git -C "$BORED_HOME" status --porcelain | wc -l | tr -d ' ')" != "0" ] && pass "--no-commit leaves changes" || bad "--no-commit"
expect_exit 0 "sync commits" "$BIN" sync -m "batch"
[ "$(git -C "$BORED_HOME" status --porcelain | wc -l | tr -d ' ')" = "0" ] && pass "sync cleaned up" || bad "sync"

commits=$(git -C "$BORED_HOME" rev-list --count HEAD)
echo "commits in store: $commits"
[ "$commits" -gt 20 ] && pass "one commit per mutation" || bad "commit count $commits"

ls "$BORED_HOME/tickets" | grep -q tmp && bad "stray tmp files" || pass "no tmp files"

if [ "$fail" = 0 ]; then echo "SMOKE PASSED"; rm -rf "$TMP"; else echo "SMOKE FAILED (store kept at $BORED_HOME)"; exit 1; fi
