# CLAUDE.md

## Code comments

Default to writing no inline comments. Add one only when the why is non-obvious — a hidden constraint, a workaround for a specific bug or provider quirk, an invariant a reader would otherwise miss. If removing the comment wouldn't confuse a future reader, don't write it.

* Don't restate what the code already says (`# create the bucket` above a `google_storage_bucket` resource is noise).
* Don't reference the current task, ticket, or callers in the comment itself (`# added for PLATFORM-XXXX`, `# used by ops/prod/...`) — that belongs in the commit message or PR description and rots as the codebase moves.
* Do leave a short comment when a value or argument exists for a reason the next reader can't infer from the code (e.g. "must match the value in the Atlantis Helm chart" or "GCS only allows ≤ 63 chars here").
* The same no-ticket-ref rule applies to resource `description = "…"` attributes — they surface in the GCP console as operator-facing metadata and rot the same way as code comments. Reference Jira keys in commit messages and PR descriptions, not in resource definitions.
* Prefer one line. If a comment needs a paragraph, the content almost certainly belongs in the commit message, the PR, or `docs/*`.

Existing comments are load-bearing until proven otherwise. Don't drop one while editing or reformatting nearby code; if a change makes a comment inaccurate, update it rather than deleting it, and say so in the commit body.

For larger documentation work — READMEs, runbooks, `docs/*` pages, module interface docs — use the `technical-documentation` skill rather than hand-rolling structure.

## Git workflow — agents do NOT commit

Agents must never run `git commit`, `git push`, `git merge`, `git rebase`, or any other mutating git command in this repo. The human operator owns all history-writing operations.

What an agent should do when a change is ready:

1. Write/edit files and stage them with `git add` if helpful for pre-commit.
2. Run `pre-commit run -a` (or `--files <paths>`) and fix any findings.
3. Hand the operator a ready-to-paste commit message in a shell-safe heredoc block so they can paste it verbatim. Use this exact format:

   ```
   git commit -m "$(cat <<'EOF'
   <type>(<scope>): <subject line under 72 chars>

   <body — wrap at ~72 chars, reference the Jira key, explain the why>

   EOF
   )"
   ```

   Rules for the message:
   * Use a single-quoted heredoc terminator (`<<'EOF'`) so `$`, backticks, and backslashes in the body are not interpreted by the shell.
   * Keep the subject to one line, imperative mood, ≤72 chars.
   * Reference the Jira key (e.g. `PLATFORM-3992`) in the body.
   * Do not include trailing whitespace on any line.
4. Stop. Wait for the operator to paste the command, review the staged diff, and commit themselves.

Apply the same rule to `git push`, PR creation (`gh pr create`), merges, and force-pushes — surface the suggested command, never execute it.

## What belongs in the commit body

The body carries the reasoning that must not live in the code. It is the reason the comment rules above can stay strict — anything cut from a comment for being narrative belongs here.

* Why the change, and why this shape rather than the obvious alternative.
* Deliberate deviations from repo or org convention, named as deliberate, so a reviewer doesn't "fix" them back.
* What was verified: the command run and its result. Never claim a plan, a hook, or a check passed without having run it.
* For lint or config changes, which findings were fixed versus suppressed, and why each.
* Pre-existing failures the change does not address, so they aren't mistaken for regressions.
