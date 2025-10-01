package models

import table "github.com/sttts/kc/internal/table"

// PodContainersFolder lists containers and initContainers for a pod.
type PodContainersFolder struct {
	*BaseFolder
	Namespace string
	Pod       string
}

// NewPodContainersFolder constructs the pod containers folder.
func NewPodContainersFolder(deps Deps, parentPath []string, namespace, pod string) *PodContainersFolder {
	path := append(append([]string{}, parentPath...), "containers")
	key := composeKey(deps, path)
	cols := []table.Column{{Title: " Name"}}
	base := NewBaseFolder(deps, cols, path, key, nil)
	return &PodContainersFolder{BaseFolder: base, Namespace: namespace, Pod: pod}
}
