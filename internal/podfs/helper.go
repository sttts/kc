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
	helperName := sanitizeHelperName(fmt.Sprintf("%s-%s", m.cfg.namePrefix, spec.Container))
	pods := m.client.CoreV1().Pods(spec.Namespace)
	pod, err := pods.Get(ctx, spec.Pod, metav1.GetOptions{})
	if err != nil {
		return spec, err
	}
	if !helperExists(pod, helperName) {
		ec := corev1.EphemeralContainer{
			EphemeralContainerCommon: corev1.EphemeralContainerCommon{
				Name:            helperName,
				Image:           m.cfg.image,
				Command:         append([]string(nil), m.cfg.command...),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Stdin:           true,
				TTY:             true,
			},
			TargetContainerName: spec.Container,
		}
		pod.Spec.EphemeralContainers = append(pod.Spec.EphemeralContainers, ec)
		if _, err := pods.UpdateEphemeralContainers(ctx, spec.Pod, pod, metav1.UpdateOptions{}); err != nil {
			return spec, err
		}
	}
	if err := m.waitForHelper(ctx, spec.Namespace, spec.Pod, helperName); err != nil {
		return spec, err
	}
	return SessionSpec{
		Namespace: spec.Namespace,
		Pod:       spec.Pod,
		Container: helperName,
	}, nil
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
				return false, fmt.Errorf("debug helper terminated: %s", status.State.Terminated.Message)
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
