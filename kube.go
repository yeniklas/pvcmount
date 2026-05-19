package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

type Client struct {
	cs        *kubernetes.Clientset
	restCfg   *rest.Config
	namespace string
}

func NewClient(namespace, kubeconfig string) (*Client, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})

	restCfg, err := clientConfig.ClientConfig()
	if err != nil {
		restCfg, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("cannot load kubeconfig or in-cluster config: %w", err)
		}
	}

	if namespace == "" {
		namespace, _, err = clientConfig.Namespace()
		if err != nil || namespace == "" {
			namespace = "default"
		}
	}

	restCfg.WarningHandler = rest.NoWarnings{}

	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, err
	}

	return &Client{cs: cs, restCfg: restCfg, namespace: namespace}, nil
}

type PVCInfo struct {
	Name        string
	AccessModes []corev1.PersistentVolumeAccessMode
	NodeName    string
}

func (c *Client) InspectPVC(ctx context.Context, pvcName string) (*PVCInfo, error) {
	pvc, err := c.cs.CoreV1().PersistentVolumeClaims(c.namespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get pvc %q: %w", pvcName, err)
	}

	info := &PVCInfo{Name: pvcName, AccessModes: pvc.Spec.AccessModes}

	isRWO := false
	for _, m := range pvc.Spec.AccessModes {
		if m == corev1.ReadWriteOnce || m == corev1.ReadWriteOncePod {
			isRWO = true
			break
		}
	}

	if isRWO {
		pods, err := c.cs.CoreV1().Pods(c.namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			return nil, fmt.Errorf("list pods: %w", err)
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}
			for _, vol := range pod.Spec.Volumes {
				if vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.ClaimName == pvcName {
					info.NodeName = pod.Spec.NodeName
					break
				}
			}
			if info.NodeName != "" {
				break
			}
		}
	}

	return info, nil
}

func PodName(pvcName string) string {
	h := sha256.Sum256([]byte(pvcName))
	return fmt.Sprintf("pvcmount-%x", h[:4])
}

const bootstrapScript = `mkdir -p /root/.ssh && \
chmod 700 /root/.ssh && \
printf '%s\n' "$AUTHORIZED_KEY" > /root/.ssh/authorized_keys && \
chmod 600 /root/.ssh/authorized_keys && \
exec /usr/sbin/sshd -D -e -p 22 \
  -o PermitRootLogin=yes \
  -o UsePAM=no`

func (c *Client) EnsurePod(ctx context.Context, info *PVCInfo, pubKeyLine, sshdImage string) (string, error) {
	podName := PodName(info.Name)

	existing, err := c.cs.CoreV1().Pods(c.namespace).Get(ctx, podName, metav1.GetOptions{})
	if err == nil {
		if err := c.DeletePod(ctx, podName); err != nil {
			return "", fmt.Errorf("delete stale pod: %w", err)
		}
		// wait for deletion
		watcher, err := c.cs.CoreV1().Pods(c.namespace).Watch(ctx, metav1.ListOptions{
			FieldSelector:   "metadata.name=" + podName,
			ResourceVersion: existing.ResourceVersion,
		})
		if err == nil {
			for ev := range watcher.ResultChan() {
				if ev.Type == watch.Deleted {
					watcher.Stop()
					break
				}
			}
		}
	}

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: c.namespace,
			Labels:    map[string]string{"app": "pvcmount", "pvc": info.Name},
		},
		Spec: corev1.PodSpec{
			NodeName:      info.NodeName,
			RestartPolicy: corev1.RestartPolicyNever,
			SecurityContext: &corev1.PodSecurityContext{
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
			Containers: []corev1.Container{{
				Name:    "sshd",
				Image:   sshdImage,
				Command: []string{"/bin/sh", "-c"},
				Args:    []string{bootstrapScript},
				Env: []corev1.EnvVar{{
					Name:  "AUTHORIZED_KEY",
					Value: pubKeyLine,
				}},
				Ports: []corev1.ContainerPort{{ContainerPort: 22}},
				SecurityContext: &corev1.SecurityContext{
					AllowPrivilegeEscalation: boolPtr(false),
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "data", MountPath: "/data"},
				},
			}},
			Volumes: []corev1.Volume{
				{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: info.Name,
						},
					},
				},
			},
		},
	}

	if _, err := c.cs.CoreV1().Pods(c.namespace).Create(ctx, pod, metav1.CreateOptions{}); err != nil {
		return "", fmt.Errorf("create pod: %w", err)
	}

	debugf("pod %s", podName)
	return podName, nil
}

