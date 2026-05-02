# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest  | ✅ |

## Reporting a Vulnerability

Please **do not** open a public GitHub issue for security vulnerabilities.

Email the maintainer directly or use GitHub's private vulnerability reporting:
**Settings → Security → Report a vulnerability**

We aim to respond within 72 hours and release a patch within 14 days for confirmed issues.

## Scope

kubectl-inventory is a **read-only** tool. It only calls `List` and `Get` against the Kubernetes API using your existing kubeconfig credentials. It makes no mutations to your cluster.
