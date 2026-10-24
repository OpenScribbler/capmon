# Agent Instructions

## Rules

- No mutation testing while the first implementation of the OpenScribbler stack is built (the `OpenScribbler/stack` repo's README defines it), which overrides the global rule to mutation-test each stage.
- Merge a PR you opened without asking once both second-opinion rounds are done, the second round's fixes are pushed, and CI passes on the PR's head commit. Merge with `gh pr merge --admin`, since `main` requires an approving review; ADR 0005's amendment allows this while capmon is unshared. A PR the pipeline opens (a `capmon/heal-*` or `capmon/drift-*` branch) waits for a person to approve it (heal PRs under ADR 0014, drift PRs under ADR 0005's amendment).

## Non-Interactive Shell Commands

**ALWAYS use non-interactive flags** with file operations to avoid hanging on confirmation prompts.

Shell commands like `cp`, `mv`, and `rm` may be aliased to include `-i` (interactive) mode on some systems, causing the agent to hang indefinitely waiting for y/n input.

**Use these forms instead:**
```bash
# Force overwrite without prompting
cp -f source dest           # NOT: cp source dest
mv -f source dest           # NOT: mv source dest
rm -f file                  # NOT: rm file

# For recursive operations
rm -rf directory            # NOT: rm -r directory
cp -rf source dest          # NOT: cp -r source dest
```

**Other commands that may prompt:**
- `scp` - use `-o BatchMode=yes` for non-interactive
- `ssh` - use `-o BatchMode=yes` to fail instead of prompting
- `apt-get` - use `-y` flag
- `brew` - use `HOMEBREW_NO_AUTO_UPDATE=1` env var
