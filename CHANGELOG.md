# Changelog

All notable changes to kubectl-inventory are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

---

## [Unreleased]

### Added
- `explain` command — detailed per-resource classification report with owner chain,
  referrers, GitOps signals, and confidence level
- `--api-groups` and `--exclude-api-groups` — restrict or exclude API groups from scan
- `--resources` and `--exclude-resources` — restrict or exclude specific resource types
- `--include-events`, `--include-metrics`, `--include-noisy` — opt in to noisy resources
  excluded by the default noise profile
- `-l / --selector` — label selector forwarded to every List call
- `--concurrency` — configurable goroutine pool size (default 10)
- `--request-timeout` — per-GVR context deadline (default 30s)
- `--deep` — full-object hydration for all types (metadata-only by default)
- Metadata-first scan using `PartialObjectMetadataList` — ~10× faster than full-object scans
- Selective hydration: 13 core Kinds always fetched in full; all others use metadata
- Noise profile: events, metrics, leases, CiliumEndpoints excluded by default with footer hint
- `NoisySkipped` stat shown in summary footer

### Changed
- `CollectAll` now accepts an `Options` struct (concurrency, timeout, selector, deep)
- `gitops.go`: split `DetectGitOps` into `DetectGitOps` + `DetectGitOpsFromMeta` so
  fast (metadata-only) path still detects ArgoCD/Helm/Flux

---

## [0.1.0] — Initial release

### Added
- Namespace-wide resource inventory across all API types including CRDs
- Dynamic API-group grouping in table output
- Suspicious orphan detection with reasons
- Dangling ownerReference detection
- Stuck finalizer detection
- GitOps signal detection (ArgoCD, Helm, Flux)
- Reference-walker for Pod, Deployment, StatefulSet, Ingress, HTTPRoute,
  RoleBinding, HPA, and generic CRD spec walking
- Owner tree output (`-o tree`)
- JSON output (`-o json`)
- `orphans` subcommand
- `stuck` subcommand
- `diff` subcommand (namespace-to-namespace comparison)
- Age filter (`--age`)
- All-namespaces scan (`-A`)
- System namespace filtering (`--include-system`)
