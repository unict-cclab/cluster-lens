# Cluster Lens

Web UI and JSON API for Kubernetes topology, workloads, and annotated metrics.

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
