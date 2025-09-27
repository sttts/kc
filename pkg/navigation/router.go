package navigation

import (
	"fmt"
	"strings"
)

// Constants for top-level entry points.
const (
	RootCluster     = "/cluster"
	RootContexts    = "/contexts"
	RootKubeconfigs = "/kubeconfigs"
	RootGroups      = "/groups" // optional
)

// Path represents a normalized kc navigation path.
// It avoids re-wrapping Kubernetes types and only captures routing parameters.
type Path struct {
	Raw string // original string, preserved for display
	// Context & scope
	Kubeconfig string // basename or identifier for kubeconfig when applicable
	Context    string
	Namespace  string
	// API identity
	Group    string
	Version  string
	Resource string // plural (e.g., pods, configmaps)
	Name     string // object name when applicable
}

// Router parses and formats kc navigation paths. It does not fetch data.
// Implementations should be pure and side-effect free.
type Router interface {
	// Parse converts a string path to a structured Path.
	Parse(raw string) (Path, error)
	// Build renders a Path back to a canonical string.
	Build(p Path) string
	// Parent returns the parent path (or the same path at root).
	Parent(p Path) Path
}

// SimpleRouter is a minimal canonical implementation covering the documented scheme.
// It is intentionally small; hierarchy semantics live in higher layers.
type SimpleRouter struct{}

func NewSimpleRouter() *SimpleRouter { return &SimpleRouter{} }

// Parse implements a conservative parser that recognizes documented roots.
func (r *SimpleRouter) Parse(raw string) (Path, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "/" {
		return Path{Raw: "/"}, nil
	}
	segs := splitClean(raw)
	p := Path{Raw: raw}
	if len(segs) == 0 {
		return p, nil
	}

	switch "/" + segs[0] {
	case RootCluster:
		// /cluster[/namespaces/<ns>/<resource>[/<name>]] or /cluster/<resource>[/<name>]
		if len(segs) >= 2 && segs[1] == "namespaces" {
			if len(segs) >= 3 {
				p.Namespace = segs[2]
			}
			if len(segs) >= 4 {
				p.Resource = segs[3]
			}
			if len(segs) >= 5 {
				p.Name = segs[4]
			}
		} else if len(segs) >= 2 {
			p.Resource = segs[1]
			if len(segs) >= 3 {
				p.Name = segs[2]
			}
		}
		return p, nil
	case RootContexts:
		// /contexts/<ctx>/... mirrors /cluster under the context
		if len(segs) >= 2 {
			p.Context = segs[1]
		}
		// Remaining path: if it already starts with a known root (cluster/groups), pass through; otherwise assume cluster root.
		tail := []string{}
		if len(segs) > 2 {
			tail = segs[2:]
		}
		if len(tail) == 0 {
			return p, nil
		}
		if tail[0] != "cluster" && tail[0] != "groups" {
			tail = append([]string{"cluster"}, tail...)
		}
		q, _ := r.Parse("/" + strings.Join(tail, "/"))
		q.Raw = raw
		q.Context = p.Context
		return q, nil
	case RootKubeconfigs:
		// /kubeconfigs/<kc>/contexts/<ctx>/... (future: add dialogs)
		if len(segs) >= 2 {
			p.Kubeconfig = segs[1]
		}
		if len(segs) >= 4 && segs[2] == "contexts" {
			p.Context = segs[3]
		}
		// Treat remaining as cluster-relative
		rem := append([]string{"cluster"}, segs[4:]...)
		q, _ := r.Parse("/" + strings.Join(rem, "/"))
		q.Raw = raw
		q.Kubeconfig = p.Kubeconfig
		q.Context = p.Context
		return q, nil
	case RootGroups:
		// /groups/<group>/<version>/[namespaces/<ns>/]<resource>[/<name>]
		if len(segs) >= 2 {
			p.Group = segs[1]
		}
		if len(segs) >= 3 {
			p.Version = segs[2]
		}
		i := 3
		if len(segs) >= 5 && segs[3] == "namespaces" {
			p.Namespace = segs[4]
			i = 5
		}
		if len(segs) > i {
			p.Resource = segs[i]
		}
		if len(segs) > i+1 {
			p.Name = segs[i+1]
		}
		return p, nil
	default:
		return p, fmt.Errorf("unrecognized root: %s", segs[0])
	}
}

func (r *SimpleRouter) Build(p Path) string {
	// Prefer group-mode if Group/Version specified
	if p.Group != "" || p.Version != "" {
		parts := []string{"groups", p.Group, p.Version}
		if p.Namespace != "" {
			parts = append(parts, "namespaces", p.Namespace)
		}
		if p.Resource != "" {
			parts = append(parts, p.Resource)
		}
		if p.Name != "" {
			parts = append(parts, p.Name)
		}
		return "/" + strings.Join(filterEmpty(parts), "/")
	}
	// Default cluster-mode with optional namespace
	parts := []string{"cluster"}
	if p.Namespace != "" {
		parts = append(parts, "namespaces", p.Namespace)
	}
	if p.Resource != "" {
		parts = append(parts, p.Resource)
	}
	if p.Name != "" {
		parts = append(parts, p.Name)
	}
	return "/" + strings.Join(parts, "/")
}

func (r *SimpleRouter) Parent(p Path) Path {
	// Trim by precedence: Name -> Resource -> Namespace -> Version -> Group -> Context/Kubeconfig
	if p.Name != "" {
		p.Name = ""
		return p
	}
	if p.Resource != "" {
		p.Resource = ""
		return p
	}
	if p.Namespace != "" {
		p.Namespace = ""
		return p
	}
	if p.Version != "" {
		p.Version = ""
		return p
	}
	if p.Group != "" {
		p.Group = ""
		return p
	}
	// Collapse to roots
	p.Context = ""
	p.Kubeconfig = ""
	p.Raw = "/"
	return p
}

func splitClean(s string) []string {
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimSuffix(s, "/")
	if s == "" {
		return nil
	}
	return strings.Split(s, "/")
}

func filterEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
