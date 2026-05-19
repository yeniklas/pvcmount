//go:build e2e

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	e2eNamespace = "pvcmount-e2e"
	e2ePVCName   = "pvcmount-e2e-data"
	e2eSshdImage = "ghcr.io/yeniklas/pvcmount-sshd:latest"
)

// TestMain wires -v → debug so sshd dot-progress and port-forward info appear
// in verbose runs.
func TestMain(m *testing.M) {
	flag.Parse()
	debug = testing.Verbose()
	os.Exit(m.Run())
}

func TestE2E_Mount(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	client, err := NewClient(e2eNamespace, "")
	if err != nil {
		t.Fatalf("connect to cluster: %v", err)
	}

	ensureNamespace(ctx, t, client)
	ensurePVC(ctx, t, client)

	mountpoint := t.TempDir()

	mountCtx, mountCancel := context.WithCancel(ctx)
	runDone := make(chan error, 1)
	go func() {
		runDone <- run(mountCtx, e2ePVCName, e2eNamespace, "", mountpoint, false, e2eSshdImage)
	}()

	// Poll for the FUSE mount to appear.
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		if isMounted(mountpoint) {
			break
		}
		select {
		case err := <-runDone:
			t.Fatalf("run() exited before mount appeared: %v", err)
		case <-time.After(500 * time.Millisecond):
		}
	}
	if !isMounted(mountpoint) {
		mountCancel()
		t.Fatal("timed out waiting for FUSE mount")
	}
	t.Log("mount is up — verifying R/W")

	testFile := filepath.Join(mountpoint, "pvcmount-e2e.txt")
	want := fmt.Sprintf("pvcmount e2e %d", time.Now().UnixNano())
	if err := os.WriteFile(testFile, []byte(want), 0644); err != nil {
		mountCancel()
		t.Fatalf("write: %v", err)
	}
	got, err := os.ReadFile(testFile)
	if err != nil {
		mountCancel()
		t.Fatalf("read: %v", err)
	}
	if string(got) != want {
		mountCancel()
		t.Fatalf("content mismatch: got %q, want %q", got, want)
	}

	t.Log("R/W OK — unmounting")
	mountCancel()

	if err := <-runDone; err != nil && err != context.Canceled {
		t.Fatalf("run() returned unexpected error: %v", err)
	}
}

func ensureNamespace(ctx context.Context, t *testing.T, c *Client) {
	t.Helper()
	_, err := c.cs.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: e2eNamespace},
	}, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("create namespace: %v", err)
	}
}

func ensurePVC(ctx context.Context, t *testing.T, c *Client) {
	t.Helper()
	_, err := c.cs.CoreV1().PersistentVolumeClaims(e2eNamespace).Create(ctx,
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: e2ePVCName, Namespace: e2eNamespace},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		}, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		t.Fatalf("create PVC: %v", err)
	}
}
