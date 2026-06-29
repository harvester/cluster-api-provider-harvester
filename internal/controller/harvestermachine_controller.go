/*
Copyright 2025 SUSE.

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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/pkg/errors"
	kubevirtv1 "kubevirt.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/paused"
	"sigs.k8s.io/cluster-api/util/predicates"

	infrav1 "github.com/harvester/cluster-api-provider-harvester/api/v1beta1"
	caphvmetrics "github.com/harvester/cluster-api-provider-harvester/internal/metrics"
	harvclient "github.com/harvester/cluster-api-provider-harvester/pkg/clientset/versioned"
	locutil "github.com/harvester/cluster-api-provider-harvester/util"
)

// HarvesterMachineReconciler reconciles a HarvesterMachine object.
type HarvesterMachineReconciler struct {
	client.Client

	Scheme *runtime.Scheme
}

// Scope stores context data for the reconciler.
type Scope struct {
	Ctx                    context.Context
	Cluster                *clusterv1.Cluster
	Machine                *clusterv1.Machine
	HarvesterCluster       *infrav1.HarvesterCluster
	HarvesterMachine       *infrav1.HarvesterMachine
	HarvesterClient        harvclient.Interface
	ReconcilerClient       client.Client
	Logger                 *logr.Logger
	EffectiveNetworkConfig *infrav1.NetworkConfig
}

const (
	vmAnnotationPVC        = "harvesterhci.io/volumeClaimTemplates"
	vmAnnotationNetworkIps = "networks.harvesterhci.io/ips"
	hvAnnotationDiskNames  = "harvesterhci.io/diskNames"
	hvAnnotationSSH        = "harvesterhci.io/sshNames"
	hvAnnotationImageID    = "harvesterhci.io/imageId"
	requeueDelay           = 10 * time.Second
)

var (
	machineInitializationProvisioned = infrav1.Initialization{
		Provisioned: true,
	}

	machineInitializationNotProvisioned = infrav1.Initialization{
		Provisioned: false,
	}
)

//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=harvestermachines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=harvestermachines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=harvestermachines/finalizers,verbs=update
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=harvesterclusters,verbs=get;list
//+kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=clusters;machines,verbs=get;list;watch

// Reconcile reconciles the HarvesterMachine object.
func (r *HarvesterMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, rerr error) {
	logger := log.FromContext(ctx)
	ctx = ctrl.LoggerInto(ctx, logger)

	logger.Info("Reconciling HarvesterMachine ...")

	hvMachine := &infrav1.HarvesterMachine{}

	err := r.Get(ctx, req.NamespacedName, hvMachine)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("harvestermachine not found")

			return ctrl.Result{}, nil
		}

		logger.Error(err, "Error happened when getting harvestermachine")

		return ctrl.Result{}, err
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(hvMachine, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always attempt to Patch the HarvesterMachine object and status after each reconciliation.
	defer func() {
		err := patchHelper.Patch(ctx,
			hvMachine,
		// conditions.WithOwnedConditions( []string{ clusterv1.ReadyCondition}),
		)
		if err != nil {
			logger.Error(err, "failed to patch HarvesterMachine")

			if rerr == nil {
				rerr = err
			}
		}
	}()

	ownerMachine, err := util.GetOwnerMachine(ctx, r.Client, hvMachine.ObjectMeta)
	if err != nil {
		logger.Error(err, "unable to get owner machine")

		return ctrl.Result{}, err
	}

	if ownerMachine == nil {
		logger.Info("Waiting for Machine Controller to set OwnerRef on HarvesterMachine")

		return ctrl.Result{RequeueAfter: requeueTimeShort}, nil
	}

	ownerCluster, err := util.GetClusterFromMetadata(ctx, r.Client, ownerMachine.ObjectMeta)
	if err != nil {
		logger.Info("HarvesterMachine owner Machine is missing cluster label or cluster does not exist")

		return ctrl.Result{}, err
	}

	if ownerCluster == nil {
		logger.Info("Please associate this machine with a cluster using the label " + clusterv1.ClusterNameLabel + ": <name of cluster>")

		return ctrl.Result{}, nil
	}

	logger = logger.WithValues("machine", ownerMachine.Namespace+"/"+ownerMachine.Name, "cluster", ownerCluster.Namespace+"/"+ownerCluster.Name)
	ctx = ctrl.LoggerInto(ctx, logger)

	// Publish the Paused condition (v1beta2 contract) and freeze reconciliation while
	// the Cluster or the HarvesterMachine is paused. Placed before any Harvester access
	// so a paused object never triggers infrastructure calls.
	isPaused, requeuePaused, err := paused.EnsurePausedCondition(ctx, r.Client, ownerCluster, hvMachine)
	if err != nil || isPaused || requeuePaused {
		return ctrl.Result{}, err
	}

	hvCluster := &infrav1.HarvesterCluster{}

	// CAPI v1beta2 ContractVersionedObjectReference dropped the Namespace field —
	// the infrastructure reference is always in the same namespace as the Cluster.
	hvClusterKey := types.NamespacedName{
		Namespace: ownerCluster.Namespace,
		Name:      ownerCluster.Spec.InfrastructureRef.Name,
	}

	err = r.Get(ctx, hvClusterKey, hvCluster)
	if err != nil {
		logger.Error(err, "unable to find corresponding harvestercluster to harvestermachine")

		return ctrl.Result{}, err
	}

	hvSecret, err := locutil.GetSecretForHarvesterConfig(ctx, hvCluster, r.Client)
	if err != nil {
		logger.Error(err, "unable to get Datasource secret")

		return ctrl.Result{}, err
	}

	hvClient, err := locutil.GetHarvesterClientFromSecret(hvSecret)
	if err != nil {
		logger.Error(err, "unable to create Harvester client from Datasource secret "+hvSecret.Name)
	}

	hvScope := Scope{
		Ctx:              ctx,
		Cluster:          ownerCluster,
		Machine:          ownerMachine,
		HarvesterCluster: hvCluster,
		HarvesterMachine: hvMachine,
		HarvesterClient:  hvClient,
		ReconcilerClient: r.Client,
		Logger:           &logger,
	}

	if !hvMachine.DeletionTimestamp.IsZero() {
		return r.ReconcileDelete(hvScope) //nolint:contextcheck
	}

	return r.ReconcileNormal(&hvScope) //nolint:contextcheck
}

// SetupWithManager sets up the controller with the Manager.
func (r *HarvesterMachineReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	clusterToHarvesterMachine, err := util.ClusterToTypedObjectsMapper(mgr.GetClient(), &infrav1.HarvesterMachineList{}, mgr.GetScheme())
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.HarvesterMachine{}).
		// No pause-filtering predicates: pause/unpause transitions must reach the
		// reconciler so the Paused condition (v1beta2 contract) is published.
		Watches(
			&clusterv1.Machine{},
			handler.EnqueueRequestsFromMapFunc(util.MachineToInfrastructureMapFunc(infrav1.GroupVersion.WithKind("HarvesterMachine"))),
		).
		Watches(
			&clusterv1.Cluster{},
			handler.EnqueueRequestsFromMapFunc(clusterToHarvesterMachine),
			builder.WithPredicates(predicates.ClusterPausedTransitions(mgr.GetScheme(), ctrl.LoggerFrom(ctx))),
		).
		Complete(r)
}

// ReconcileNormal reconciles the HarvesterMachine object.
//
//nolint:gocyclo // complexity increased by finalizer migration logic, will decrease once legacy support is removed
func (r *HarvesterMachineReconciler) ReconcileNormal(hvScope *Scope) (res reconcile.Result, rerr error) {
	reconcileStart := time.Now()

	defer func() {
		caphvmetrics.MachineReconcileDuration.WithLabelValues("normal").Observe(time.Since(reconcileStart).Seconds())

		clusterName := hvScope.Cluster.Namespace + "/" + hvScope.Cluster.Name
		machineName := hvScope.HarvesterMachine.Namespace + "/" + hvScope.HarvesterMachine.Name

		if hvScope.HarvesterMachine.Status.Ready {
			caphvmetrics.MachineStatus.WithLabelValues(clusterName, machineName).Set(1)
		} else {
			caphvmetrics.MachineStatus.WithLabelValues(clusterName, machineName).Set(0)
		}
	}()

	logger := log.FromContext(hvScope.Ctx)

	// Migrate legacy finalizer: if the object has the old finalizer but not the new one, swap them.
	if controllerutil.ContainsFinalizer(hvScope.HarvesterMachine, infrav1.MachineFinalizerLegacy) &&
		!controllerutil.ContainsFinalizer(hvScope.HarvesterMachine, infrav1.MachineFinalizer) &&
		hvScope.HarvesterMachine.DeletionTimestamp.IsZero() {
		controllerutil.AddFinalizer(hvScope.HarvesterMachine, infrav1.MachineFinalizer)
		controllerutil.RemoveFinalizer(hvScope.HarvesterMachine, infrav1.MachineFinalizerLegacy)

		return ctrl.Result{}, nil
	}

	// Add finalizer first if not exist to avoid the race condition between init and delete
	if !controllerutil.ContainsFinalizer(hvScope.HarvesterMachine, infrav1.MachineFinalizer) && hvScope.HarvesterMachine.DeletionTimestamp.IsZero() {
		controllerutil.AddFinalizer(hvScope.HarvesterMachine, infrav1.MachineFinalizer)

		hvScope.HarvesterMachine.Status.Ready = false
		hvScope.HarvesterMachine.Status.Initialization = machineInitializationNotProvisioned

		return ctrl.Result{}, nil
	}

	// Return early if the ownerCluster has not been provisioned by its infrastructure.
	// CAPI v1beta2 replaced Cluster.Status.InfrastructureReady by
	// Cluster.Status.Initialization.InfrastructureProvisioned (a *bool).
	infraProvisioned := hvScope.Cluster.Status.Initialization.InfrastructureProvisioned
	if infraProvisioned == nil || !*infraProvisioned {
		logger.Info("Waiting for Infrastructure to be ready ... ")

		hvScope.HarvesterMachine.Status.Ready = false
		hvScope.HarvesterMachine.Status.Initialization = machineInitializationNotProvisioned

		conditions.Set(hvScope.HarvesterMachine, metav1.Condition{
			Type:    infrav1.InfrastructureReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.InfrastructureProvisioningInProgressReason,
			Message: "Waiting for cluster infrastructure to be ready",
		})

		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	// Set InfrastructureReady condition when cluster infrastructure is ready
	conditions.Set(hvScope.HarvesterMachine, metav1.Condition{
		Type:    infrav1.InfrastructureReadyCondition,
		Status:  metav1.ConditionTrue,
		Reason:  infrav1.InfrastructureReadyReason,
		Message: "Cluster infrastructure is ready",
	})

	// Return early if no userdata secret is referenced in ownerMachine
	if hvScope.Machine.Spec.Bootstrap.DataSecretName == nil {
		logger.Info("Waiting for Machine's Userdata to be set ... ")

		hvScope.HarvesterMachine.Status.Ready = false
		hvScope.HarvesterMachine.Status.Initialization = machineInitializationNotProvisioned

		return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
	}

	// Report the failure domain the machine lands in (CAPI contract field,
	// mirrored to Machine.status.failureDomain by the core controller)
	hvScope.HarvesterMachine.Status.FailureDomain = effectiveFailureDomain(hvScope)

	// Resolve effective network config: pool allocation (machine-level
	// vmNetworkConfig taking precedence over the cluster-level one) or
	// machine-level static config
	if effectiveVMNetworkConfig(hvScope) != nil && hvScope.HarvesterMachine.Spec.NetworkConfig == nil {
		// Allocate IP from cluster-level VM IP pool
		allocErr := r.allocateVMIP(hvScope)
		if allocErr != nil {
			logger.Error(allocErr, "failed to allocate VM IP from pool")

			conditions.Set(hvScope.HarvesterMachine, metav1.Condition{
				Type:    infrav1.VMIPAllocatedCondition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1.VMIPAllocationFailedReason,
				Message: fmt.Sprintf("Failed to allocate VM IP: %v", allocErr),
			})

			return ctrl.Result{RequeueAfter: requeueTimeShort}, allocErr
		}

		vmNetCfg := effectiveVMNetworkConfig(hvScope)
		hvScope.EffectiveNetworkConfig = &infrav1.NetworkConfig{
			Address:    hvScope.HarvesterMachine.Status.AllocatedIPAddress,
			Gateway:    vmNetCfg.Gateway,
			DNSServers: vmNetCfg.DNSServers,
			DNSSearch:  vmNetCfg.DNSSearch,
		}

		conditions.Set(hvScope.HarvesterMachine, metav1.Condition{
			Type:    infrav1.VMIPAllocatedCondition,
			Status:  metav1.ConditionTrue,
			Reason:  infrav1.VMIPAllocatedReason,
			Message: fmt.Sprintf("Allocated IP %s from pool", hvScope.HarvesterMachine.Status.AllocatedIPAddress),
		})
	} else if hvScope.HarvesterMachine.Spec.NetworkConfig != nil {
		// Use machine-level static network config
		hvScope.EffectiveNetworkConfig = hvScope.HarvesterMachine.Spec.NetworkConfig
	}

	vmExists := false

	// check if Harvester has a machine with the same name and namespace
	existingVM, err := hvScope.HarvesterClient.KubevirtV1().VirtualMachines(hvScope.HarvesterCluster.Spec.TargetNamespace).Get(
		hvScope.Ctx, hvScope.HarvesterMachine.Name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		logger.Error(err, "unable to check existence of VM from Harvester")

		hvScope.HarvesterMachine.Status.Ready = false
		hvScope.HarvesterMachine.Status.Initialization = machineInitializationNotProvisioned

		return ctrl.Result{}, err
	}

	if (existingVM != nil) && (existingVM.Name == hvScope.HarvesterMachine.Name) {
		vmExists = true

		// Set VMProvisioningReady condition for existing VM
		conditions.Set(hvScope.HarvesterMachine, metav1.Condition{
			Type:    infrav1.VMProvisioningReadyCondition,
			Status:  metav1.ConditionTrue,
			Reason:  infrav1.VMProvisioningReadyReason,
			Message: "VM already exists and is provisioned",
		})

		if isVMRunning(existingVM) {
			ipAddresses, err := getIPAddressesFromVMI(hvScope.Ctx, existingVM, hvScope.HarvesterClient)
			if err != nil {
				hvScope.HarvesterMachine.Status.Ready = false
				hvScope.HarvesterMachine.Status.Initialization = machineInitializationNotProvisioned

				if apierrors.IsNotFound(err) {
					logger.Info("VM is not running yet, waiting for it to be ready")

					return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
				}

				logger.V(1).Info("unable to get IP addresses from VMI in Harvester, requeuing ...")

				return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
			}

			hvScope.HarvesterMachine.Status.Addresses = ipAddresses

			if len(ipAddresses) == 0 && hvScope.HarvesterMachine.Status.Ready {
				hvScope.HarvesterMachine.Status.Ready = false
				hvScope.HarvesterMachine.Status.Initialization = machineInitializationNotProvisioned

				return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
			}

			if len(ipAddresses) > 0 && !hvScope.HarvesterMachine.Status.Ready {
				logger.Info("VM is running, IP addresses found", "addresses", ipAddresses)

				hvScope.HarvesterMachine.Status.Ready = true
				hvScope.HarvesterMachine.Status.Initialization = machineInitializationProvisioned
				conditions.Set(hvScope.HarvesterMachine, metav1.Condition{
					Type:   infrav1.VMProvisioningReadyCondition,
					Status: metav1.ConditionTrue,
					Reason: infrav1.VMProvisioningReadyReason,
				})

				return ctrl.Result{Requeue: true}, nil
			}

			if hvScope.HarvesterMachine.Spec.ProviderID == "" {
				providerID, err := getProviderIDFromWorkloadCluster(hvScope)
				if err != nil {
					providerID = "harvester://" + string(existingVM.UID)
				}

				if providerID != "" {
					hvScope.HarvesterMachine.Spec.ProviderID = providerID
					hvScope.HarvesterMachine.Status.Ready = true
					hvScope.HarvesterMachine.Status.Initialization = machineInitializationProvisioned
				} else {
					logger.Info("Waiting for ProviderID to be set on Node resource in Workload Cluster ...")

					return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
				}
			} else {
				conditions.Set(hvScope.HarvesterMachine, metav1.Condition{
					Type:   infrav1.MachineCreatedCondition,
					Status: metav1.ConditionTrue,
					Reason: infrav1.MachineCreatedCondition,
				})
			}
		} else {
			return ctrl.Result{RequeueAfter: requeueTimeShort}, nil
		}
	}

	if !conditions.IsTrue(hvScope.HarvesterMachine, infrav1.MachineCreatedCondition) {
		logger.Info("No existing VM found in Harvester, creating a new one ...")

		hvScope.HarvesterMachine.Status.Ready = false

		// Set VMProvisioningReady condition to in progress
		conditions.Set(hvScope.HarvesterMachine, metav1.Condition{
			Type:    infrav1.VMProvisioningReadyCondition,
			Status:  metav1.ConditionFalse,
			Reason:  infrav1.VMProvisioningInProgressReason,
			Message: "VM provisioning in progress",
		})

		caphvmetrics.MachineCreateTotal.Inc()

		createStart := time.Now()

		_, err = createVMFromHarvesterMachine(hvScope)
		if err != nil {
			caphvmetrics.MachineCreateErrorsTotal.Inc()
			logger.Error(err, "unable to create VM from HarvesterMachine information")

			conditions.Set(hvScope.HarvesterMachine, metav1.Condition{
				Type:    infrav1.VMProvisioningReadyCondition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1.VMProvisioningFailedReason,
				Message: fmt.Sprintf("Failed to create VM: %v", err),
			})

			return ctrl.Result{}, err
		}

		caphvmetrics.MachineCreationDuration.Observe(time.Since(createStart).Seconds())

		conditions.Set(hvScope.HarvesterMachine, metav1.Condition{
			Type:   infrav1.MachineCreatedCondition,
			Status: metav1.ConditionTrue,
			Reason: infrav1.MachineCreatedCondition,
		})
		hvScope.HarvesterMachine.Status.Ready = false

		// Patch the HarvesterCluster resource with the InitMachineCreatedCondition if it is not already set.
		if !conditions.IsTrue(hvScope.HarvesterCluster, infrav1.InitMachineCreatedCondition) {
			hvClusterCopy := hvScope.HarvesterCluster.DeepCopy()
			conditions.Set(hvClusterCopy, metav1.Condition{
				Type:   infrav1.InitMachineCreatedCondition,
				Status: metav1.ConditionTrue,
				Reason: infrav1.InitMachineCreatedCondition,
			})
			hvClusterCopy.Status.Ready = hvScope.HarvesterCluster.Status.Ready

			err := r.Client.Status().Patch(hvScope.Ctx, hvClusterCopy, client.MergeFrom(hvScope.HarvesterCluster))
			if err != nil {
				logger.Error(err, "failed to update HarvesterCluster Conditions with InitMachineCreatedCondition")
			}
		}
	} else {
		if !vmExists {
			hvScope.HarvesterMachine.Status.Ready = false
			conditions.Set(hvScope.HarvesterMachine, metav1.Condition{
				Type:    infrav1.MachineCreatedCondition,
				Status:  metav1.ConditionFalse,
				Reason:  infrav1.MachineNotFoundReason,
				Message: "VM not found in Harvester",
			})

			return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
		}
	}

	hvScope.HarvesterMachine.Status.Ready = true

	// Initialize workload cluster node: set providerID and remove uninitialized taint.
	// This bypasses the cloud-provider bootstrap chicken-and-egg problem.
	if !r.initializeWorkloadNode(hvScope) {
		// The node has not registered (or could not be patched) yet. Requeue
		// explicitly: once the machine events settle, nothing else triggers a
		// reconcile, and without the providerID on the node the machine would
		// stay in the Provisioned phase forever.
		return ctrl.Result{RequeueAfter: requeueDelay}, nil
	}

	return ctrl.Result{}, nil
}

// initializeWorkloadNode sets the providerID and removes the cloud-provider
// uninitialized taint on the workload cluster node corresponding to this machine.
// It reports whether the node is initialized; errors are logged as warnings and
// surface as a false return so the caller requeues.
//
//nolint:funcorder
func (r *HarvesterMachineReconciler) initializeWorkloadNode(hvScope *Scope) bool {
	if hvScope.HarvesterMachine.Spec.ProviderID == "" {
		return false
	}

	workloadConfig, err := getWorkloadClusterConfig(hvScope)
	if err != nil {
		// Workload cluster not ready yet, will retry on next reconcile
		return false
	}

	caphvmetrics.NodeInitTotal.Inc()

	initStart := time.Now()

	initialized := locutil.InitializeWorkloadNode(
		hvScope.Ctx,
		*hvScope.Logger,
		workloadConfig,
		hvScope.HarvesterMachine.Name,
		hvScope.HarvesterMachine.Spec.ProviderID,
	)

	caphvmetrics.NodeInitDuration.Observe(time.Since(initStart).Seconds())

	return initialized
}

func getProviderIDFromWorkloadCluster(hvScope *Scope) (string, error) {
	var workloadConfig *rest.Config

	workloadConfig, err := getWorkloadClusterConfig(hvScope)
	if err != nil {
		return "", errors.Wrap(err, "unable to get workload cluster config from Management Cluster")
	}

	// Get Kubernetes client for workload cluster.
	workloadClient, err := client.New(workloadConfig, client.Options{})
	if err != nil {
		return "", errors.Wrap(err, "unable to get workload cluster client from Kubeconfig")
	}

	// Get ProviderID from the Node object in the workload cluster
	node := &v1.Node{}

	err = workloadClient.Get(hvScope.Ctx, types.NamespacedName{Name: hvScope.HarvesterMachine.Name}, node)
	if err != nil {
		return "", err
	}

	return node.Spec.ProviderID, nil
}

// getWorkloadClusterConfig returns a rest.Config for the workload cluster from a secret in the management cluster.
func getWorkloadClusterConfig(hvScope *Scope) (*rest.Config, error) {
	// Get the workload cluster kubeconfig secret
	workloadClusterKubeconfigSecret := &v1.Secret{}

	err := hvScope.ReconcilerClient.Get(hvScope.Ctx, types.NamespacedName{
		Namespace: hvScope.Cluster.Namespace,
		Name:      hvScope.Cluster.Name + "-kubeconfig",
	}, workloadClusterKubeconfigSecret)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get workload cluster kubeconfig secret")
	}

	// Get the kubeconfig data from the secret
	kubeconfigData, ok := workloadClusterKubeconfigSecret.Data["value"]
	if !ok {
		return nil, fmt.Errorf("no kubeconfig data found in secret %s", workloadClusterKubeconfigSecret.Name)
	}

	// Create a rest.Config from the kubeconfig data
	workloadConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigData)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get workload cluster config from kubeconfig data")
	}

	return workloadConfig, nil
}

// isVMRunning checks whether a VM is intended to be running.
// KubeVirt VMs can use either spec.running (bool pointer) or spec.runStrategy.
// Harvester uses runStrategy instead of running, which makes spec.running nil.
// We use KubeVirt's built-in RunStrategy() method that handles both cases.
func isVMRunning(vm *kubevirtv1.VirtualMachine) bool {
	strategy, err := vm.RunStrategy()
	if err != nil {
		// Both running and runStrategy set (mutually exclusive) — fallback to status
		return vm.Status.Ready
	}

	return strategy == kubevirtv1.RunStrategyAlways ||
		strategy == kubevirtv1.RunStrategyRerunOnFailure ||
		strategy == kubevirtv1.RunStrategyOnce
}

func getIPAddressesFromVMI(
	ctx context.Context, existingVM *kubevirtv1.VirtualMachine, hvClient harvclient.Interface,
) ([]clusterv1.MachineAddress, error) {
	ipAddresses := []clusterv1.MachineAddress{}

	vmInstance, err := hvClient.KubevirtV1().VirtualMachineInstances(existingVM.Namespace).Get(ctx, existingVM.Name, metav1.GetOptions{})
	if err != nil {
		// if apierrors.IsNotFound(err) {
		// 	return ipAddresses, fmt.Errorf("no VM instance found for VM %s", existingVM.Name)
		// }
		return ipAddresses, err
	}

	for _, nic := range vmInstance.Status.Interfaces {
		if nic.IP == "" {
			continue
		}

		ipAddresses = append(ipAddresses, clusterv1.MachineAddress{
			Type:    clusterv1.MachineExternalIP,
			Address: nic.IP,
		})
	}

	return ipAddresses, nil
}

// diskInfo holds the PVC name and volume index for each disk attached to a VM.
type diskInfo struct {
	pvcName string
	index   int
}

func createVMFromHarvesterMachine(hvScope *Scope) (*kubevirtv1.VirtualMachine, error) {
	var err error

	vmLabels := map[string]string{
		"harvesterhci.io/creator": "docker-machine-driver-harvester",
	}

	if _, ok := hvScope.HarvesterMachine.Labels[clusterv1.MachineControlPlaneLabel]; ok {
		vmLabels[cpVMLabelKey] = cpVMLabelValuePrefix + "-" + hvScope.Cluster.Name
	}

	vmiLabels := vmLabels

	vmName := hvScope.HarvesterMachine.Name

	vmiLabels["harvesterhci.io/vmName"] = vmName
	vmiLabels["harvesterhci.io/vmNamePrefix"] = vmName

	targetNS := hvScope.HarvesterCluster.Spec.TargetNamespace

	// Build PVC list for all volumes
	pvcs := make([]*v1.PersistentVolumeClaim, 0, len(hvScope.HarvesterMachine.Spec.Volumes))
	disks := make([]diskInfo, 0, len(hvScope.HarvesterMachine.Spec.Volumes))

	for i, vol := range hvScope.HarvesterMachine.Spec.Volumes {
		diskRandomID := locutil.RandomID()
		pvcName := fmt.Sprintf("%s-disk-%d-%s", vmName, i, diskRandomID)

		pvc, err := buildPVCForVolume(&vol, pvcName, targetNS, hvScope)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to build PVC for volume %d", i)
		}

		pvcs = append(pvcs, pvc)
		disks = append(disks, diskInfo{pvcName: pvcName, index: i})
	}

	pvcAnnotation, err := json.Marshal(pvcs)
	if err != nil {
		return nil, errors.Wrap(err, "unable to marshal PVC annotations")
	}

	vmTemplate, err := buildVMTemplate(hvScope, disks, vmiLabels)
	if err != nil {
		return &kubevirtv1.VirtualMachine{}, errors.Wrap(err, "unable to build VM definition")
	}

	if vmTemplate.ObjectMeta.Labels == nil {
		vmTemplate.ObjectMeta.Labels = make(map[string]string)
	}

	ubuntuVM := &kubevirtv1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmName,
			Namespace: targetNS,
			Annotations: map[string]string{
				vmAnnotationPVC:        string(pvcAnnotation),
				vmAnnotationNetworkIps: "[]",
			},
			Labels: vmLabels,
		},
		Spec: kubevirtv1.VirtualMachineSpec{
			RunStrategy: new(kubevirtv1.RunStrategyAlways),
			Template:    vmTemplate,
		},
	}

	hvCreatedMachine, err := hvScope.HarvesterClient.KubevirtV1().VirtualMachines(targetNS).Create(
		hvScope.Ctx,
		ubuntuVM,
		metav1.CreateOptions{})
	if err != nil {
		return hvCreatedMachine, err
	}

	return hvCreatedMachine, nil
}

// storageClassForImage returns the StorageClass backing a Harvester VM image.
// Harvester publishes it in the image status; up to Harvester 1.8.0 it happened to
// be "longhorn-<image>", but 1.8.1 names it "lh-<uuid>", so the status is the only
// reliable source. The old convention is kept as a fallback for images whose status
// is not populated yet.
func storageClassForImage(vmImage *harvesterv1beta1.VirtualMachineImage) string {
	if vmImage.Status.StorageClassName != "" {
		return vmImage.Status.StorageClassName
	}

	return "longhorn-" + vmImage.Name
}

// buildPVCForVolume creates a PersistentVolumeClaim for a single volume.
// For "image" volumes, the PVC references a Harvester VM image (StorageClass resolved
// from the image status).
// For "storageClass" volumes, the PVC uses the specified StorageClass directly (blank data disk).
func buildPVCForVolume(
	vol *infrav1.Volume,
	pvcName string,
	pvcNamespace string,
	hvScope *Scope,
) (*v1.PersistentVolumeClaim, error) {
	block := v1.PersistentVolumeBlock

	pvc := &v1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:        pvcName,
			Namespace:   pvcNamespace,
			Annotations: map[string]string{},
		},
		Spec: v1.PersistentVolumeClaimSpec{
			AccessModes: []v1.PersistentVolumeAccessMode{
				v1.ReadWriteMany,
			},
			Resources: v1.VolumeResourceRequirements{
				Requests: v1.ResourceList{
					"storage": *vol.VolumeSize,
				},
			},
			VolumeMode: &block,
		},
	}

	switch vol.VolumeType {
	case "image":
		vmImage, err := getImageByName(vol.ImageName, pvcNamespace, hvScope)
		if err != nil {
			return nil, errors.Wrapf(err, "unable to find VM image %s", vol.ImageName)
		}

		scName := vmImage.Status.StorageClassName
		pvc.Spec.StorageClassName = &scName
		pvc.Annotations[hvAnnotationImageID] = vmImage.Namespace + "/" + vmImage.Name

	case "storageClass":
		scName := vol.StorageClass
		pvc.Spec.StorageClassName = &scName
	}

	return pvc, nil
}

// getImageByName resolves a Harvester VirtualMachineImage by its display name.
// imageName can be "namespace/name" or just "name" (defaults to targetNamespace).
func getImageByName(imageName, defaultNamespace string, hvScope *Scope) (*harvesterv1beta1.VirtualMachineImage, error) {
	vmImageNamespacedName, err := locutil.GetNamespacedName(imageName, defaultNamespace)
	if err != nil {
		return nil, fmt.Errorf("imageName %q is malformed, expecting <NAMESPACE>/<NAME> format: %w", imageName, err)
	}

	foundImages, err := hvScope.HarvesterClient.HarvesterhciV1beta1().VirtualMachineImages(vmImageNamespacedName.Namespace).List(
		hvScope.Ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	for _, image := range foundImages.Items {
		if image.Spec.DisplayName == vmImageNamespacedName.Name || image.Name == vmImageNamespacedName.Name {
			return &image, nil
		}
	}

	return nil, fmt.Errorf(
		"VM image %s not found in namespace %s (searched by display name and resource name)",
		vmImageNamespacedName.Name, vmImageNamespacedName.Namespace,
	)
}

// buildVMTemplate creates a *kubevirtv1.VirtualMachineInstanceTemplateSpec from the CLI Flags and some computed values.
func buildVMTemplate(hvScope *Scope,
	disks []diskInfo, vmiLabels map[string]string,
) (vmTemplate *kubevirtv1.VirtualMachineInstanceTemplateSpec, err error) {
	var sshKey *harvesterv1beta1.KeyPair

	keyName := hvScope.HarvesterMachine.Spec.SSHKeyPair

	keyPairFullName, err := locutil.GetNamespacedName(keyName, hvScope.HarvesterCluster.Spec.TargetNamespace)
	if err != nil {
		return nil, errors.New("SSHKeyPair is HarvesterMachine is Malformed, expecting <NAMESPACE>/<NAME> format or simple DNS name without slash")
	}

	sshKey, err = hvScope.HarvesterClient.HarvesterhciV1beta1().KeyPairs(keyPairFullName.Namespace).Get(
		hvScope.Ctx, keyPairFullName.Name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = fmt.Errorf(
				`no keypair could be found in namespace %s, keypair was only referenced by name %s, 
try to specify the namespace using the format <NAMESPACE>/<NAME>`,
				keyPairFullName.Namespace, keyName)

			return nil, err
		}

		err = fmt.Errorf("error during getting keypair from Harvester: %w", err)

		return nil, err
	}

	hvScope.Logger.V(3).Info("SSH Key Name " + keyName + " given does exist!") //nolint:mnd

	// building cloud-init user data
	cloudInitBase := `package_update: true
packages:
  - qemu-guest-agent
  - iptables
runcmd:
  - - systemctl
    - enable
    - --now
    - qemu-guest-agent.service`
	cloudInitSSHSection := "\nssh_authorized_keys:\n  - " + sshKey.Spec.PublicKey + "\n"

	cloudInitUserData, err1 := getCloudInitData(hvScope)
	if err1 != nil {
		err = fmt.Errorf("error during getting cloud init user data from Harvester: %w", err1)

		return nil, err
	}

	// In DHCP mode, add dhclient bootcmd to work around wicked BPF filter bug on KubeVirt/virtio-net.
	// Wicked uses AF_PACKET SOCK_DGRAM with a BPF filter that has link-layer offsets, but on SOCK_DGRAM
	// the kernel presents network-layer data to BPF, causing all DHCP responses to be silently dropped.
	// ISC dhclient uses AF_PACKET SOCK_RAW (LPF) which works correctly.
	cloudInitDHCP := ""
	if hvScope.EffectiveNetworkConfig == nil && len(hvScope.HarvesterMachine.Spec.Networks) > 0 {
		cloudInitDHCP = buildDHCPCloudInit(hvScope)
	}

	finalCloudInit, err := locutil.MergeCloudInitData(cloudInitBase, cloudInitSSHSection, cloudInitDHCP, cloudInitUserData)
	if err != nil {
		err = fmt.Errorf("error during merging cloud init user data from Harvester: %w", err)

		return nil, err
	}

	// Build cloud-init secret data with lowercase keys (required by KubeVirt)
	secretData := map[string][]byte{
		"userdata": finalCloudInit,
	}

	// Build network-config for NICs. For static mode, generate v1 networkdata with IP configuration.
	// For DHCP mode, skip networkdata entirely — dhclient handles network setup via bootcmd,
	// and generating networkdata would cause wicked to interfere with the DHCP-assigned IP.
	if hvScope.EffectiveNetworkConfig != nil && len(hvScope.HarvesterMachine.Spec.Networks) > 0 {
		secretData["networkdata"] = []byte(buildNetworkDataStatic(hvScope))
	}

	// create cloud-init secret for reference in Harvester.
	cloudInitSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      hvScope.HarvesterMachine.Name + "-cloud-init",
			Namespace: hvScope.HarvesterCluster.Spec.TargetNamespace,
		},
		Data: secretData,
	}

	hvScope.Logger.V(5).Info("cloud-init final value is " + string(finalCloudInit)) //nolint:mnd

	// check if secret already exists
	_, err = hvScope.HarvesterClient.CoreV1().Secrets(hvScope.HarvesterCluster.Spec.TargetNamespace).Get(
		hvScope.Ctx, hvScope.HarvesterMachine.Name+"-cloud-init", metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			hvScope.Logger.V(3).Info("unable to get cloud-init secret, error was different than NotFound")
		} else {
			_, err = hvScope.HarvesterClient.CoreV1().Secrets(hvScope.HarvesterCluster.Spec.TargetNamespace).Create(
				hvScope.Ctx, cloudInitSecret, metav1.CreateOptions{})
			if err != nil {
				return nil, errors.Wrap(err, "unable to create cloud-init secret")
			}
		}
	} else {
		_, err = hvScope.HarvesterClient.CoreV1().Secrets(hvScope.HarvesterCluster.Spec.TargetNamespace).Update(
			hvScope.Ctx, cloudInitSecret, metav1.UpdateOptions{})
		if err != nil {
			return nil, errors.Wrap(err, "unable to update cloud-init secret")
		}
	}

	// Build volumes and disks lists dynamically from all configured volumes
	volumes := make([]kubevirtv1.Volume, 0, len(disks)+1)
	kvDisks := make([]kubevirtv1.Disk, 0, len(disks)+1)

	for _, d := range disks {
		diskName := fmt.Sprintf("disk-%d", d.index)

		volumes = append(volumes, kubevirtv1.Volume{
			Name: diskName,
			VolumeSource: kubevirtv1.VolumeSource{
				PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: v1.PersistentVolumeClaimVolumeSource{
						ClaimName: d.pvcName,
					},
				},
			},
		})

		disk := kubevirtv1.Disk{
			Name: diskName,
			DiskDevice: kubevirtv1.DiskDevice{
				Disk: &kubevirtv1.DiskTarget{
					Bus: "virtio",
				},
			},
		}

		// Apply boot order if specified in the volume spec
		if d.index < len(hvScope.HarvesterMachine.Spec.Volumes) {
			bootOrder := hvScope.HarvesterMachine.Spec.Volumes[d.index].BootOrder
			if bootOrder > 0 {
				bo := uint(bootOrder)
				disk.BootOrder = &bo
			}
		}

		kvDisks = append(kvDisks, disk)
	}

	// Append cloud-init disk last
	volumes = append(volumes, kubevirtv1.Volume{
		Name: "cloudinitdisk",
		VolumeSource: kubevirtv1.VolumeSource{
			CloudInitNoCloud: &kubevirtv1.CloudInitNoCloudSource{
				UserDataSecretRef: &v1.LocalObjectReference{
					Name: hvScope.HarvesterMachine.Name + "-cloud-init",
				},
				NetworkDataSecretRef: &v1.LocalObjectReference{
					Name: hvScope.HarvesterMachine.Name + "-cloud-init",
				},
			},
		},
	})
	kvDisks = append(kvDisks, kubevirtv1.Disk{
		Name: "cloudinitdisk",
		DiskDevice: kubevirtv1.DiskDevice{
			Disk: &kubevirtv1.DiskTarget{
				Bus: "virtio",
			},
		},
	})

	// Build network interfaces
	interfaces := buildNetworkInterfaces(hvScope.HarvesterMachine)

	// Build affinity with user-specified NodeAffinity and WorkloadAffinity
	affinity := buildAffinity(hvScope)

	// Build disk names annotation as JSON array
	diskNames := make([]string, 0, len(disks))

	for _, d := range disks {
		diskNames = append(diskNames, d.pvcName)
	}

	diskNamesJSON, err := json.Marshal(diskNames)
	if err != nil {
		return nil, errors.Wrap(err, "unable to marshal disk names to JSON")
	}

	vmTemplate = &kubevirtv1.VirtualMachineInstanceTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				hvAnnotationDiskNames: string(diskNamesJSON),
				hvAnnotationSSH:       "[\"" + sshKey.GetName() + "\"]",
				cpVMLabelKey:          cpVMLabelValuePrefix + "-" + hvScope.Cluster.Name,
			},
			Labels: vmiLabels,
		},
		Spec: kubevirtv1.VirtualMachineInstanceSpec{
			Hostname: hvScope.HarvesterMachine.Name,
			Networks: getKubevirtNetworksFromHarvesterMachine(hvScope.HarvesterMachine),
			Volumes:  volumes,
			Domain: kubevirtv1.DomainSpec{
				CPU: &kubevirtv1.CPU{
					Cores:   hvScope.HarvesterMachine.Spec.CPU,
					Sockets: 1,
					Threads: 1,
				},
				Devices: kubevirtv1.Devices{
					Inputs: []kubevirtv1.Input{
						{
							Bus:  "usb",
							Type: "tablet",
							Name: "tablet",
						},
					},
					Interfaces: interfaces,
					Disks:      kvDisks,
				},
				Memory: &kubevirtv1.Memory{
					Guest: new(resource.MustParse(hvScope.HarvesterMachine.Spec.Memory)),
				},
				Resources: kubevirtv1.ResourceRequirements{
					Requests: v1.ResourceList{
						"memory": resource.MustParse(hvScope.HarvesterMachine.Spec.Memory),
					},
					Limits: v1.ResourceList{
						"cpu":    *resource.NewQuantity(int64(hvScope.HarvesterMachine.Spec.CPU), resource.DecimalSI),
						"memory": resource.MustParse(hvScope.HarvesterMachine.Spec.Memory),
					},
				},
			},
			Affinity: affinity,
		},
	}

	applyFirmwareAndTPM(hvScope.HarvesterMachine, &vmTemplate.Spec.Domain)

	return vmTemplate, nil
}

// applyFirmwareAndTPM maps the optional firmware and TPM settings of the
// HarvesterMachine onto the kubevirt domain. The EFI secureBoot flag is always
// pinned explicitly because kubevirt defaults it to true when EFI is enabled,
// and Secure Boot additionally requires the SMM CPU feature.
func applyFirmwareAndTPM(machine *infrav1.HarvesterMachine, domain *kubevirtv1.DomainSpec) {
	if firmware := machine.Spec.Firmware; firmware != nil && firmware.EFI {
		domain.Firmware = &kubevirtv1.Firmware{
			Bootloader: &kubevirtv1.Bootloader{
				EFI: &kubevirtv1.EFI{SecureBoot: ptr.To(firmware.SecureBoot)},
			},
		}

		if firmware.SecureBoot {
			domain.Features = &kubevirtv1.Features{
				SMM: &kubevirtv1.FeatureState{Enabled: ptr.To(true)},
			}
		}
	}

	if tpm := machine.Spec.TPM; tpm != nil && tpm.Enabled {
		domain.Devices.TPM = &kubevirtv1.TPMDevice{Persistent: ptr.To(tpm.Persistent)}
	}
}

// buildNetworkInterfaces creates kubevirt Interface specs for each network.
func buildNetworkInterfaces(machine *infrav1.HarvesterMachine) []kubevirtv1.Interface {
	interfaces := make([]kubevirtv1.Interface, 0, len(machine.Spec.Networks))

	for i := range machine.Spec.Networks {
		interfaces = append(interfaces, kubevirtv1.Interface{
			Name:                   "nic-" + strconv.Itoa(i+1),
			Model:                  "virtio",
			InterfaceBindingMethod: kubevirtv1.DefaultBridgeNetworkInterface().InterfaceBindingMethod,
		})
	}

	return interfaces
}

// buildNetworkDataStatic generates cloud-init network-config v1 with static IP on eth0
// and DHCP on additional NICs.
func buildNetworkDataStatic(hvScope *Scope) string {
	var b strings.Builder

	netCfg := hvScope.EffectiveNetworkConfig
	subnetMask := "255.255.0.0" // default

	if vmNetCfg := effectiveVMNetworkConfig(hvScope); vmNetCfg != nil {
		subnetMask = vmNetCfg.SubnetMask
	}

	b.WriteString("version: 1\nconfig:\n")

	for i := range hvScope.HarvesterMachine.Spec.Networks {
		ethName := "eth" + strconv.Itoa(i)

		fmt.Fprintf(&b, "  - type: physical\n    name: %s\n    subnets:\n", ethName)

		if i == 0 {
			fmt.Fprintf(&b, "      - type: static\n        address: %s\n        netmask: %s\n        gateway: %s\n",
				netCfg.Address, subnetMask, netCfg.Gateway)
		} else {
			b.WriteString("      - type: dhcp\n")
		}
	}

	if len(netCfg.DNSServers) > 0 {
		b.WriteString("  - type: nameserver\n    address:\n")

		for _, dns := range netCfg.DNSServers {
			fmt.Fprintf(&b, "      - %s\n", dns)
		}

		if len(netCfg.DNSSearch) > 0 {
			b.WriteString("    search:\n")

			for _, search := range netCfg.DNSSearch {
				fmt.Fprintf(&b, "      - %s\n", search)
			}
		}
	}

	return b.String()
}

// buildDHCPCloudInit generates cloud-init YAML with write_files and bootcmd entries that
// configure network interfaces via dhclient. This works around wicked's BPF filter bug:
// wicked uses AF_PACKET SOCK_DGRAM with a BPF filter that has link-layer offsets, but on
// SOCK_DGRAM the kernel runs BPF against network-layer data, causing all DHCP responses
// to be silently dropped. ISC dhclient uses SOCK_RAW (LPF) which works correctly.
func buildDHCPCloudInit(hvScope *Scope) string {
	var b strings.Builder

	// dhclient-script: configures IP from DHCP lease, converts netmask to CIDR prefix.
	// SLES ships a wicked-dependent dhclient-script that fails, so we provide our own.
	// IMPORTANT: the script must be created in bootcmd (not write_files) because
	// cloud-init runs bootcmd BEFORE write_files, and dhclient needs the script to exist.
	dhclientScript := dhclientScriptContent()

	scriptPath := "/usr/local/bin/dhclient-script-caphv.sh"

	b.WriteString("bootcmd:\n")

	// First bootcmd entry: create the dhclient-script file via heredoc
	b.WriteString("  - |\n")
	fmt.Fprintf(&b, "    cat > %s << 'DHCSCRIPT'\n", scriptPath)

	for line := range strings.SplitSeq(dhclientScript, "\n") {
		fmt.Fprintf(&b, "    %s\n", line)
	}

	b.WriteString("    DHCSCRIPT\n")
	fmt.Fprintf(&b, "    chmod +x %s\n", scriptPath)

	// Then one entry per NIC to run dhclient
	for i := range hvScope.HarvesterMachine.Spec.Networks {
		ethName := "eth" + strconv.Itoa(i)
		// -1 = try once then fork to background (parent exits, bootcmd continues).
		// Do NOT use -d (foreground) as it would block cloud-init forever.
		fmt.Fprintf(&b,
			"  - ['dhclient', '-1', '-sf', '%s', '-lf', '/tmp/dhclient-%s.lease', '-pf', '/tmp/dhclient-%s.pid', '%s']\n",
			scriptPath, ethName, ethName, ethName)
	}

	return b.String()
}

//nolint:dupword // bash nested loops have consecutive "done" keywords
func dhclientScriptContent() string {
	return `#!/bin/bash
case "$reason" in
  PREINIT)
    ip link set dev $interface up
    ;;
  BOUND|RENEW|REBIND|REBOOT)
    ip addr flush dev $interface 2>/dev/null
    IFS=. read -r a b c d <<< "$new_subnet_mask"
    prefix=0
    for octet in $a $b $c $d; do
      while [ $octet -gt 0 ]; do
        prefix=$((prefix + (octet & 1)))
        octet=$((octet >> 1))
      done
    done
    ip addr add $new_ip_address/$prefix dev $interface
    if [ -n "$new_routers" ]; then
      ip route add default via $new_routers dev $interface 2>/dev/null
    fi
    if [ -n "$new_domain_name_servers" ]; then
      : > /etc/resolv.conf
      for dns in $new_domain_name_servers; do
        echo "nameserver $dns" >> /etc/resolv.conf
      done
    fi
    ;;
esac`
}

// buildAffinity merges user-specified NodeAffinity and WorkloadAffinity with default PodAntiAffinity.
func buildAffinity(hvScope *Scope) *v1.Affinity {
	affinity := &v1.Affinity{
		PodAntiAffinity: &v1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
				{
					Weight: int32(1),
					PodAffinityTerm: v1.PodAffinityTerm{
						TopologyKey: "kubernetes.io/hostname",
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"harvesterhci.io/vmNamePrefix": hvScope.HarvesterMachine.Name,
							},
						},
					},
				},
			},
		},
	}

	// Merge user-specified NodeAffinity
	if hvScope.HarvesterMachine.Spec.NodeAffinity != nil {
		affinity.NodeAffinity = hvScope.HarvesterMachine.Spec.NodeAffinity
	}

	// Pin the VM to its failure domain (appended to every user term so the
	// user constraints stay AND-ed with the domain one)
	if failureDomain := effectiveFailureDomain(hvScope); failureDomain != "" {
		key, value := failureDomainNodeSelector(hvScope.HarvesterCluster, failureDomain)
		requirement := v1.NodeSelectorRequirement{Key: key, Operator: v1.NodeSelectorOpIn, Values: []string{value}}

		if affinity.NodeAffinity == nil {
			affinity.NodeAffinity = &v1.NodeAffinity{}
		}

		if affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
			affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution = &v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{{}},
			}
		}

		terms := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		for i := range terms {
			terms[i].MatchExpressions = append(terms[i].MatchExpressions, requirement)
		}
	}

	// Merge user-specified WorkloadAffinity (PodAffinity)
	if hvScope.HarvesterMachine.Spec.WorkloadAffinity != nil {
		affinity.PodAffinity = hvScope.HarvesterMachine.Spec.WorkloadAffinity
	}

	return affinity
}

func getKubevirtNetworksFromHarvesterMachine(harvesterMachine *infrav1.HarvesterMachine) []kubevirtv1.Network {
	networks := make([]kubevirtv1.Network, 0, len(harvesterMachine.Spec.Networks))
	for i, network := range harvesterMachine.Spec.Networks {
		networks = append(networks, kubevirtv1.Network{
			Name: "nic-" + strconv.Itoa(i+1),
			NetworkSource: kubevirtv1.NetworkSource{
				Multus: &kubevirtv1.MultusNetwork{
					NetworkName: network,
				},
			},
		})
	}

	return networks
}

func getCloudInitData(hvScope *Scope) (string, error) {
	dataSecretNamespacedName := types.NamespacedName{
		Namespace: hvScope.Machine.Namespace,
		Name:      *hvScope.Machine.Spec.Bootstrap.DataSecretName,
	}

	dataSecret := &v1.Secret{}

	err := hvScope.ReconcilerClient.Get(hvScope.Ctx, dataSecretNamespacedName, dataSecret)
	if err != nil {
		return "", err
	}

	userData, ok := dataSecret.Data["value"]
	if !ok {
		return "", fmt.Errorf("no userData key found in secret %s", dataSecretNamespacedName)
	}

	return string(userData), nil
}

// effectiveVMNetworkConfig returns the pool-based network configuration that
// applies to the machine: the machine-level spec.vmNetworkConfig when set,
// otherwise the cluster-level one from the HarvesterCluster.
//
//nolint:funcorder
func effectiveVMNetworkConfig(hvScope *Scope) *infrav1.VMNetworkConfig {
	if hvScope.HarvesterMachine.Spec.VMNetworkConfig != nil {
		return hvScope.HarvesterMachine.Spec.VMNetworkConfig
	}

	return hvScope.HarvesterCluster.Spec.VMNetworkConfig
}

// allocateVMIP allocates an IP from the effective VM IP pool configuration
// (machine-level or cluster-level) for this machine.
// It is idempotent: if an IP is already allocated, it returns early.
//
//nolint:funcorder
func (r *HarvesterMachineReconciler) allocateVMIP(hvScope *Scope) error {
	machine := hvScope.HarvesterMachine
	logger := hvScope.Logger

	// Return early if already allocated (idempotent)
	if machine.Status.AllocatedIPAddress != "" {
		logger.V(3).Info("VM IP already allocated", "ip", machine.Status.AllocatedIPAddress)

		return nil
	}

	vmNetCfg := effectiveVMNetworkConfig(hvScope)
	if vmNetCfg == nil {
		return errors.New("VMNetworkConfig is nil on both the HarvesterMachine and the HarvesterCluster")
	}

	poolRefs := vmNetCfg.GetIPPoolRefs()
	if len(poolRefs) == 0 {
		return errors.New("no IPPool references configured, ensure reconcileVMIPPool has run")
	}

	machineID := machine.Namespace + "/" + machine.Name

	// Try each pool in order until allocation succeeds. Pools assigned to a
	// network the machine is not attached to are skipped, so that clusters can
	// dedicate distinct pools to control-plane and worker networks by listing
	// them all in ipPoolRefs.
	var lastErr error

	skippedNetwork := 0

	for _, poolRef := range poolRefs {
		pool, err := hvScope.HarvesterClient.LoadbalancerV1beta1().IPPools().Get(
			hvScope.Ctx, poolRef, metav1.GetOptions{})
		if err != nil {
			logger.V(1).Info("Failed to get IP pool, trying next", "pool", poolRef, "error", err)
			lastErr = errors.Wrapf(err, "failed to get VM IP pool %s", poolRef)

			continue
		}

		// Check for existing allocation in this pool
		if pool.Status.Allocated != nil {
			for ip, id := range pool.Status.Allocated {
				if id == machineID {
					machine.Status.AllocatedIPAddress = ip
					machine.Status.AllocatedPoolRef = poolRef
					logger.Info("Found existing IP allocation in pool", "ip", ip, "pool", poolRef)

					return nil
				}
			}
		}

		if !locutil.PoolMatchesNetworks(pool, machine.Spec.Networks) {
			logger.V(1).Info("Pool assigned to another network, trying next",
				"pool", poolRef, "poolNetwork", pool.Spec.Selector.Network, "machineNetworks", machine.Spec.Networks)

			skippedNetwork++

			continue
		}

		// Try to allocate from this pool
		caphvmetrics.IPPoolAllocationsTotal.Inc()

		allocatedIP, allocErr := locutil.AllocateVMIPFromPool(pool, machineID)
		if allocErr != nil {
			logger.V(1).Info("Pool exhausted or allocation failed, trying next", "pool", poolRef, "error", allocErr)
			lastErr = errors.Wrapf(allocErr, "failed to allocate from pool %s", poolRef)

			continue
		}

		// Update pool in Harvester
		_, err = hvScope.HarvesterClient.LoadbalancerV1beta1().IPPools().Update(
			hvScope.Ctx, pool, metav1.UpdateOptions{})
		if err != nil {
			caphvmetrics.IPPoolAllocationErrorsTotal.Inc()

			return errors.Wrapf(err, "failed to update VM IP pool %s after allocation", poolRef)
		}

		machine.Status.AllocatedIPAddress = allocatedIP
		machine.Status.AllocatedPoolRef = poolRef
		logger.Info("Allocated VM IP from pool", "ip", allocatedIP, "pool", poolRef, "machine", machine.Name)

		return nil
	}

	// All pools exhausted
	caphvmetrics.IPPoolAllocationErrorsTotal.Inc()

	if lastErr == nil && skippedNetwork > 0 {
		return errors.Errorf(
			"no IP pool among %v matches the machine networks %v (pools are assigned to other networks)",
			poolRefs, machine.Spec.Networks)
	}

	return errors.Wrap(lastErr, "all configured IP pools exhausted")
}

// releaseVMIP releases the allocated IP back to the pool during machine deletion.
// Errors are logged as warnings but do not block deletion.
//
//nolint:funcorder
func (r *HarvesterMachineReconciler) releaseVMIP(hvScope *Scope) {
	machine := hvScope.HarvesterMachine
	logger := hvScope.Logger

	if machine.Status.AllocatedIPAddress == "" {
		return
	}

	// Determine which pool to release from: use AllocatedPoolRef if available,
	// fall back to first configured pool for backward compatibility.
	vmNetCfg := effectiveVMNetworkConfig(hvScope)

	poolRef := machine.Status.AllocatedPoolRef
	if poolRef == "" && vmNetCfg != nil {
		poolRefs := vmNetCfg.GetIPPoolRefs()
		if len(poolRefs) > 0 {
			poolRef = poolRefs[0]
		}
	}

	if poolRef == "" {
		logger.Info("No pool reference for IP release, skipping")

		return
	}

	pool, err := hvScope.HarvesterClient.LoadbalancerV1beta1().IPPools().Get(
		hvScope.Ctx, poolRef, metav1.GetOptions{})
	if err != nil {
		logger.Info("Warning: failed to get VM IP pool for release, skipping", "error", err)

		return
	}

	store := locutil.NewStore(pool)
	ip := net.ParseIP(machine.Status.AllocatedIPAddress)

	if ip == nil {
		logger.Info("Warning: failed to parse allocated IP for release", "ip", machine.Status.AllocatedIPAddress)

		return
	}

	releaseErr := store.Release(ip)
	if releaseErr != nil {
		logger.Info("Warning: failed to release IP from pool", "error", releaseErr, "ip", machine.Status.AllocatedIPAddress)

		return
	}

	_, err = hvScope.HarvesterClient.LoadbalancerV1beta1().IPPools().Update(
		hvScope.Ctx, pool, metav1.UpdateOptions{})
	if err != nil {
		logger.Info("Warning: failed to update pool after IP release", "error", err)

		return
	}

	caphvmetrics.IPPoolReleasesTotal.Inc()
	logger.Info("Released VM IP back to pool", "ip", machine.Status.AllocatedIPAddress, "machine", machine.Name)
}

// removeEtcdMemberIfControlPlane removes the etcd member for a control-plane machine
// from the workload cluster before VM deletion. This prevents stale etcd members from
// blocking replacement nodes from joining the cluster.
// Errors are logged as warnings but do not block deletion.
//
//nolint:funcorder
func (r *HarvesterMachineReconciler) removeEtcdMemberIfControlPlane(hvScope *Scope) {
	logger := hvScope.Logger

	// Skip if not a control-plane machine
	if _, ok := hvScope.HarvesterMachine.Labels[clusterv1.MachineControlPlaneLabel]; !ok {
		return
	}

	workloadConfig, err := getWorkloadClusterConfig(hvScope)
	if err != nil {
		logger.Info("Warning: failed to get workload cluster config for etcd cleanup, skipping",
			"error", err)

		return
	}

	caphvmetrics.EtcdMemberRemoveTotal.Inc()
	locutil.RemoveEtcdMember(hvScope.Ctx, *logger, workloadConfig, hvScope.HarvesterMachine.Name)
}

// ReconcileDelete deletes a HarvesterMachine with all its dependencies.
func (r *HarvesterMachineReconciler) ReconcileDelete(hvScope Scope) (res ctrl.Result, rerr error) {
	reconcileStart := time.Now()

	caphvmetrics.MachineDeleteTotal.Inc()

	defer func() {
		caphvmetrics.MachineReconcileDuration.WithLabelValues("delete").Observe(time.Since(reconcileStart).Seconds())

		if rerr != nil {
			caphvmetrics.MachineDeleteErrorsTotal.Inc()
		}

		// Clean up gauge on deletion
		clusterName := hvScope.Cluster.Namespace + "/" + hvScope.Cluster.Name
		machineName := hvScope.HarvesterMachine.Namespace + "/" + hvScope.HarvesterMachine.Name
		caphvmetrics.MachineStatus.DeleteLabelValues(clusterName, machineName)
	}()

	logger := log.FromContext(hvScope.Ctx)
	logger.Info("Deleting HarvesterMachine ...")

	// Release allocated IP back to pool before deletion
	r.releaseVMIP(&hvScope)

	// Remove etcd member from workload cluster before VM deletion (control-plane only)
	r.removeEtcdMemberIfControlPlane(&hvScope)

	err := hvScope.HarvesterClient.CoreV1().Secrets(hvScope.HarvesterCluster.Spec.TargetNamespace).Delete(
		hvScope.Ctx, hvScope.HarvesterMachine.Name+"-cloud-init", metav1.DeleteOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Error(err, "unable to delete cloud-init secret, error was different than NotFound")

			return ctrl.Result{Requeue: true}, err
		}

		logger.Info("cloud-init secret not found, doing nothing")
	}

	logger.V(5).Info("cloud-init secret deleted successfully: " + hvScope.HarvesterMachine.Name + "-cloud-init")

	targetNS := hvScope.HarvesterCluster.Spec.TargetNamespace
	machineName := hvScope.HarvesterMachine.Name

	vm, err := hvScope.HarvesterClient.KubevirtV1().VirtualMachines(targetNS).Get(
		hvScope.Ctx, machineName, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			logger.Error(err, "unable to get VM, error was different than NotFound")

			return ctrl.Result{Requeue: true}, err
		}

		// VM is gone — clean up any orphaned PVCs by name prefix
		logger.Info("No VM found, cleaning up orphaned PVCs")
		r.deletePVCsByPrefix(hvScope.Ctx, &hvScope, targetNS, machineName+"-disk-")
	} else {
		logger.V(5).Info("found VM: " + vm.Namespace + "/" + vm.Name)

		if (vm != &kubevirtv1.VirtualMachine{}) {
			// Delete VM first
			err = hvScope.HarvesterClient.KubevirtV1().VirtualMachines(targetNS).Delete(
				hvScope.Ctx, machineName, metav1.DeleteOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				logger.Error(err, "unable to delete VM, error was different than NotFound")

				return ctrl.Result{Requeue: true}, err
			}

			logger.Info("VM delete requested, requeuing to wait for termination before PVC cleanup")

			return ctrl.Result{RequeueAfter: requeueDelay}, nil
		}
	}

	// Remove both new and legacy finalizers to handle objects from before the migration.
	controllerutil.RemoveFinalizer(hvScope.HarvesterMachine, infrav1.MachineFinalizerLegacy)

	if ok := controllerutil.RemoveFinalizer(hvScope.HarvesterMachine, infrav1.MachineFinalizer); !ok {
		return ctrl.Result{}, fmt.Errorf("unable to remove finalizer %s from HarvesterMachine %s/%s",
			infrav1.MachineFinalizer,
			hvScope.HarvesterMachine.Namespace,
			hvScope.HarvesterMachine.Name)
	}

	return ctrl.Result{}, nil
}

// deletePVCsByPrefix lists all PVCs in the namespace and deletes those whose name
// starts with the given prefix. This handles orphaned PVCs left behind when VM
// deletion completes before PVC cleanup.
func (r *HarvesterMachineReconciler) deletePVCsByPrefix(ctx context.Context, hvScope *Scope, namespace, prefix string) {
	logger := hvScope.Logger

	pvcList, err := hvScope.HarvesterClient.CoreV1().PersistentVolumeClaims(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Info("Warning: failed to list PVCs for cleanup", "error", err)

		return
	}

	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		if !strings.HasPrefix(pvc.Name, prefix) {
			continue
		}

		err = hvScope.HarvesterClient.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, pvc.Name, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			logger.Info("Warning: failed to delete orphaned PVC", "pvc", pvc.Name, "error", err)

			continue
		}

		logger.Info("Deleted orphaned PVC", "pvc", pvc.Name)
	}
}
