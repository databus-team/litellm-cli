# Repository Guidelines

## Project Structure & Module Organization

This is a Go CLI module named `litellm-cli`. `main.go` only starts the Cobra command tree. CLI commands live in `cmd/`, usually one command or feature per file, such as `logs.go`, `models.go`, and `teams.go`. Shared implementation belongs under `internal/`: API request and response types are in `internal/api/`, HTTP client logic is in `internal/client/`, and local configuration helpers are in `internal/config/`. Tests are colocated with packages as `*_test.go`. Top-level shell scripts provide manual LiteLLM permission and endpoint checks. Planning and ideation notes live under `docs/`.

## Build, Test, and Development Commands

- `make build`: builds the stripped `litellm-cli` binary in the repository root.
- `make test`: runs `go test -v -race ./...` across all packages.
- `make install`: builds and moves the binary to `~/.local/bin/litellm-cli`.
- `make release`: cross-compiles Darwin and Linux release binaries.
- `make clean`: removes generated binaries and release artifacts.
- `go run . <command>`: runs the CLI locally during development, for example `go run . models --api-key "$LITELLM_API_KEY"`.

## Coding Style & Naming Conventions

Format Go code with `gofmt`; use standard Go tabs and import grouping. Package names should be short, lowercase, and match their directory. Keep Cobra command files in `cmd/` named after the command or feature they implement. Use clear exported names only for APIs needed outside the package. Existing user-facing output is primarily Chinese; keep new command help and errors consistent with nearby text.

## Testing Guidelines

Use Go’s standard `testing` package. Name tests `TestXxx` and colocate them with the package under test. Prefer table-driven tests for API types, config parsing, and command/client behavior with multiple cases. Use environment isolation for config tests, and run `make test` before opening a pull request.

## Commit & Pull Request Guidelines

Recent history follows Conventional Commits with scopes, for example `feat(logs): 添加日志详情查看功能` and `fix(logs): 修复 TUI 导航功能`. Keep the scope tied to a command or package. Pull requests should include a short purpose statement, linked issue when available, commands run, and notes for any API, config, or terminal UI behavior changes. Include screenshots or terminal output when changing TUI views.

## Security & Configuration Tips

Do not commit API keys or personal config. Use `LITELLM_API_KEY`, `--api-key`, `--base-url`, or `~/.litellm-cli.yaml` for local settings. Avoid logging full credentials in errors, debug output, or test fixtures.
