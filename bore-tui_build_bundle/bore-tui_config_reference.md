# bore-tui Config Reference (V1)

File: `repo/.bore/config.json`

## ui
- `theme`: string, default `"navy_red_dark"`
  - V1 supports only this theme, but keep field for future

## agents
- `claude_cli_path`: string, default `"claude"`
  - Executable name/path for Claude CLI
- `default_model`: string, default `""`
  - If non-empty, pass as a CLI flag or env var if supported; otherwise ignore safely
- `commander_context_limit`: int, default `5`
  - Max number of prior relevant runs to inject into Commander prompt
- `max_total_workers`: int, default `6`
  - Global cap for concurrent workers across all executions
- `max_workers_basic`: int, default `1`
- `max_workers_medium`: int, default `2`
- `max_workers_complex`: int, default `4`

## git
- `worktree_strategy`: string, default `"worktree"`
  - V1 only supports `worktree`
- `review_required`: bool, default `true`
  - Must be true for V1
- `auto_commit`: bool, default `false`
  - Must be false for V1

## logging
- `level`: string, default `"info"`
  - one of `debug|info|warn|error`
- `to_console`: bool, default `true`
  - also print logs to stderr in addition to file
- `rotation_mb`: int, default `10`
  - rotate rolling logs when exceeding this size
