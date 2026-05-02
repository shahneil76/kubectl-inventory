# How kubectl-inventory Achieves Fast Cluster Scans

**kubectl-inventory** scans every API resource in a Kubernetes namespace — including all CRDs — and classifies them by ownership, references, GitOps signals, and stuck finalizers. A typical namespace with 198 API types completes in **~7–8 seconds**.

This document explains the four architectural decisions behind that.

---

## Benchmark

| Command | Task | Time | Resources found |
|---|---|---:|---:|
| `kubectl inventory -n argocd` | full inventory + analysis | 7.8s | 140 |
| `kubectl inventory -n argocd --deep` | full-spec + reference analysis | 6.4s | 140 |
| `kubectl inventory -n argocd --api-groups=apps,argoproj.io` | focused scan, 2 groups | 3.8s | 64 |

> Tested on AWS EKS, GKE, Azure AKS, RKE2, and minikube clusters.

For reference: a naive `kubectl get` loop over all 198 API types in the same namespace takes 70–80s because it fetches full objects serially.

---

## 1. Metadata-Only Fetching (PartialObjectMetadataList)

The single biggest performance win.

**Problem:** A full resource list (`kubectl get`) fetches the entire `.spec`, `.status`, and `.data` for every object — most of which is never needed for inventory purposes.

**Solution:** kubectl-inventory uses the Kubernetes **metadata client** (`k8s.io/client-go/metadata`) by default. This calls the same List API but requests only `PartialObjectMetadata` via the `Accept: application/json;as=PartialObjectMetadata` header.

What `PartialObjectMetadata` includes:
- `name`, `namespace`, `uid`
- `ownerReferences`
- `labels`, `annotations`
- `finalizers`
- `deletionTimestamp`
- `creationTimestamp`
- `managedFields` (first manager only)

What it omits:
- `.spec` (deployment templates, container images, volumes, etc.)
- `.status` (conditions, replicas, endpoints, etc.)
- `.data` (ConfigMap/Secret payloads)

**Impact:** Payloads are 5–50× smaller per resource. The API server can often serve metadata from its watch cache without touching etcd. On a namespace with 140 resources across 186 types, this alone cuts scan time from ~70s to ~7s.

**Opt-out:** Pass `--deep` to fetch full objects for all types.

```bash
# Default: metadata-only for most types (~7s)
kubectl inventory -n prod

# Full spec for everything (~7–10s depending on cluster size)
kubectl inventory -n prod --deep
```

### Selective Hydration

Not all metadata is enough. Some analysis — like detecting that a Deployment references a Secret via `envFrom` — requires the full `.spec`. kubectl-inventory maintains a list of **deep Kinds** that are always fetched as full objects, even in default mode:

```
Pod, Deployment, ReplicaSet, StatefulSet, DaemonSet, Job, CronJob,
Ingress, HTTPRoute, Service, HorizontalPodAutoscaler,
RoleBinding, ClusterRoleBinding
```

These 13 Kinds power the reference-walker. The remaining 170+ types use metadata-only.

---

## 2. Bounded Concurrency

**Problem:** Listing 192 resource types sequentially is slow even if each individual call is fast. A serial loop at 500ms per GVR would take 96 seconds.

**Solution:** kubectl-inventory runs all API list calls concurrently using a goroutine pool with a bounded semaphore.

```go
sem := make(chan struct{}, concurrency) // default: 10
for _, apiResource := range apiResources {
    go func() {
        sem <- struct{}{}
        defer func() { <-sem }()
        // list this GVR
    }()
}
```

Default concurrency is 10. Configurable via `--concurrency`:

```bash
kubectl inventory -n prod --concurrency 20
```

**Impact:** With 192 resource types and concurrency 10, the scan completes in roughly `ceil(192/10)` batches of parallel calls instead of 192 sequential ones.

---

## 3. Noise Profile (Default Exclusions)

**Problem:** Many API types return high-volume, low-value data. `metrics.k8s.io` endpoints are aggregated APIs that are often slow. `events` and `leases` are high-churn objects that add noise without diagnostic value for most inventory use cases.

