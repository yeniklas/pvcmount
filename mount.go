package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func SSHFS(mountpoint string, localPort uint16, keyFile string) (*exec.Cmd, error) {
	args := []string{
		fmt.Sprintf("root@127.0.0.1:/data"),
		mountpoint,
		"-p", fmt.Sprintf("%d", localPort),
		"-o", "IdentityFile=" + keyFile,
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "reconnect",
		"-o", "auto_unmount",
		"-f",
	}

	cmd := exec.Command("sshfs", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start sshfs: %w", err)
	}

	// Give sshfs a moment to mount before returning.
	time.Sleep(time.Second)

	if !isMounted(mountpoint) {
		cmd.Process.Kill()
		return nil, fmt.Errorf("sshfs failed to mount %s", mountpoint)
	}

	return cmd, nil
}

func WaitUntilUnmounted(ctx context.Context, mountpoint string) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
			if !isMounted(mountpoint) {
				return
			}
		}
	}
}

func Unmount(mountpoint string) error {
	if err := exec.Command("fusermount3", "-u", mountpoint).Run(); err == nil {
		return nil
	}
	return exec.Command("umount", mountpoint).Run()
}

func isMounted(mountpoint string) bool {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return false
	}
	defer f.Close()

	abs := mountpoint
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 2 && fields[1] == abs {
			return true
		}
	}
	return false
}
