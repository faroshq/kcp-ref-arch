/*
Copyright 2026 The KCP Reference Architecture Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package virtualmachines

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	commonv1alpha1 "github.com/faroshq/kcp-ref-arch/project/platform/apis/common/v1alpha1"
	computev1alpha1 "github.com/faroshq/kcp-ref-arch/project/platform/apis/compute/v1alpha1"

	mcbuilder "sigs.k8s.io/multicluster-runtime/pkg/builder"
	mcmanager "sigs.k8s.io/multicluster-runtime/pkg/manager"
	mcreconcile "sigs.k8s.io/multicluster-runtime/pkg/reconcile"
)

// Reconciler reconciles VirtualMachine resources.
type Reconciler struct {
	mgr mcmanager.Manager
}

// SetupWithManager registers the VirtualMachine controller with the multicluster manager.
func SetupWithManager(mgr mcmanager.Manager) error {
	r := &Reconciler{mgr: mgr}

	klog.Info("Registering VirtualMachine controller")
	if err := mcbuilder.ControllerManagedBy(mgr).
		Named("virtualmachine").
		For(&computev1alpha1.VirtualMachine{}).
		Complete(r); err != nil {
		return fmt.Errorf("setting up VirtualMachine controller: %w", err)
	}

	return nil
}

// Reconcile handles VirtualMachine reconciliation.
// It simulates a KubeVirt backend by transitioning VMs through lifecycle phases.
func (r *Reconciler) Reconcile(ctx context.Context, req mcreconcile.Request) (ctrl.Result, error) {
	logger := klog.FromContext(ctx).WithValues("vm", req.NamespacedName, "cluster", req.ClusterName)
	logger.Info("Reconciling VirtualMachine")

	cl, err := r.mgr.GetCluster(ctx, req.ClusterName)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("getting cluster %s: %w", req.ClusterName, err)
	}
	c := cl.GetClient()

	var vm computev1alpha1.VirtualMachine
	if err := c.Get(ctx, req.NamespacedName, &vm); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if vm.DeletionTimestamp != nil {
		logger.Info("[kubevirt-mock] VM deleted, would destroy KubeVirt VMI",
			"cores", vm.Spec.Cores,
			"memory", vm.Spec.Memory,
			"image", vm.Spec.Disk.Image,
		)
		return ctrl.Result{}, nil
	}

	switch vm.Status.Phase {
	case "", computev1alpha1.VirtualMachinePending:
		return r.handlePending(ctx, c, &vm, logger)
	case computev1alpha1.VirtualMachineProvisioning:
		return r.handleProvisioning(ctx, c, &vm, logger)
	case computev1alpha1.VirtualMachineRunning:
		return r.handleRunning(ctx, c, &vm, logger)
	default:
		logger.Info("VM in terminal state", "phase", vm.Status.Phase)
		return ctrl.Result{}, nil
	}
}

// handlePending transitions a new VM from Pending to Provisioning.
func (r *Reconciler) handlePending(ctx context.Context, c client.Client, vm *computev1alpha1.VirtualMachine, logger klog.Logger) (ctrl.Result, error) {
	logger.Info("[kubevirt-mock] Creating KubeVirt VMI",
		"cores", vm.Spec.Cores,
		"memory", vm.Spec.Memory,
		"image", vm.Spec.Disk.Image,
		"diskSize", vm.Spec.Disk.Size,
	)
	if vm.Spec.GPU != nil && vm.Spec.GPU.Count > 0 {
		logger.Info("[kubevirt-mock] Attaching GPU",
			"count", vm.Spec.GPU.Count,
		)
	}
	if vm.Spec.SSH != nil && vm.Spec.SSH.PublicKey != "" {
		logger.Info("[kubevirt-mock] Injecting SSH public key via cloud-init")
	}

	vm.Status.Phase = computev1alpha1.VirtualMachineProvisioning
	vm.Status.Message = "KubeVirt VMI created, waiting for scheduling"
	setCondition(&vm.Status.Conditions, metav1.Condition{
		Type:               commonv1alpha1.ConditionProgessing,
		Status:             metav1.ConditionTrue,
		Reason:             "VMICreated",
		Message:            "KubeVirt VirtualMachineInstance created",
		LastTransitionTime: metav1.Now(),
	})

	if err := c.Status().Update(ctx, vm); err != nil {
		return ctrl.Result{}, err
	}

	// Requeue to simulate scheduling delay.
	return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
}

// handleProvisioning transitions a VM from Provisioning to Running.
func (r *Reconciler) handleProvisioning(ctx context.Context, c client.Client, vm *computev1alpha1.VirtualMachine, logger klog.Logger) (ctrl.Result, error) {
	logger.Info("[kubevirt-mock] VMI scheduled, node assigned, starting VM",
		"cores", vm.Spec.Cores,
		"memory", vm.Spec.Memory,
	)

	vm.Status.Phase = computev1alpha1.VirtualMachineRunning
	vm.Status.Message = "VirtualMachineInstance is running"
	vm.Status.InternalIP = "10.244.1.42" // mock IP

	setCondition(&vm.Status.Conditions, metav1.Condition{
		Type:               commonv1alpha1.ConditionAvailable,
		Status:             metav1.ConditionTrue,
		Reason:             "VMIRunning",
		Message:            "KubeVirt VirtualMachineInstance is running",
		LastTransitionTime: metav1.Now(),
	})
	setCondition(&vm.Status.Conditions, metav1.Condition{
		Type:               commonv1alpha1.ConditionProgessing,
		Status:             metav1.ConditionFalse,
		Reason:             "VMIRunning",
		Message:            "Provisioning complete",
		LastTransitionTime: metav1.Now(),
	})

	if err := c.Status().Update(ctx, vm); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("[kubevirt-mock] VM is now running", "internalIP", vm.Status.InternalIP)
	return ctrl.Result{}, nil
}

// handleRunning is a no-op for already running VMs.
func (r *Reconciler) handleRunning(_ context.Context, _ client.Client, vm *computev1alpha1.VirtualMachine, logger klog.Logger) (ctrl.Result, error) {
	logger.V(4).Info("[kubevirt-mock] VM already running, nothing to do",
		"internalIP", vm.Status.InternalIP,
	)
	return ctrl.Result{}, nil
}

// setCondition sets a condition on the list, returning true if the condition was changed.
func setCondition(conditions *[]metav1.Condition, condition metav1.Condition) bool {
	for i, existing := range *conditions {
		if existing.Type == condition.Type {
			if existing.Status == condition.Status && existing.Reason == condition.Reason {
				return false
			}
			(*conditions)[i] = condition
			return true
		}
	}
	*conditions = append(*conditions, condition)
	return true
}
