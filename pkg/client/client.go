/*
 * +-------------------------------------------------------------------+
 * | Copyright IBM Corp. 2025 All Rights Reserved                      |
 * | PID 5698-SPR                                                      |
 * +-------------------------------------------------------------------+
 */

package client

import (
	"context"
	"fmt"
	"os"

	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	spyrev1alpha1 "github.com/ibm-aiu/spyre-operator/api/v1alpha1"
)

const (
	NodeStateResName     = "spyrenodestates"
	ClusterPolicyResName = "spyreclusterpolicies"

	NodeNameEnvKey = "NODE_NAME"
)

// a struct containing k8s rest client and CRUD methods
type SpyreClient struct {
	k8sClient client.Client
	cfg       *rest.Config
}

// returns SpyreNodeState-aware kube client based on the given rest.Config data.
// If config is nil, in-cluster config (rest.InClusterConfig()) will be
// automatically used.
func NewClient(ctx context.Context, cfg *rest.Config) (*SpyreClient, error) {
	var err error
	logger := log.FromContext(ctx)
	if cfg == nil {
		if cfg, err = rest.InClusterConfig(); err != nil {
			logger.Error(err, "Failed to get rest.InClusterConfig()")
			return nil, fmt.Errorf("failed to get rest client in cluster config: %w", err)
		}
	}
	scheme := runtime.NewScheme()
	if err = spyrev1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add spyre scheme: %w", err)
	}
	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize client: %w", err)
	}
	return &SpyreClient{k8sClient: k8sClient, cfg: cfg}, nil
}

func (c *SpyreClient) Create(ctx context.Context, s *spyrev1alpha1.SpyreNodeState) (*spyrev1alpha1.SpyreNodeState, error) {
	err := c.k8sClient.Create(ctx, s)
	if err != nil {
		return nil, fmt.Errorf("failed to create spyre node state: %w", err)
	}
	return s, nil
}

func (c *SpyreClient) Get(ctx context.Context, nodeStateName string) (*spyrev1alpha1.SpyreNodeState, error) {
	result := &spyrev1alpha1.SpyreNodeState{}
	err := c.k8sClient.Get(ctx, client.ObjectKey{Name: nodeStateName}, result)
	if err != nil {
		return nil, fmt.Errorf("failed to get spyre node state: %w", err)
	}
	return result, nil
}

func (c *SpyreClient) List(ctx context.Context, opts client.ListOption) (*spyrev1alpha1.SpyreNodeStateList, error) {
	result := &spyrev1alpha1.SpyreNodeStateList{}
	err := c.k8sClient.List(ctx, result, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list SpyreNodeState: %w", err)
	}
	return result, nil
}

func (c *SpyreClient) Delete(ctx context.Context, nodeStateName string, opts client.DeleteOption) error {
	obj, err := c.Get(ctx, nodeStateName)
	if k8serr.IsNotFound(err) {
		return nil
	}
	if err == nil {
		if err = c.k8sClient.Delete(ctx, obj, opts); err != nil {
			return fmt.Errorf("failed to delete SpyreNodeState: %w", err)
		}
		return nil
	}
	return fmt.Errorf("failed to get for SpyreNodeState deletion: %w", err)
}

func (c *SpyreClient) DeleteAll(ctx context.Context) error {
	nsList, err := c.List(ctx, &client.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete all SpyreNodeState: %w", err)
	}
	for _, ns := range nsList.Items {
		if err := c.Delete(ctx, ns.Name, &client.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete spyre node state: %w", err)
		}
	}
	return nil
}

func (c *SpyreClient) Update(ctx context.Context,
	ns *spyrev1alpha1.SpyreNodeState, retryOnConflict bool) (*spyrev1alpha1.SpyreNodeState, error) {
	err := c.k8sClient.Update(ctx, ns, &client.UpdateOptions{})
	if retryOnConflict && k8serr.IsConflict(err) {
		err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			nodeState, err := c.Get(ctx, ns.Name)
			if err != nil {
				return fmt.Errorf("failed to get SpyreNodeState: %w, retry", err)
			}
			nodeState.Spec = ns.Spec
			if err = c.k8sClient.Update(ctx, nodeState, &client.UpdateOptions{}); err == nil {
				ns = nodeState
				return nil
			}
			return fmt.Errorf("failed to update SpyreNodeState: %w, retry", err)
		})
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update SpyreNodeState: %w", err)
	}
	return ns, nil
}

