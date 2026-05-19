package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	klog "k8s.io/klog/v2"
)

var version = "dev"

func main() {
	klog.SetOutput(io.Discard)

	// Handle subcommands before flag parsing so `flag` doesn't choke on unknown args.
	if len(os.Args) >= 2 {
		switch os.Args[1] {
		case "completion":
			shell := "zsh"
			if len(os.Args) >= 3 {
				shell = os.Args[2]
			}
			runCompletion(shell)
			os.Exit(0)
		case "__list-pvcs":
			runListPVCs(os.Args[2:])
			os.Exit(0)
		}
	}

	var namespace string
	var mountpointFlag string
	var sshdImage string
	var versionFlag bool
	var updateFlag bool
	flag.StringVar(&namespace, "namespace", "", "Kubernetes namespace (defaults to current context namespace)")
	flag.StringVar(&mountpointFlag, "mountpoint", "", "local directory to mount into (default: ~/pvcmount/<pvc-name>-<random>)")
	flag.StringVar(&sshdImage, "sshd-image", "ghcr.io/yeniklas/pvcmount-sshd:latest", "container image for the temporary sshd pod")
	flag.BoolVar(&versionFlag, "version", false, "print version and exit")
	flag.BoolVar(&updateFlag, "self-update", false, "update pvcmount to the latest release")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: pvcmount [--namespace <ns>] [--mountpoint <path>] <pvc-name>\n")
		fmt.Fprintf(os.Stderr, "       pvcmount completion zsh\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	if versionFlag {
		fmt.Println(version)
		os.Exit(0)
	}

	if updateFlag {
		if err := runSelfUpdate(version); err != nil {
			fmt.Fprintln(os.Stderr, "update:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(1)
	}

	pvcName := flag.Arg(0)

	mountpoint, autoCreated, err := resolveMountpoint(pvcName, mountpointFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, pvcName, namespace, mountpoint, autoCreated, sshdImage); err != nil && err != context.Canceled {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func resolveMountpoint(pvcName, override string) (path string, autoCreated bool, err error) {
	if override != "" {
		info, err := os.Stat(override)
		if err != nil || !info.IsDir() {
			return "", false, fmt.Errorf("mountpoint %q must be an existing directory", override)
		}
		abs, _ := filepath.Abs(override)
		return abs, false, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", false, fmt.Errorf("resolve home dir: %w", err)
	}
	base := filepath.Join(home, "pvcmount")
	if err := os.MkdirAll(base, 0755); err != nil {
		return "", false, fmt.Errorf("create ~/pvcmount: %w", err)
	}

	var buf [2]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", false, fmt.Errorf("generate random suffix: %w", err)
	}
	dir := filepath.Join(base, fmt.Sprintf("%s-%x", pvcName, buf))
	if err := os.Mkdir(dir, 0755); err != nil {
		return "", false, fmt.Errorf("create mountpoint: %w", err)
	}
	return dir, true, nil
}

func run(ctx context.Context, pvcName, namespace, mountpoint string, autoCreated bool, sshdImage string) error {
	if autoCreated {
		defer os.Remove(mountpoint)
	}

	client, err := NewClient(namespace)
	if err != nil {
		return fmt.Errorf("kubernetes client: %w", err)
	}

	fmt.Printf("\n%s  %s\n\n", styleBold("Mounting "+pvcName), styleDim("·  "+client.namespace))

	pvcInfo, err := client.InspectPVC(ctx, pvcName)
	if err != nil {
		return err
	}

	kp, err := GenerateKeyPair()
	if err != nil {
		return fmt.Errorf("generate ssh key: %w", err)
	}

	podName, err := client.EnsurePod(ctx, pvcInfo, kp.AuthorizedLine, sshdImage)
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
	stepDone("Pod ready")

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

	fmt.Printf("\nMounted at %s\n%s\n", styleBold(mountpoint), styleDim("Press Ctrl+C to unmount."))

	WaitUntilUnmounted(ctx, mountpoint)
	if ctx.Err() == nil {
		// Unmounted without Ctrl-C — sshfs/sshd dropped the connection.
		if logs, _ := client.podLogs(podName); logs != "" {
			fmt.Printf("sshd logs:\n%s\n", logs)
		}
		return fmt.Errorf("sshfs connection dropped unexpectedly")
	}
	return nil
}
