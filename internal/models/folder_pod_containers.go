package models

import (
	"context"
	"fmt"

	table "github.com/sttts/kc/internal/table"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// PodContainersFolder lists high-level container categories for a pod (main/init/ephemeral).
type PodContainersFolder struct {
	*BaseFolder
	Namespace string
	Pod       string
}

// NewPodContainersFolder constructs the pod containers folder.
func NewPodContainersFolder(deps Deps, parentPath []string, namespace, pod string) *PodContainersFolder {
	path := append([]string{}, parentPath...)
	cols := []table.Column{{Title: " Name"}}
	base := NewBaseFolder(deps, cols, path)
	folder := &PodContainersFolder{BaseFolder: base, Namespace: namespace, Pod: pod}
	rows := newPodSectionRowSource(deps, namespace, pod, folder.buildRows, folder.BaseFolder.markDirtyFromSource)
	base.SetRowSource(rows)
	return folder
}

func (f *PodContainersFolder) buildRows(ctx context.Context) ([]table.Row, error) {
	podObj, err := f.fetchPod(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]table.Row, 0, 3)
	sections := []struct {
		id     string
		label  string
		detail string
		count  int
		kind   containerKind
	}{
		{id: "containers", label: "/containers", detail: "main containers", count: len(podObj.Spec.Containers), kind: containerKindPrimary},
		{id: "init", label: "/init-containers", detail: "init containers", count: len(podObj.Spec.InitContainers), kind: containerKindInit},
		{id: "ephemeral", label: "/ephemeral-containers", detail: "ephemeral containers", count: len(podObj.Spec.EphemeralContainers), kind: containerKindEphemeral},
	}
	for _, section := range sections {
		if section.count == 0 {
			continue
		}
		sectionPath := append(append([]string{}, f.Path()...), section.id)
		detail := fmt.Sprintf("%d %s", section.count, section.detail)
		item := NewContainerSectionItem(section.id, []string{section.label}, sectionPath, WhiteStyle(), func() (Folder, error) {
			return NewPodContainerListFolder(f.Deps, sectionPath, f.Namespace, f.Pod, section.kind), nil
		})
		item.RowItem.details = detail
		rows = append(rows, item)
	}
	return rows, nil
}

func (f *PodContainersFolder) fetchPod(ctx context.Context) (*corev1.Pod, error) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	obj, err := f.Deps.Cl.GetByGVR(ctx, gvr, f.Namespace, f.Pod)
	if err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, fmt.Errorf("pod %s/%s not found", f.Namespace, f.Pod)
	}
	var pod corev1.Pod
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &pod); err != nil {
		return nil, err
	}
	return &pod, nil
}

// containerKind identifies which pod container slice to read.
type containerKind int

const (
	containerKindPrimary containerKind = iota
	containerKindInit
	containerKindEphemeral
)

// PodContainerListFolder lists containers for a specific category (main/init/ephemeral).
type PodContainerListFolder struct {
	*BaseFolder
	Namespace string
	Pod       string
	Kind      containerKind
}

func NewPodContainerListFolder(deps Deps, path []string, namespace, pod string, kind containerKind) *PodContainerListFolder {
	base := NewBaseFolder(deps, []table.Column{{Title: " Name"}}, path)
	folder := &PodContainerListFolder{BaseFolder: base, Namespace: namespace, Pod: pod, Kind: kind}
	rows := newPodContainerRowSource(deps, namespace, pod, kind, folder.buildRows, folder.BaseFolder.markDirtyFromSource)
	base.SetRowSource(rows)
	return folder
}

