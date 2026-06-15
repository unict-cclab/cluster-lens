# cluster-lens

`cluster-lens` is a lightweight local viewer for Kubernetes cluster topology and
pod placement.

It shows:

- cluster nodes with metrics from node annotations
- node-to-node latency from `network-latency.<node>` annotations
- pods placed on their current nodes
- pod colors based on the `group` label
- app/deployment metrics from annotations such as `cpu-usage`, `memory-usage`,
  `disk-bandwidth`, `network-bandwidth`, `rps.<peer>`, and `traffic.<peer>`

The backend uses the Kubernetes API directly and serves a static frontend. It
uses in-cluster credentials when deployed on Kubernetes, then falls back to the
current kubeconfig for local development.

## Run

```bash
cd backend
go run .
```

Then open:

```text
http://127.0.0.1:8088
```

Optional environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `CLUSTER_LENS_ADDR` | `127.0.0.1:8088` | HTTP listen address |
| `CLUSTER_LENS_REFRESH` | `2s` | Frontend polling interval hint |
| `CLUSTER_LENS_STATIC_DIR` | `../frontend` | Static frontend directory |
| `CLUSTER_LENS_CONTEXT` | `in-cluster` or current kubeconfig context | Display name for the connected cluster |
| `KUBECONFIG` | in-cluster config, then `~/.kube/config` | Optional local kubeconfig for development |

## Build

```bash
make build
```

Build and push a container image:

```bash
make build-image IMG=ghcr.io/<owner>/cluster-lens:v0.1.0
make push-image IMG=ghcr.io/<owner>/cluster-lens:v0.1.0
```

## Kubernetes

Install the service account, read-only RBAC, deployment, and service:

```bash
kubectl apply -f config/kubernetes/rbac.yaml
kubectl apply -f config/kubernetes/deployment.yaml
```

Then access the UI through port forwarding:

```bash
kubectl -n observability port-forward svc/cluster-lens 8088:8088
```

Open:

```text
http://127.0.0.1:8088
```

If you publish your own image, edit `config/kubernetes/deployment.yaml` or use
`kubectl set image deployment/cluster-lens cluster-lens=<image> -n observability`.

## Release

The GitHub Actions release pipeline builds and pushes an image to GHCR whenever
a tag matching `v*` is pushed:

```bash
git tag v0.1.0
git push origin v0.1.0
```

## Notes

The Kubernetes manifests grant read-only access to nodes, pods, deployments, and
replicasets. `cluster-lens` does not modify cluster state.
