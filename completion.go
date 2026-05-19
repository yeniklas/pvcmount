package main

import (
	"context"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const zshCompletion = `#compdef pvcmount

_pvcmount() {
  # Pick up --namespace value from already-typed words so PVC list is namespace-aware.
  local namespace=""
  local i
  for (( i = 2; i <= ${#words}; i++ )); do
    if [[ "${words[i]}" == (--namespace|-namespace) ]]; then
      namespace="${words[i+1]}"
      break
    fi
  done

  local -a ns_arg
  [[ -n "$namespace" ]] && ns_arg=(--namespace "$namespace")

  _arguments \
    '--namespace[Kubernetes namespace (default: current context)]:namespace:' \
    '--mountpoint[local mount directory (default: ~/pvcmount/<pvc>-<random>)]:directory:_directories' \
    '--sshd-image[container image for the temporary sshd pod]:image:' \
    '--version[print version and exit]' \
    '--self-update[update pvcmount to the latest release]' \
    ':PVC name:($(pvcmount __list-pvcs "${ns_arg[@]}" 2>/dev/null))'
}

_pvcmount "$@"
`

func runCompletion(shell string) {
	switch shell {
	case "zsh":
		fmt.Print(zshCompletion)
	default:
		fmt.Fprintf(os.Stderr, "unsupported shell %q — supported: zsh\n", shell)
		os.Exit(1)
	}
}

// runListPVCs is the backend for zsh completion — called as `pvcmount __list-pvcs [--namespace <ns>]`.
// All errors are swallowed so completion stays silent on failures.
func runListPVCs(args []string) {
	var ns string
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--namespace" || args[i] == "-namespace" {
			ns = args[i+1]
			break
		}
	}
	client, err := NewClient(ns)
	if err != nil {
		return
	}
	pvcs, err := client.cs.CoreV1().PersistentVolumeClaims(client.namespace).List(
		context.Background(), metav1.ListOptions{},
	)
	if err != nil {
		return
	}
	for _, pvc := range pvcs.Items {
		fmt.Println(pvc.Name)
	}
}
