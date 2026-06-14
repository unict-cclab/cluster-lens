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

This first version is intentionally simple. The backend shells out to `kubectl`
using the current kubeconfig context and serves a static frontend.

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

## Notes

The app expects `kubectl` to be available and configured for the target cluster.
It does not modify cluster state.