**Solution:** kubectl-inventory excludes a small set of types by default:

| Excluded | API Group | Why |
|---|---|---|
| `events` | core | High churn, not inventory |
| `events` | `events.k8s.io` | High churn, not inventory |
| `podmetrics` | `metrics.k8s.io` | Aggregated API, often slow |
| `nodemetrics` | `metrics.k8s.io` | Aggregated API, often slow |
| `leases` | `coordination.k8s.io` | Controller heartbeats, not inventory |

Plus runtime Kinds: `CiliumEndpoint`, `CiliumNode`, `ControllerRevision`.

This is never silent — the output always reports it:

```
Noise profile:  6 resource types excluded  (use --include-noisy to scan all)
```

Opt-in flags:
```bash
--include-events     # add back events
--include-metrics    # add back metrics
--include-noisy      # add back everything
```

**Impact:** Saves 6 API calls, avoids the slowest aggregated APIs, and reduces output noise.

---

## 4. Per-GVR Timeout

**Problem:** One slow or broken API group (an overloaded aggregated API, a webhook-backed CRD under pressure) can block an entire scan. A single 30s hang propagates to the whole result.

**Solution:** kubectl-inventory wraps each GVR list call in its own `context.WithTimeout`:

```go
if opts.RequestTimeout > 0 {
    listCtx, cancel = context.WithTimeout(ctx, opts.RequestTimeout)
    defer cancel()
}
```

Default: 30s per GVR. Configurable:

```bash
kubectl inventory -n prod --request-timeout 5s
```

If a GVR times out, it is skipped and reported:

```
Skipped: podmetrics.metrics.k8s.io: context deadline exceeded
```

**Impact:** Prevents one broken API from making the entire scan slow. On clusters with flaky aggregated APIs, this alone can save 30–60s.

---

## Architecture

```
┌─────────────┐    ┌──────────────┐    ┌──────────────────────┐
│  Discovery   │───▶│   Filter     │───▶│     Collector         │
│ (all GVRs)   │    │ (noise,      │    │ ┌─────────────────┐  │
│              │    │  api-groups,  │    │ │ metadata client  │  │
│              │    │  resources)   │    │ │ (170+ types)     │  │
│              │    │              │    │ ├─────────────────┤  │
│              │    │              │    │ │ dynamic client   │  │
│              │    │              │    │ │ (13 deep Kinds)  │  │
│              │    │              │    │ └─────────────────┘  │
└─────────────┘    └──────────────┘    └──────────┬───────────┘
                                                   │
                                       ┌───────────▼───────────┐
                                       │      Analyzer          │
                                       │  - owner graph         │
                                       │  - reference walker    │
                                       │  - orphan classifier   │
                                       │  - finalizer detector  │
                                       │  - GitOps detector     │
                                       └───────────┬───────────┘
                                                   │
                                       ┌───────────▼───────────┐
                                       │      Output            │
                                       │  table / tree / json   │
                                       └───────────────────────┘
```

---

## Quick Start

```bash
# Scan current namespace (fast mode, ~7s)
kubectl inventory

# Scan with full reference analysis
kubectl inventory --deep

# Scan only apps and networking
kubectl inventory --api-groups=apps,networking.k8s.io

# Scan with label selector
kubectl inventory -l app=payment

# Exclude slow API groups
kubectl inventory --exclude-api-groups=metrics.k8s.io

# Tune for large clusters
kubectl inventory --concurrency 20 --request-timeout 5s

# Include everything
kubectl inventory --include-noisy
```

---

## TL;DR

Four decisions make kubectl-inventory fast:

1. **Metadata-only fetching** — 5–50× smaller payloads via `PartialObjectMetadataList`
2. **Concurrent collection** — 10 goroutines (configurable) instead of serial calls
3. **Noise profile** — skip events, metrics, leases by default
4. **Per-GVR timeout** — one slow API doesn't stall the whole scan

The result: a full namespace inventory with orphan detection, reference graphs, and GitOps signals in **~7s** on clusters with 190+ API types.
