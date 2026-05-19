# pvcmount

Mount a Kubernetes PersistentVolumeClaim as a local filesystem on Linux.

```
pvcmount [--namespace <ns>] <pvc-name> <mountpoint>
```

## How it works

1. Generates an ephemeral Ed25519 SSH keypair (in memory, never written to disk unencrypted).
2. Spins up a temporary Alpine pod in your cluster with the PVC mounted at `/data` and an SSH server configured with that key.
3. Port-forwards the pod's SSH port to a random local port via the Kubernetes API.
4. Mounts the pod's `/data` over SSHFS at your chosen local mountpoint.
5. On exit (Ctrl-C or unmount), tears everything down: SSHFS, port-forward, and the temporary pod.

The private key file on disk uses mode `0600` and is removed on exit.

## Requirements

- Linux host with [`sshfs`](https://github.com/libfuse/sshfs) installed
- `kubectl` access to a Kubernetes cluster (kubeconfig at `~/.kube/config` or `KUBECONFIG`)
- The cluster must be able to pull `alpine:3.20`

## Installation

```sh
go install github.com/yeniklas/pvcmount@latest
```

Or build from source:

```sh
git clone https://github.com/yeniklas/pvcmount
cd pvcmount
go build -o pvcmount .
```

## Usage

```sh
# Mount a PVC from the current kubeconfig namespace
mkdir -p /tmp/mnt
pvcmount my-pvc /tmp/mnt

# Specify a namespace explicitly
pvcmount --namespace staging my-pvc /tmp/mnt
```

Browse the files normally. Press Ctrl-C when done — pvcmount will unmount and delete the temporary pod.

## ReadWriteOnce PVCs

If the PVC is `ReadWriteOnce` and already in use by a running pod, pvcmount schedules its temporary pod on the same node so the volume attachment isn't disrupted.

## Security considerations

- The SSH keypair is freshly generated for each invocation. The public key is passed to the pod as an environment variable; no secrets are stored in ConfigMaps or Secrets.
- The temporary pod runs as root inside Alpine. It is deleted immediately on exit. If pvcmount is killed unexpectedly, rerun it — it will delete and recreate the stale pod before mounting.
- The pod name is deterministic (`pvcmount-<8 hex chars of sha256(pvcName)>`) so stale pods from crashes are easy to identify and clean up manually if needed.

## Limitations

- Requires `sshfs` on the local machine (FUSE-based, so FUSE must be available).
- The temporary pod pulls `alpine:3.20` and installs `openssh-server` via `apk` on startup. Cold starts take 15–30 seconds depending on cluster image cache and network.
- Clusters with a `restricted` PodSecurity admission policy will emit warnings (the Alpine pod runs as root). The warnings are non-fatal; if your cluster enforces `restricted` strictly, the pod will be blocked.