func (c *SpyreClient) UpdateStatus(ctx context.Context,
	ns *spyrev1alpha1.SpyreNodeState, retryOnConflict bool) (*spyrev1alpha1.SpyreNodeState, error) {
	err := c.k8sClient.Status().Update(ctx, ns, &client.SubResourceUpdateOptions{})
	if retryOnConflict && k8serr.IsConflict(err) {
		err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			nodeState, err := c.Get(ctx, ns.Name)
			if err != nil {
				return fmt.Errorf("failed to get SpyreNodeState: %w, retry", err)
			}
			nodeState.Status = ns.Status
			if err = c.k8sClient.Update(ctx, nodeState, &client.UpdateOptions{}); err == nil {
				ns = nodeState
				return nil
			}
			return fmt.Errorf("failed to update SpyreNodeState status: %w, retry", err)
		})
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update SpyreNodeState status: %w", err)
	}
	return ns, nil
}

func (c *SpyreClient) GetSpyreClusterPolicy(ctx context.Context, name string) (*spyrev1alpha1.SpyreClusterPolicy, error) {
	cp := &spyrev1alpha1.SpyreClusterPolicy{}
	err := c.k8sClient.Get(ctx, client.ObjectKey{Name: name}, cp)
	if err != nil {
		return nil, fmt.Errorf("failed to get spyre cluster policy: %w", err)
	}
	return cp, nil
}

func (c *SpyreClient) CreateSpyreClusterPolicy(ctx context.Context, p *spyrev1alpha1.SpyreClusterPolicy) (*spyrev1alpha1.SpyreClusterPolicy, error) { //nolint:lll
	if err := c.k8sClient.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("failed to create cluster policy: %w", err)
	}
	return p, nil
}

func (c *SpyreClient) DeleteSpyreClusterPolicy(ctx context.Context, name string, opts client.DeleteOption) error {
	obj, err := c.GetSpyreClusterPolicy(ctx, name)
	if k8serr.IsNotFound(err) {
		return nil
	}
	if err == nil {
		if err = c.k8sClient.Delete(ctx, obj, opts); err != nil {
			return fmt.Errorf("failed to delete spyre cluster policy: %w", err)
		}
		return nil
	}
	return fmt.Errorf("failed to get for SpyreClusterPolicy deletion: %w", err)
}

// UpdateSpyreClusterPolicyStatus updates status of SpyreClusterPolicy resource
func (c *SpyreClient) UpdateSpyreClusterPolicyStatus(ctx context.Context, p *spyrev1alpha1.SpyreClusterPolicy, retryOnConflict bool) (*spyrev1alpha1.SpyreClusterPolicy, error) { //nolint:lll
	err := c.k8sClient.Status().Update(ctx, p, &client.SubResourceUpdateOptions{})
	if retryOnConflict && k8serr.IsConflict(err) {
		err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			policy, err := c.GetSpyreClusterPolicy(ctx, p.Name)
			if err != nil {
				return fmt.Errorf("failed to get SpyreClusterPolicy: %w, retry", err)
			}
			policy.Status = p.Status
			if err = c.k8sClient.Status().Update(ctx, policy, &client.SubResourceUpdateOptions{}); err == nil {
				p = policy
				return nil
			}
			return fmt.Errorf("failed to update SpyreClusterPolicy status: %w, retry", err)
		})
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update SpyreClusterPolicy status: %w", err)
	}
	return p, nil
}

// GetSpyreNamespace returns the name of Spyre namespace by searching SpyreClusterPolicy resource.
func (c *SpyreClient) GetSpyreNamespace(ctx context.Context, name string) (string, error) {
	cp, err := c.GetSpyreClusterPolicy(ctx, name)
	if err != nil {
		return "", fmt.Errorf("failed to get spyre namespace from cluster policy: %w", err)
	}
	return cp.Namespace, nil
}

// GetSpyreNodeState returns a deepcopy of SpyreNodeState for the specified node.
// if the nodeName is empty, returns a SpyreNodeState of the node on which the device plugin runs.
func (c *SpyreClient) GetSpyreNodeState(ctx context.Context, nodeName string) (*spyrev1alpha1.SpyreNodeState, error) {
	if nodeName == "" {
		nodeName = os.Getenv(NodeNameEnvKey)
	}
	var nodeStateList *spyrev1alpha1.SpyreNodeStateList
	var err error
	if nodeStateList, err = c.List(ctx, &client.ListOptions{}); err != nil {
		klog.Errorf("Failed to get SpyreNodeState list: %v", err)
		return nil, fmt.Errorf("failed to get SpyreNodeState list: %w", err)
	}
	for _, nodeState := range nodeStateList.Items {
		if nodeState.Spec.NodeName == nodeName {
			return nodeState.DeepCopy(), nil
		}
	}
	return nil, fmt.Errorf("failed to get SpyreNodeState resource for node '%s': %w", nodeName, err)
}
