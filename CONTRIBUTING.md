# Contributing to kubectl-inventory

Thank you for considering a contribution! This is a small, focused tool — PRs are welcome as long as they keep it that way.

## Ground Rules

- **Read-only**: the tool must never mutate cluster state.
- **No cluster-side installation**: no CRDs, no webhooks, no controllers installed by this tool.
- **Conservative signals**: if in doubt, don't flag something as suspicious. False positives erode trust.
- **Test with a real cluster**: run `go build && ./bin/kubectl-inventory -n <some-namespace>` before submitting.

## Development Setup

```bash
git clone https://github.com/shahneil76/kubectl-inventory
cd kubectl-inventory
go mod tidy
make build
```

Binary lands in `bin/kubectl-inventory`. Drop it in your `$PATH` and it works as a kubectl plugin.

## Project Structure

```
cmd/            CLI commands (root scan, explain, orphans, stuck, diff)
pkg/
  analyzer/     Classification logic (orphans, stuck, coverage)
  client/       Kubernetes client setup (dynamic + metadata + discovery)
  collector/    Resource fetching (fast metadata path + full spec path)
  discovery/    API resource enumeration
  filter/       API-group, resource, and noise-profile filtering
  output/       Renderers (table, tree, JSON)
  types/        Core types (Resource, Inventory, ScanStats)
```

## Adding a New Feature

1. Open an issue first if it changes the output format or adds a new flag.
2. Keep `pkg/` packages decoupled — analyzers must not import collectors.
3. Add a `// Package ...` doc comment to any new package.
4. Run `go vet ./...` and `gofmt -w .` before pushing.

## Submitting a PR

- Target the `main` branch.
- Include a description of *why*, not just *what*.
- Link the issue it closes.
- Keep diffs small — one logical change per PR.

## Not Accepted

- Features that require cluster-side installation
- Dependencies on `kubectl` binary (use client-go directly)
- Caching of cluster state to disk
- Cloud-provider-specific logic in core packages
