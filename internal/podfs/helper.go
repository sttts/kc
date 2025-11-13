package podfs

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

type helperManager struct {
	client kubernetes.Interface
	cfg    helperConfig
}

type helperConfig struct {
	enabled          bool
	image            string
	command          []string
	namePrefix       string
	readinessTimeout time.Duration
}

func newHelperManager(client kubernetes.Interface) *helperManager {
	cfg := helperConfig{
		enabled:          true,
		image:            getenvDefault("KC_PODFS_HELPER_IMAGE", "docker.io/library/busybox:1.36.1"),
		command:          []string{"/bin/sh", "-c", "trap : TERM INT; while true; do sleep 3600; done"},
		namePrefix:       "kc-fs-helper",
		readinessTimeout: 60 * time.Second,
	}
	if strings.EqualFold(os.Getenv("KC_PODFS_HELPER_DISABLE"), "1") {
		cfg.enabled = false
	}
	return &helperManager{client: client, cfg: cfg}
}

func (m *helperManager) available() bool {
	return m != nil && m.cfg.enabled && m.cfg.image != ""
}

func (m *helperManager) ensureHelper(ctx context.Context, spec SessionSpec) (SessionSpec, error) {
	if !m.available() {
		return spec, fmt.Errorf("pod filesystem helper disabled")
	}
	pods := m.client.CoreV1().Pods(spec.Namespace)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		helperName := sanitizeHelperName(fmt.Sprintf("%s-%s-%d", m.cfg.namePrefix, spec.Container, attempt))
		pod, err := pods.Get(ctx, spec.Pod, metav1.GetOptions{})
		if err != nil {
			return spec, err
		}
		if !helperExists(pod, helperName) {
			if err := m.createHelper(ctx, pods, pod, spec.Container, helperName); err != nil {
				lastErr = err
				continue
			}
		}
		if err := m.waitForHelper(ctx, spec.Namespace, spec.Pod, helperName); err != nil {
			lastErr = err
			if ctx.Err() != nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
			continue
		}
		return SessionSpec{Namespace: spec.Namespace, Pod: spec.Pod, Container: helperName}, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("failed to start debug helper after retries")
	}
	return spec, lastErr
}

func (m *helperManager) waitForHelper(ctx context.Context, namespace, podName, helperName string) error {
	timeout := m.cfg.readinessTimeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return wait.PollUntilContextTimeout(ctx, 500*time.Millisecond, timeout, true, func(ctx context.Context) (bool, error) {
		pod, err := m.client.CoreV1().Pods(namespace).Get(ctx, podName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, status := range pod.Status.EphemeralContainerStatuses {
			if status.Name != helperName {
				continue
			}
			if status.State.Running != nil {
				return true, nil
			}
			if status.State.Terminated != nil {
				reason := strings.TrimSpace(status.State.Terminated.Reason)
				message := strings.TrimSpace(status.State.Terminated.Message)
				detail := reason
				if detail == "" {
					detail = "unknown"
				}
				if message != "" {
					detail = fmt.Sprintf("%s: %s", detail, message)
				}
				return false, fmt.Errorf("debug helper terminated: %s", detail)
			}
		}
		return false, nil
	})
}

func helperExists(pod *corev1.Pod, name string) bool {
	for _, ec := range pod.Spec.EphemeralContainers {
		if ec.Name == name {
			return true
		}
	}
	return false
}

var helperNameSanitizer = regexp.MustCompile(`[^a-z0-9-]+`)

func sanitizeHelperName(input string) string {
	s := strings.ToLower(input)
	s = helperNameSanitizer.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) == 0 {
		return "kc-fs-helper"
	}
	if len(s) <= 63 {
		return s
	}
	return s[:63]
}

func getenvDefault(key, def string) string {
	if val := strings.TrimSpace(os.Getenv(key)); val != "" {
		return val
	}
	return def
}

func (m *helperManager) createHelper(ctx context.Context, pods typedcorev1.PodInterface, pod *corev1.Pod, targetContainer, helperName string) error {
	ec := corev1.EphemeralContainer{
		EphemeralContainerCommon: corev1.EphemeralContainerCommon{
			Name:            helperName,
			Image:           m.cfg.image,
			Command:         append([]string(nil), m.cfg.command...),
			ImagePullPolicy: corev1.PullIfNotPresent,
			Stdin:           true,
			TTY:             true,
		},
		TargetContainerName: targetContainer,
	}
	pod.Spec.EphemeralContainers = append(pod.Spec.EphemeralContainers, ec)
	_, err := pods.UpdateEphemeralContainers(ctx, pod.Name, pod, metav1.UpdateOptions{})
	return err
}