func (c *Client) WaitPodReady(ctx context.Context, podName string) error {
	watcher, err := c.cs.CoreV1().Pods(c.namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: "metadata.name=" + podName,
	})
	if err != nil {
		return fmt.Errorf("watch pod: %w", err)
	}
	defer watcher.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-watcher.ResultChan():
			if !ok {
				return fmt.Errorf("pod watch channel closed")
			}
			pod, ok := ev.Object.(*corev1.Pod)
			if !ok {
				continue
			}
			switch pod.Status.Phase {
			case corev1.PodFailed, corev1.PodSucceeded:
				return fmt.Errorf("pod entered terminal phase %s", pod.Status.Phase)
			case corev1.PodRunning:
				return nil
			}
		}
	}
}

func (c *Client) StartPortForward(ctx context.Context, podName string) (uint16, func(), error) {
	url := c.cs.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(c.namespace).
		Name(podName).
		SubResource("portforward").
		URL()

	transport, upgrader, err := spdy.RoundTripperFor(c.restCfg)
	if err != nil {
		return 0, nil, fmt.Errorf("spdy roundtripper: %w", err)
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, url)

	stopChan := make(chan struct{}, 1)
	readyChan := make(chan struct{})

	pf, err := portforward.New(dialer, []string{"0:22"}, stopChan, readyChan, io.Discard, io.Discard)
	if err != nil {
		return 0, nil, fmt.Errorf("portforward.New: %w", err)
	}

	errChan := make(chan error, 1)
	go func() { errChan <- pf.ForwardPorts() }()

	select {
	case <-ctx.Done():
		close(stopChan)
		return 0, nil, ctx.Err()
	case err := <-errChan:
		return 0, nil, fmt.Errorf("portforward failed: %w", err)
	case <-readyChan:
	}

	ports, err := pf.GetPorts()
	if err != nil || len(ports) == 0 {
		close(stopChan)
		return 0, nil, fmt.Errorf("get forwarded ports: %w", err)
	}

	localPort := ports[0].Local
	debugf("port-forward localhost:%d → pod/%s:22", localPort, podName)

	// Wait until sshd is actually accepting connections.
	// A plain TCP dial succeeds immediately because the port-forward proxy accepts the
	// connection before forwarding it. We must read the SSH banner to confirm sshd is up.
	deadline := time.Now().Add(60 * time.Second)
	addr := fmt.Sprintf("127.0.0.1:%d", localPort)
	if debug {
		fmt.Print(styleDim("  waiting for sshd"))
	}
	ready := false
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			if debug {
				fmt.Println()
			}
			close(stopChan)
			return 0, nil, ctx.Err()
		}
		if sshdReady(addr) {
			if debug {
				fmt.Println()
			}
			stepDone("sshd ready")
			ready = true
			break
		}
		if debug {
			fmt.Print(styleDim("."))
		}
		time.Sleep(2 * time.Second)
	}
	if !ready {
		logs, _ := c.podLogs(podName)
		if logs != "" {
			fmt.Printf("pod logs:\n%s\n", logs)
		}
		close(stopChan)
		return 0, nil, fmt.Errorf("timed out waiting for sshd in pod %s", podName)
	}

	stop := func() { close(stopChan) }
	return uint16(localPort), stop, nil
}

// sshdReady dials addr and checks for the SSH banner. The port-forward proxy accepts
// connections locally before forwarding, so a successful dial alone proves nothing.
func sshdReady(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	buf := make([]byte, 8)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, err := conn.Read(buf)
	return err == nil && n > 0
}

func (c *Client) podLogs(podName string) (string, error) {
	req := c.cs.CoreV1().Pods(c.namespace).GetLogs(podName, &corev1.PodLogOptions{TailLines: int64ptr(20)})
	rc, err := req.Stream(context.Background())
	if err != nil {
		return "", err
	}
	defer rc.Close()
	b, err := io.ReadAll(rc)
	return string(b), err
}

func int64ptr(i int64) *int64 { return &i }
func boolPtr(b bool) *bool   { return &b }

// kubeConfigNamespaces extracts namespace names from kubeconfig contexts.
// Used as a fallback when RBAC prevents listing namespaces from the cluster API.
func kubeConfigNamespaces(kc string) []string {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kc != "" {
		rules.ExplicitPath = kc
	}
	cfg, err := rules.Load()
	if err != nil {
		return []string{"default"}
	}
	seen := map[string]bool{}
	var out []string
	for _, ctx := range cfg.Contexts {
		if ctx.Namespace != "" && !seen[ctx.Namespace] {
			seen[ctx.Namespace] = true
			out = append(out, ctx.Namespace)
		}
	}
	if !seen["default"] {
		out = append(out, "default")
	}
	return out
}

func (c *Client) DeletePod(ctx context.Context, podName string) error {
	grace := int64(0)
	prop := metav1.DeletePropagationBackground
	return c.cs.CoreV1().Pods(c.namespace).Delete(ctx, podName, metav1.DeleteOptions{
		GracePeriodSeconds: &grace,
		PropagationPolicy:  &prop,
	})
}
