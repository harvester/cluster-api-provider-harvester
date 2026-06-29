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

package v1beta1

import (
	"context"
	"fmt"
	"net"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// HarvesterClusterValidator implements admission.Validator for HarvesterCluster.
type HarvesterClusterValidator struct{}

// SetupHarvesterClusterWebhookWithManager sets up the validating webhook for HarvesterCluster.
func SetupHarvesterClusterWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &HarvesterCluster{}).
		WithValidator(&HarvesterClusterValidator{}).
		Complete()
}

//nolint:lll
// +kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1beta1-harvestercluster,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=harvesterclusters,verbs=create;update,versions=v1beta1,name=vharvestercluster.v1beta1.kb.io,admissionReviewVersions=v1,serviceName=caphv-webhook-service,serviceNamespace=caphv-system

var _ admission.Validator[*HarvesterCluster] = &HarvesterClusterValidator{}

// ValidateCreate implements admission.Validator.
func (v *HarvesterClusterValidator) ValidateCreate(_ context.Context, obj *HarvesterCluster) (admission.Warnings, error) {
	return validateHarvesterCluster(obj)
}

// ValidateUpdate implements admission.Validator.
func (v *HarvesterClusterValidator) ValidateUpdate(_ context.Context, _, newObj *HarvesterCluster) (admission.Warnings, error) {
	return validateHarvesterCluster(newObj)
}

// ValidateDelete implements admission.Validator.
func (v *HarvesterClusterValidator) ValidateDelete(_ context.Context, _ *HarvesterCluster) (admission.Warnings, error) {
	return nil, nil
}

func validateHarvesterCluster(r *HarvesterCluster) (admission.Warnings, error) {
	var errs []string

	if r.Spec.TargetNamespace == "" {
		errs = append(errs, "spec.targetNamespace is required")
	}

	if r.Spec.IdentitySecret.Name == "" {
		errs = append(errs, "spec.identitySecret.name is required")
	}

	if r.Spec.IdentitySecret.Namespace == "" {
		errs = append(errs, "spec.identitySecret.namespace is required")
	}

	if r.Spec.LoadBalancerConfig.IPAMType != IPAMType(DHCP) && r.Spec.LoadBalancerConfig.IPAMType != IPAMType(POOL) {
		errs = append(errs, fmt.Sprintf("spec.loadBalancerConfig.ipamType must be %q or %q", DHCP, POOL))
	}

	if r.Spec.VMNetworkConfig != nil {
		vmCfg := r.Spec.VMNetworkConfig
		if len(vmCfg.GetIPPoolRefs()) == 0 && vmCfg.IPPool == nil {
			errs = append(errs, "spec.vmNetworkConfig requires one of ipPoolRef, ipPoolRefs or ipPool")
		}

		if vmCfg.Gateway == "" {
			errs = append(errs, "spec.vmNetworkConfig.gateway is required")
		} else if net.ParseIP(vmCfg.Gateway) == nil {
			errs = append(errs, fmt.Sprintf("spec.vmNetworkConfig.gateway %q is not a valid IP address", vmCfg.Gateway))
		}

		if vmCfg.SubnetMask == "" {
			errs = append(errs, "spec.vmNetworkConfig.subnetMask is required")
		} else if net.ParseIP(vmCfg.SubnetMask) == nil {
			errs = append(errs, fmt.Sprintf("spec.vmNetworkConfig.subnetMask %q is not a valid IP address", vmCfg.SubnetMask))
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("validation failed for HarvesterCluster %s/%s: %s",
			r.Namespace, r.Name, strings.Join(errs, "; "))
	}

	return nil, nil
}
