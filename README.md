# pvcmount

Mount a Kubernetes PersistentVolumeClaim as a local filesystem on Linux.

```
pvcmount [flags] <pvc-name>
```

```
Mounting my-pvc  ·  default

  ✓ Pod ready
  ✓ sshd ready

Mounted at ~/pvcmount/my-pvc-3a8f
Press Ctrl+C to unmount.
```

## How it works

1. Generates an ephemeral Ed25519 SSH keypair (in memory only — the private key file is written at mode `0600` and removed on exit).
2. Creates a temporary pod in your cluster with the PVC mounted at `/data` and a pre-built sshd image configured with that key.
3. Port-forwards the pod's SSH port to a random local port via the Kubernetes API (no `kubectl` binary required).
4. Mounts `/data` over SSHFS at a local directory.
5. On exit (Ctrl+C), tears everything down: SSHFS unmount, port-forward, and the temporary pod.

If the PVC is `ReadWriteOnce` or `ReadWriteOncePod` and currently in use, the temporary pod is pinned to the same node so the volume attachment is not disrupted.

## Requirements

- Linux with [`sshfs`](https://github.com/libfuse/sshfs) installed (`apt install sshfs` / `dnf install fuse-sshfs`)
- A kubeconfig file (`~/.kube/config` or `$KUBECONFIG`) — or run inside the cluster

## Installation

Download a pre-built binary from the [releases page](https://github.com/yeniklas/pvcmount/releases):

```sh
curl -L https://github.com/yeniklas/pvcmount/releases/latest/download/pvcmount-latest-linux-amd64 -o pvcmount
chmod +x pvcmount
sudo mv pvcmount /usr/local/bin/
```

Or install with Go:

```sh
go install github.com/yeniklas/pvcmount@latest
```

Once installed, keep it up to date with:

```sh
pvcmount --self-update
```

## Usage

```sh
# Mount a PVC — creates ~/pvcmount/<pvc-name>-<random>/ automatically
pvcmount my-pvc

# Specify a namespace
pvcmount -n staging my-pvc

# Use a specific local directory instead of the auto-generated one
pvcmount --mountpoint /mnt/data my-pvc
```

Browse the files normally. Press Ctrl+C when done — pvcmount unmounts and deletes the temporary pod.

The auto-generated mountpoint (`~/pvcmount/<pvc-name>-<random>/`) is removed when pvcmount exits. A manually specified `--mountpoint` is left in place.

## Flags

| Flag | Default | Description |
|---|---|---|
| `-n`, `--namespace` | current context | Kubernetes namespace |
| `--kubeconfig` | `~/.kube/config` or `$KUBECONFIG` | Path to kubeconfig file |
| `--mountpoint` | `~/pvcmount/<pvc>-<random>` | Local directory to mount into |
| `--sshd-image` | `ghcr.io/yeniklas/pvcmount-sshd:latest` | Container image for the temporary pod |
| `--debug` | false | Show verbose output for troubleshooting |
| `--allow-root` | false | Allow running as root (not recommended) |
| `--version` | | Print version and exit |
| `--self-update` | | Update to the latest release |

## Zsh completion

```sh
pvcmount completion zsh > "${fpath[1]}/_pvcmount"
```

Completion covers flags and live PVC names from the cluster (namespace-aware via `--namespace`).

## Security

- pvcmount refuses to run as root. FUSE filesystems mounted by root are only accessible to root, and elevated privileges are not needed. Use `--allow-root` to override if you have a specific reason.
- The SSH keypair is freshly generated per invocation and passed to the pod as an environment variable — no Kubernetes Secrets or ConfigMaps are created.
- The temporary pod runs as root (required for unrestricted access to files on the PVC regardless of ownership). It uses `allowPrivilegeEscalation: false` and `seccompProfile: RuntimeDefault`.
- Clusters with a `restricted` PodSecurity policy will log one warning (`runAsNonRoot != true`) which is non-fatal. If your cluster enforces `restricted` strictly, the pod will be blocked.
- The pod name is deterministic (`pvcmount-<8 hex chars of sha256(pvc-name)>`). If pvcmount crashes, the stale pod is automatically detected and replaced on the next run, or can be cleaned up with `kubectl delete pod <name>`.

## Troubleshooting

Run with `--debug` to see the pod name, port-forward address, node assignment, and raw sshfs/sshd output:

```sh
pvcmount --debug my-pvc
```

If your shell freezes after a failed mount, a stale FUSE mount may have been left behind. Unmount it from another terminal:

```sh
fusermount3 -u ~/pvcmount/<name>
```