func (f *PodContainerListFolder) buildRows(ctx context.Context) ([]table.Row, error) {
	podObj, err := f.fetchPod(ctx)
	if err != nil {
		return nil, err
	}
	rows := make([]table.Row, 0)
	nameStyle := WhiteStyle()
	containers := f.extractContainers(podObj)
	for _, c := range containers {
		sectionPath := append(append([]string{}, f.Path()...), c.Name)
		item := NewContainerItem(c.Name, []string{c.Label}, sectionPath, nameStyle, containerViewContent(f.Deps, f.Namespace, f.Pod, c.Name), func() (Folder, error) {
			logsPath := append(append([]string{}, sectionPath...), "logs")
			return NewPodContainerLogsFolder(f.Deps, logsPath, f.Namespace, f.Pod, c.Name), nil
		})
		item.RowItem.details = c.Detail
		rows = append(rows, item)
	}
	return rows, nil
}

func (f *PodContainerListFolder) extractContainers(pod *corev1.Pod) []containerRecord {
	records := make([]containerRecord, 0)
	switch f.Kind {
	case containerKindPrimary:
		for _, c := range pod.Spec.Containers {
			records = append(records, containerRecord{Name: c.Name, Label: "/" + c.Name, Detail: c.Image})
		}
	case containerKindInit:
		for _, c := range pod.Spec.InitContainers {
			records = append(records, containerRecord{Name: c.Name, Label: "/" + c.Name, Detail: "init"})
		}
	case containerKindEphemeral:
		for _, c := range pod.Spec.EphemeralContainers {
			records = append(records, containerRecord{Name: c.Name, Label: "/" + c.Name, Detail: "ephemeral"})
		}
	}
	return records
}

func (f *PodContainerListFolder) fetchPod(ctx context.Context) (*corev1.Pod, error) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	obj, err := f.Deps.Cl.GetByGVR(ctx, gvr, f.Namespace, f.Pod)
	if err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, fmt.Errorf("pod %s/%s not found", f.Namespace, f.Pod)
	}
	var pod corev1.Pod
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, &pod); err != nil {
		return nil, err
	}
	return &pod, nil
}

type containerRecord struct {
	Name   string
	Label  string
	Detail string
}

// PodContainerLogsFolder provides log entries for a specific container.
type PodContainerLogsFolder struct {
	*BaseFolder
	Namespace string
	Pod       string
	Container string
}

func NewPodContainerLogsFolder(deps Deps, path []string, namespace, pod, container string) *PodContainerLogsFolder {
	base := NewBaseFolder(deps, []table.Column{{Title: " Name"}}, path)
	folder := &PodContainerLogsFolder{BaseFolder: base, Namespace: namespace, Pod: pod, Container: container}
	rows := newPodContainerLogRowSource(deps, namespace, pod, container, folder.buildRows, folder.BaseFolder.markDirtyFromSource)
	base.SetRowSource(rows)
	return folder
}

func (f *PodContainerLogsFolder) buildRows(context.Context) ([]table.Row, error) {
	rows := make([]table.Row, 0, 1)
	item := NewContainerLogItem("latest", []string{"logs"}, append(append([]string{}, f.Path()...), "latest"), containerLogsViewContent(f.Deps, f.Namespace, f.Pod, f.Container, 200))
	rows = append(rows, item)
	return rows, nil
}

func newPodSectionRowSource(deps Deps, namespace, pod string, populate func(context.Context) ([]table.Row, error), onDirty func()) rowSource {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	return newLiveObjectRowSourceWithHooks(populate, onDirty, func(cb func()) {
		startInformerForResource(deps, gvr, namespace, pod, cb)
	})
}

func newPodContainerRowSource(deps Deps, namespace, pod string, kind containerKind, populate func(context.Context) ([]table.Row, error), onDirty func()) rowSource {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	return newLiveObjectRowSourceWithHooks(populate, onDirty, func(cb func()) {
		startInformerForResource(deps, gvr, namespace, pod, cb)
	})
}

func newPodContainerLogRowSource(deps Deps, namespace, pod, container string, populate func(context.Context) ([]table.Row, error), onDirty func()) rowSource {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	return newLiveObjectRowSourceWithHooks(populate, onDirty, func(cb func()) {
		startInformerForResource(deps, gvr, namespace, pod, cb)
	})
}
