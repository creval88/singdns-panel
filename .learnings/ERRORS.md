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

## 2026-03-31 — sessions_spawn 参数与 edit 模式误用（复发）
- subagent 不支持 streamTo，调用会直接报参数错误。
- edit 不能混用单块替换与 edits[]；首次报错后应立即切换到短脚本/write。

## 2026-03-31 — sandbox 无 Go 导致测试无法执行（复发）
- 命令：`go test ./internal/services ./internal/handlers ./cmd/server ./...`
- 报错：`/bin/sh: 1: go: not found`
- 处理：该容器无法本地编译/测试，需在宿主机（有 Go）执行同命令。

## 2026-04-02 — edit 工具再次触发“single replacement 与 edits 混用”
- 报错：`Edit tool input is invalid. Use either edits or single replacement mode, not both.`
- 触发场景：在修复 `manual_nodes.go` 字符串字面量时，调用参数里同时携带了 single replacement 字段与 `edits`。
- 处理：立即切换到 `python3` 小脚本直接改文件，然后 `gofmt`。
- 规避：后续优先使用 `write/exec python` 做小范围修改，除非明确只走 edit 的单一模式。
