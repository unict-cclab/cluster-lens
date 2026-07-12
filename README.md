# Cluster Lens

Web UI and JSON API for Kubernetes topology, workloads, and annotated metrics.
Nodes are automatically grouped into movable zone regions using the
`topology.kubernetes.io/zone` label. Manual node positions remain saved per
cluster in the browser.

## Parameters

| Environment variable | Default | Description |
| --- | --- | --- |
| `CLUSTER_LENS_ADDR` | `127.0.0.1:8088` | HTTP listen address |
| `CLUSTER_LENS_REFRESH` | `2s` | Frontend polling interval |
| `CLUSTER_LENS_STATIC_DIR` | `../frontend` | Frontend asset directory |
| `CLUSTER_LENS_CONTEXT` | detected context | Displayed cluster name |
| `KUBECONFIG` | in-cluster/default | Optional local kubeconfig |

Endpoints:

- `/` — frontend
- `/api/config` — frontend configuration
- `/api/snapshot` — cluster snapshot

Kubernetes manifests are in [`config/kubernetes`](config/kubernetes).

```bash
cd backend
go test ./...
go run .
```
