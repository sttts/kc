package ui

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestChoosePodContainer_DefaultSingle(t *testing.T) {
	pod := &corev1.Pod{}
	pod.Spec.Containers = []corev1.Container{{Name: "app"}}
	target, err := choosePodContainer(pod, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.Container != "app" || target.SectionID != logSectionContainers {
		t.Fatalf("unexpected target %#v", target)
	}
}

func TestChoosePodContainer_RequireFlag(t *testing.T) {
	pod := &corev1.Pod{}
	pod.Spec.Containers = []corev1.Container{{Name: "a"}, {Name: "b"}}
	if _, err := choosePodContainer(pod, ""); err == nil {
		t.Fatalf("expected error when multiple containers without -c")
	}
}

func TestChoosePodContainer_SpecificInit(t *testing.T) {
	pod := &corev1.Pod{}
	pod.Spec.InitContainers = []corev1.Container{{Name: "init-a"}}
	target, err := choosePodContainer(pod, "init-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.SectionID != logSectionInit {
		t.Fatalf("expected init section, got %#v", target)
	}
}
