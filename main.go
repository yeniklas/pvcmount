package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

func main() {
	var namespace string
	flag.StringVar(&namespace, "namespace", "", "Kubernetes namespace (defaults to current context namespace)")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: pvcmount [--namespace <ns>] <pvc-name> <mountpoint>\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(1)
	}

	pvcName := flag.Arg(0)
	mountpoint, err := filepath.Abs(flag.Arg(1))
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid mountpoint: %v\n", err)
		os.Exit(1)
	}

	if info, err := os.Stat(mountpoint); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "mountpoint %q must be an existing directory\n", mountpoint)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, pvcName, namespace, mountpoint); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, pvcName, namespace, mountpoint string) error {
	client, err := NewClient(namespace)
	if err != nil {
		return fmt.Errorf("kubernetes client: %w", err)
	}

	pvcInfo, err := client.InspectPVC(ctx, pvcName)
	if err != nil {
		return err
	}
	if pvcInfo.NodeName != "" {
		fmt.Printf("PVC is ReadWriteOnce, pinning pod to node %s\n", pvcInfo.NodeName)
	}

	kp, err := GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("generate ssh key: %w", err)
	}

	podName, err := client.EnsurePod(ctx, pvcInfo, kp.AuthorizedLine)
	if err != nil {
		return err
	}
	defer func() {
		fmt.Printf("deleting pod %s\n", podName)
		_ = client.DeletePod(context.Background(), podName)
	}()

	if err := client.WaitPodReady(ctx, podName); err != nil {
		return fmt.Errorf("pod not ready: %w", err)
	}

	localPort, stopForward, err := client.StartPortForward(ctx, podName)
	if err != nil {
		return fmt.Errorf("port-forward: %w", err)
	}
	defer stopForward()

	keyFile, keyCleanup, err := WriteKeyFile(kp)
	if err != nil {
		return fmt.Errorf("write key file: %w", err)
	}
	defer keyCleanup()

	sshfsCmd, err := SSHFS(mountpoint, localPort, keyFile)
	if err != nil {
		return fmt.Errorf("sshfs: %w", err)
	}
	defer func() {
		_ = Unmount(mountpoint)
		if sshfsCmd.Process != nil {
			_ = sshfsCmd.Process.Kill()
		}
	}()

	fmt.Printf("\nMounted PVC %q at %s\nPress Ctrl-C to unmount.\n", pvcName, mountpoint)

	WaitUntilUnmounted(ctx, mountpoint)
	return nil
}
