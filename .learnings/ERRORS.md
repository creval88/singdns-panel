# ERRORS.md

## 2026-03-29 — go test failed with `cannot find main module` due to wrong working directory

### Symptom
Running:
- `cd /Users/abc/.openclaw/workspace-panda/singdns-panel && go test ...`

Returned:
- `go: cannot find main module, but found .git/config in /Users/abc/.openclaw/workspace-coq`

### Root cause
Tool exec did not actually change to the intended directory; the effective cwd stayed at `/Users/abc/.openclaw/workspace-coq`.

### Fix / prevention
- Prefer passing `workdir` explicitly to `exec` instead of relying on `cd ... && ...`.
- When in doubt, verify with `pwd` using `workdir`.

### Notes
This can produce confusing downstream errors like `No such file or directory` for paths that do exist under the intended repo.

## 2026-03-29 — grep/rg "No such file or directory" caused by wrong cwd

### Symptom
`grep -R ... internal/...` reported missing paths.

### Root cause
Same as above: command executed outside the repo.

### Fix
Re-run with `workdir=/Users/abc/.openclaw/workspace-panda/singdns-panel`.
