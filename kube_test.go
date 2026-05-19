package main

import (
	"strings"
	"testing"
)

func TestPodName(t *testing.T) {
	tests := []struct {
		pvc  string
		want string
	}{
		// Golden values — changing the hash algorithm will break these.
		{"stash-data", "pvcmount-8d0c56fd"},
		{"my-pvc", "pvcmount-d85250fe"},
		{"", "pvcmount-e3b0c442"},
	}

	for _, tt := range tests {
		got := PodName(tt.pvc)
		if got != tt.want {
			t.Errorf("PodName(%q) = %q, want %q", tt.pvc, got, tt.want)
		}
	}
}

func TestPodName_format(t *testing.T) {
	name := PodName("anything")

	if !strings.HasPrefix(name, "pvcmount-") {
		t.Errorf("missing pvcmount- prefix: %q", name)
	}

	suffix := strings.TrimPrefix(name, "pvcmount-")
	if len(suffix) != 8 {
		t.Errorf("expected 8 hex chars, got %d in %q", len(suffix), name)
	}
	for _, c := range suffix {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Errorf("non-hex char %q in pod name %q", c, name)
		}
	}
}

func TestPodName_unique(t *testing.T) {
	names := map[string]string{}
	pvcs := []string{"pvc-a", "pvc-b", "pvc-c", "foo", "bar"}
	for _, pvc := range pvcs {
		n := PodName(pvc)
		if prev, seen := names[n]; seen {
			t.Errorf("collision: PodName(%q) == PodName(%q) == %q", pvc, prev, n)
		}
		names[n] = pvc
	}
}
