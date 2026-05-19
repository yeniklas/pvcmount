package main

import (
	"context"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const zshCompletion = `#compdef pvcmount

_pvcmount() {
  # Pick up --namespace/-n and --kubeconfig from already-typed words so lists are context-aware.
  local namespace="" kubeconfig=""
  local i
  for (( i = 2; i <= ${#words}; i++ )); do
    if [[ "${words[i]}" == (-n|--namespace) ]]; then
      namespace="${words[i+1]}"
    elif [[ "${words[i]}" == --kubeconfig ]]; then
      kubeconfig="${words[i+1]}"
    fi
  done

  local -a ns_arg kc_arg
  [[ -n "$namespace" ]] && ns_arg=(--namespace "$namespace")
  [[ -n "$kubeconfig" ]] && kc_arg=(--kubeconfig "$kubeconfig")

  _arguments \
    '(-n --namespace)'{-n,--namespace}'[Kubernetes namespace (default: current context)]:namespace:($(pvcmount __list-namespaces "${kc_arg[@]}" 2>/dev/null))' \
    '--kubeconfig[path to kubeconfig file]:file:_files' \
    '--mountpoint[local mount directory (default: ~/pvcmount/<pvc>-<random>)]:directory:_directories' \
    '--sshd-image[container image for the temporary sshd pod]:image:' \
    '--version[print version and exit]' \
    '--self-update[update pvcmount to the latest release]' \
    ':PVC name:($(pvcmount __list-pvcs "${ns_arg[@]}" "${kc_arg[@]}" 2>/dev/null))'
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
	var ns, kc string
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--namespace" || args[i] == "-n" {
			ns = args[i+1]
		} else if args[i] == "--kubeconfig" {
			kc = args[i+1]
		}
	}
	client, err := NewClient(ns, kc)
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

// runListNamespaces is the backend for zsh namespace completion.
// It tries the cluster API first; if RBAC or connectivity prevents that it falls
// back to namespace names found in kubeconfig contexts.
func runListNamespaces(args []string) {
	var kc string
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--kubeconfig" {
			kc = args[i+1]
		}
	}
	client, err := NewClient("", kc)
	if err == nil {
		nsList, err := client.cs.CoreV1().Namespaces().List(context.Background(), metav1.ListOptions{})
		if err == nil {
			for _, ns := range nsList.Items {
				fmt.Println(ns.Name)
			}
			return
		}
	}
	for _, ns := range kubeConfigNamespaces(kc) {
		fmt.Println(ns)
	}
}
