package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAddr      = "127.0.0.1:8088"
	defaultRefresh   = 2 * time.Second
	defaultStaticDir = "../frontend"
	kubectlTimeout   = 10 * time.Second
)

type snapshot struct {
	GeneratedAt time.Time  `json:"generatedAt"`
	Context     string     `json:"context"`
	Nodes       []nodeView `json:"nodes"`
	NodeEdges   []edgeView `json:"nodeEdges"`
	Pods        []podView  `json:"pods"`
	AppEdges    []edgeView `json:"appEdges"`
	Warnings    []string   `json:"warnings,omitempty"`
}

type nodeView struct {
	Name        string            `json:"name"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Ready       bool              `json:"ready"`
	Role        string            `json:"role,omitempty"`
	CPU         float64           `json:"cpu,omitempty"`
	Memory      float64           `json:"memory,omitempty"`
	Disk        float64           `json:"disk,omitempty"`
	Network     float64           `json:"network,omitempty"`
}

type podView struct {
	Namespace  string            `json:"namespace"`
	Name       string            `json:"name"`
	Node       string            `json:"node,omitempty"`
	Phase      string            `json:"phase,omitempty"`
	Group      string            `json:"group,omitempty"`
	App        string            `json:"app,omitempty"`
	Owner      string            `json:"owner,omitempty"`
	Labels     map[string]string `json:"labels,omitempty"`
	Metrics    map[string]string `json:"metrics,omitempty"`
	CreatedAt  string            `json:"createdAt,omitempty"`
	Containers []string          `json:"containers,omitempty"`
}

type edgeView struct {
	Source string  `json:"source"`
	Target string  `json:"target"`
	Kind   string  `json:"kind"`
	Value  float64 `json:"value,omitempty"`
	Label  string  `json:"label,omitempty"`
}

type kubeList[T any] struct {
	Items []T `json:"items"`
}

type kubeObjectMeta struct {
	Name              string            `json:"name"`
	Namespace         string            `json:"namespace"`
	Labels            map[string]string `json:"labels"`
	Annotations       map[string]string `json:"annotations"`
	CreationTimestamp string            `json:"creationTimestamp"`
	OwnerReferences   []ownerReference  `json:"ownerReferences"`
}

type ownerReference struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type kubeNode struct {
	Metadata kubeObjectMeta `json:"metadata"`
	Status   struct {
		Conditions []struct {
			Type   string `json:"type"`
			Status string `json:"status"`
		} `json:"conditions"`
	} `json:"status"`
}

type kubePod struct {
	Metadata kubeObjectMeta `json:"metadata"`
	Spec     struct {
		NodeName   string `json:"nodeName"`
		Containers []struct {
			Name string `json:"name"`
		} `json:"containers"`
	} `json:"spec"`
	Status struct {
		Phase string `json:"phase"`
	} `json:"status"`
}

type kubeDeployment struct {
	Metadata kubeObjectMeta `json:"metadata"`
}

type kubeReplicaSet struct {
	Metadata kubeObjectMeta `json:"metadata"`
}

type server struct {
	staticDir string
	refresh   time.Duration
}

func main() {
	addr := envOr("CLUSTER_LENS_ADDR", defaultAddr)
	staticDir := envOr("CLUSTER_LENS_STATIC_DIR", defaultStaticDir)
	refresh := defaultRefresh
	if raw := strings.TrimSpace(os.Getenv("CLUSTER_LENS_REFRESH")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			log.Fatalf("CLUSTER_LENS_REFRESH must be a positive duration")
		}
		refresh = parsed
	}

	s := &server{staticDir: staticDir, refresh: refresh}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/snapshot", s.snapshot)
	mux.HandleFunc("/api/config", s.config)
	mux.Handle("/", http.FileServer(http.Dir(staticDir)))

	log.Printf("cluster-lens listening on http://%s", addr)
	log.Printf("serving frontend from %s", filepath.Clean(staticDir))
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

func (s *server) config(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"refreshMs": s.refresh.Milliseconds(),
	})
}

func (s *server) snapshot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	snap, err := buildSnapshot(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, snap)
}

func buildSnapshot(ctx context.Context) (snapshot, error) {
	var warnings []string

	contextName, err := kubectlText(ctx, "config", "current-context")
	if err != nil {
		return snapshot{}, err
	}

	nodes, err := kubectlJSON[kubeList[kubeNode]](ctx, "get", "nodes", "-o", "json")
	if err != nil {
		return snapshot{}, err
	}
	pods, err := kubectlJSON[kubeList[kubePod]](ctx, "get", "pods", "-A", "-o", "json")
	if err != nil {
		return snapshot{}, err
	}
	deployments, err := kubectlJSON[kubeList[kubeDeployment]](ctx, "get", "deployments", "-A", "-o", "json")
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	replicaSets, err := kubectlJSON[kubeList[kubeReplicaSet]](ctx, "get", "replicasets", "-A", "-o", "json")
	if err != nil {
		warnings = append(warnings, err.Error())
	}

	deploymentByKey := map[string]kubeDeployment{}
	for _, deployment := range deployments.Items {
		deploymentByKey[key(deployment.Metadata.Namespace, deployment.Metadata.Name)] = deployment
	}
	rsOwner := map[string]string{}
	for _, rs := range replicaSets.Items {
		for _, owner := range rs.Metadata.OwnerReferences {
			if owner.Kind == "Deployment" {
				rsOwner[key(rs.Metadata.Namespace, rs.Metadata.Name)] = owner.Name
				break
			}
		}
	}

	snap := snapshot{
		GeneratedAt: time.Now(),
		Context:     strings.TrimSpace(contextName),
		Nodes:       make([]nodeView, 0, len(nodes.Items)),
		NodeEdges:   make([]edgeView, 0),
		Pods:        make([]podView, 0, len(pods.Items)),
		AppEdges:    make([]edgeView, 0),
		Warnings:    warnings,
	}

	for _, node := range nodes.Items {
		annotations := node.Metadata.Annotations
		view := nodeView{
			Name:        node.Metadata.Name,
			Labels:      node.Metadata.Labels,
			Annotations: selectedAnnotations(annotations, "cpu-usage", "memory-usage", "disk-bandwidth", "network-bandwidth"),
			Ready:       nodeReady(node),
			Role:        nodeRole(node.Metadata.Labels),
			CPU:         parseFloat(annotations["cpu-usage"]),
			Memory:      parseFloat(annotations["memory-usage"]),
			Disk:        parseFloat(annotations["disk-bandwidth"]),
			Network:     parseFloat(annotations["network-bandwidth"]),
		}
		snap.Nodes = append(snap.Nodes, view)
		for annotation, raw := range annotations {
			if !strings.HasPrefix(annotation, "network-latency.") {
				continue
			}
			target := strings.TrimPrefix(annotation, "network-latency.")
			if target == "" || target == node.Metadata.Name {
				continue
			}
			latency := parseFloat(raw)
			snap.NodeEdges = append(snap.NodeEdges, edgeView{
				Source: node.Metadata.Name,
				Target: target,
				Kind:   "latency",
				Value:  latency,
				Label:  formatMS(latency),
			})
		}
	}

	for _, pod := range pods.Items {
		if pod.Spec.NodeName == "" {
			continue
		}
		owner := podOwnerDeployment(pod, rsOwner)
		metrics := map[string]string{}
		if owner != "" {
			if deployment, ok := deploymentByKey[key(pod.Metadata.Namespace, owner)]; ok {
				metrics = selectedMetricAnnotations(deployment.Metadata.Annotations)
				appendAppEdges(&snap, deployment)
			}
		}
		containers := make([]string, 0, len(pod.Spec.Containers))
		for _, container := range pod.Spec.Containers {
			containers = append(containers, container.Name)
		}
		sort.Strings(containers)

		labels := pod.Metadata.Labels
		snap.Pods = append(snap.Pods, podView{
			Namespace:  pod.Metadata.Namespace,
			Name:       pod.Metadata.Name,
			Node:       pod.Spec.NodeName,
			Phase:      pod.Status.Phase,
			Group:      labels["group"],
			App:        labelOr(labels, "app", owner),
			Owner:      owner,
			Labels:     labels,
			Metrics:    metrics,
			CreatedAt:  pod.Metadata.CreationTimestamp,
			Containers: containers,
		})
	}

	sort.Slice(snap.Nodes, func(i, j int) bool { return snap.Nodes[i].Name < snap.Nodes[j].Name })
	sort.Slice(snap.NodeEdges, func(i, j int) bool {
		if snap.NodeEdges[i].Source == snap.NodeEdges[j].Source {
			return snap.NodeEdges[i].Target < snap.NodeEdges[j].Target
		}
		return snap.NodeEdges[i].Source < snap.NodeEdges[j].Source
	})
	sort.Slice(snap.Pods, func(i, j int) bool {
		return key(snap.Pods[i].Namespace, snap.Pods[i].Name) < key(snap.Pods[j].Namespace, snap.Pods[j].Name)
	})

	snap.AppEdges = dedupeEdges(snap.AppEdges)
	return snap, nil
}

func appendAppEdges(snap *snapshot, deployment kubeDeployment) {
	app := labelOr(deployment.Metadata.Labels, "app", deployment.Metadata.Name)
	source := key(deployment.Metadata.Namespace, app)
	for annotation, raw := range deployment.Metadata.Annotations {
		if strings.HasPrefix(annotation, "rps.") {
			peer := strings.TrimPrefix(annotation, "rps.")
			value := parseFloat(raw)
			snap.AppEdges = append(snap.AppEdges, edgeView{
				Source: source,
				Target: key(deployment.Metadata.Namespace, peer),
				Kind:   "rps",
				Value:  value,
				Label:  formatFloat(value) + " rps",
			})
		}
		if strings.HasPrefix(annotation, "traffic.") {
			peer := strings.TrimPrefix(annotation, "traffic.")
			value := parseFloat(raw)
			snap.AppEdges = append(snap.AppEdges, edgeView{
				Source: source,
				Target: key(deployment.Metadata.Namespace, peer),
				Kind:   "traffic",
				Value:  value,
				Label:  formatBytes(value) + "/s",
			})
		}
	}
}

func podOwnerDeployment(pod kubePod, rsOwner map[string]string) string {
	for _, owner := range pod.Metadata.OwnerReferences {
		switch owner.Kind {
		case "Deployment":
			return owner.Name
		case "ReplicaSet":
			if deployment := rsOwner[key(pod.Metadata.Namespace, owner.Name)]; deployment != "" {
				return deployment
			}
			return owner.Name
		}
	}
	return ""
}

func nodeReady(node kubeNode) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == "Ready" {
			return condition.Status == "True"
		}
	}
	return false
}

func nodeRole(labels map[string]string) string {
	if labels["node-role.kubernetes.io/control-plane"] == "true" {
		return "control-plane"
	}
	if labels["nodepool"] != "" {
		return labels["nodepool"]
	}
	return "worker"
}

func selectedAnnotations(annotations map[string]string, names ...string) map[string]string {
	out := map[string]string{}
	for _, name := range names {
		if value := annotations[name]; value != "" {
			out[name] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func selectedMetricAnnotations(annotations map[string]string) map[string]string {
	out := selectedAnnotations(annotations, "cpu-usage", "memory-usage", "disk-bandwidth", "network-bandwidth")
	if out == nil {
		out = map[string]string{}
	}
	for name, value := range annotations {
		if strings.HasPrefix(name, "rps.") || strings.HasPrefix(name, "traffic.") {
			out[name] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func dedupeEdges(edges []edgeView) []edgeView {
	seen := map[string]edgeView{}
	for _, edge := range edges {
		seen[edge.Kind+"|"+edge.Source+"|"+edge.Target] = edge
	}
	out := make([]edgeView, 0, len(seen))
	for _, edge := range seen {
		out = append(out, edge)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Source == out[j].Source {
			if out[i].Target == out[j].Target {
				return out[i].Kind < out[j].Kind
			}
			return out[i].Target < out[j].Target
		}
		return out[i].Source < out[j].Source
	})
	return out
}

func kubectlJSON[T any](ctx context.Context, args ...string) (T, error) {
	var out T
	text, err := kubectlText(ctx, args...)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return out, fmt.Errorf("decode kubectl %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}

func kubectlText(ctx context.Context, args ...string) (string, error) {
	kctx, cancel := context.WithTimeout(ctx, kubectlTimeout)
	defer cancel()

	cmd := exec.CommandContext(kctx, "kubectl", args...)
	output, err := cmd.CombinedOutput()
	if errors.Is(kctx.Err(), context.DeadlineExceeded) {
		return "", fmt.Errorf("kubectl %s timed out", strings.Join(args, " "))
	}
	if err != nil {
		return "", fmt.Errorf("kubectl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func key(namespace, name string) string {
	return namespace + "/" + name
}

func labelOr(labels map[string]string, name, fallback string) string {
	if labels[name] != "" {
		return labels[name]
	}
	return fallback
}

func parseFloat(raw string) float64 {
	value, _ := strconv.ParseFloat(raw, 64)
	return value
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func formatMS(value float64) string {
	return strconv.FormatFloat(value, 'f', 3, 64) + " ms"
}

func formatBytes(value float64) string {
	const unit = 1024
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	next := value
	idx := 0
	for next >= unit && idx < len(units)-1 {
		next /= unit
		idx++
	}
	if idx == 0 {
		return strconv.FormatFloat(next, 'f', 0, 64) + " " + units[idx]
	}
	return strconv.FormatFloat(next, 'f', 1, 64) + " " + units[idx]
}
