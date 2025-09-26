package handlers

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PodHandler handles Pod-specific operations
type PodHandler struct {
	base *BaseHandler
}

// NewPodHandler creates a new Pod handler
func NewPodHandler() *PodHandler {
	return &PodHandler{
		base: NewBaseHandler(),
	}
}

// GetActions returns the list of actions available for Pod resources
func (h *PodHandler) GetActions(obj client.Object) []Action {
	// Get generic actions first
	gvk := obj.GetObjectKind().GroupVersionKind()
	actions := h.base.GetGenericActions(obj, gvk)

	// Add pod-specific actions
	podActions := []Action{
		{
			Name:                 "Logs",
			Description:          "View pod logs",
			Command:              "kubectl",
			Args:                 []string{"logs", obj.GetName(), "-n", obj.GetNamespace()},
			RequiresConfirmation: false,
		},
		{
			Name:                 "Exec",
			Description:          "Execute command in pod",
			Command:              "kubectl",
			Args:                 []string{"exec", "-it", obj.GetName(), "-n", obj.GetNamespace(), "--", "/bin/sh"},
			RequiresConfirmation: false,
		},
	}

	return append(actions, podActions...)
}

// GetSubResources returns sub-resources available for Pod resources
func (h *PodHandler) GetSubResources(obj client.Object) []SubResource {
	return []SubResource{
		{
			Name:        "logs",
			Description: "View pod logs",
			Handler:     &PodLogsHandler{},
		},
		{
			Name:        "exec",
			Description: "Execute command in pod",
			Handler:     &PodExecHandler{},
		},
	}
}

// GetDisplayColumns returns the columns to display for Pod resources
func (h *PodHandler) GetDisplayColumns() []DisplayColumn {
	// Get generic columns first
	columns := h.base.GetGenericDisplayColumns()

	// Add pod-specific columns
	podColumns := []DisplayColumn{
		{
			Name:     "Status",
			Width:    15,
			Getter:   func(obj client.Object) string { return h.GetStatus(obj) },
			Sortable: true,
		},
		{
			Name:     "Ready",
			Width:    10,
			Getter:   func(obj client.Object) string { return h.getReadyStatus(obj) },
			Sortable: true,
		},
		{
			Name:     "Restarts",
			Width:    10,
			Getter:   func(obj client.Object) string { return h.getRestartCount(obj) },
			Sortable: true,
		},
	}

	return append(columns, podColumns...)
}

// GetStatus returns the status string for a Pod resource
func (h *PodHandler) GetStatus(obj client.Object) string {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return "Unknown"
	}

	switch pod.Status.Phase {
	case corev1.PodPending:
		return "Pending"
	case corev1.PodRunning:
		return "Running"
	case corev1.PodSucceeded:
		return "Succeeded"
	case corev1.PodFailed:
		return "Failed"
	case corev1.PodUnknown:
		return "Unknown"
	default:
		return string(pod.Status.Phase)
	}
}

// getReadyStatus returns the ready status of the pod
func (h *PodHandler) getReadyStatus(obj client.Object) string {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return "0/0"
	}

	ready := 0
	total := len(pod.Spec.Containers)

	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			ready = total
			break
		}
	}

	return fmt.Sprintf("%d/%d", ready, total)
}

// getRestartCount returns the total restart count for the pod
func (h *PodHandler) getRestartCount(obj client.Object) string {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return "0"
	}

	totalRestarts := int32(0)
	for _, containerStatus := range pod.Status.ContainerStatuses {
		totalRestarts += containerStatus.RestartCount
	}

	return fmt.Sprintf("%d", totalRestarts)
}

// PodLogsHandler handles pod logs sub-resource
type PodLogsHandler struct{}

// Execute performs the logs operation
func (h *PodLogsHandler) Execute(obj client.Object) error {
	// This would execute kubectl logs command
	return fmt.Errorf("not implemented")
}

// GetContent returns the logs content for viewing
func (h *PodLogsHandler) GetContent(obj client.Object) (string, error) {
	// This would fetch logs via kubectl or API
	return fmt.Sprintf("Logs for pod %s", obj.GetName()), nil
}

// PodExecHandler handles pod exec sub-resource
type PodExecHandler struct{}

// Execute performs the exec operation
func (h *PodExecHandler) Execute(obj client.Object) error {
	// This would execute kubectl exec command
	return fmt.Errorf("not implemented")
}

// GetContent returns the exec content for viewing
func (h *PodExecHandler) GetContent(obj client.Object) (string, error) {
	// This would return exec interface
	return fmt.Sprintf("Exec interface for pod %s", obj.GetName()), nil
}
