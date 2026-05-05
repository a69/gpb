# Contributing to gpb

## Setup

```bash
git clone https://github.com/a69/gpb.git
cd gpb
go build ./...
```

Zero external dependencies beyond the Go standard library and uber-go/fx.

## Development workflow

```bash
# Run checks
go vet ./...

# Run tests with race detector
go test -race ./...

# Build
go build ./...
```

## Project structure

```
cmd/gpb/main.go       — CLI entry point, flag parsing, fx DI wiring
internal/
  github/             — GitHub GraphQL client (ProjectsV2 API)
  msg/                — Messenger abstraction + platform clients
  reporter/           — Business logic: formatting, polling, state diffing
```

## Adding a new messaging platform

1. Implement `msg.Messenger` in `internal/msg/` (see `telegram.go` or `slack.go` for examples)
2. Add the platform case to `provideMessenger` in `cmd/gpb/main.go`
3. Add tests using `httptest.NewServer`
4. Update `README.md` with setup instructions
5. Update the template workflows in `a69/gpb-template` if needed

## Conventions

- Standard library first — avoid adding dependencies unless necessary
- Interfaces are defined where consumed (`reporter.Messenger`), not where implemented
- Tests use `httptest` for HTTP clients, no external mocks
- Commit messages are lowercase, brief, and describe the "why"

## Pull requests

- Open a PR against `main`
- Fill out the PR template checklist
- Keep changes focused — one concern per PR
