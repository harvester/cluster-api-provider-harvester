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

package v1alpha1

import (
	"context"
	"fmt"
	"net"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"k8s.io/apimachinery/pkg/api/resource"
)

// HarvesterMachineValidator implements admission.Validator for HarvesterMachine.
type HarvesterMachineValidator struct{}

// SetupHarvesterMachineWebhookWithManager sets up the validating webhook for HarvesterMachine.
func SetupHarvesterMachineWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &HarvesterMachine{}).
		WithValidator(&HarvesterMachineValidator{}).
		Complete()
}

//nolint:lll
// +kubebuilder:webhook:path=/validate-infrastructure-cluster-x-k8s-io-v1alpha1-harvestermachine,mutating=false,failurePolicy=fail,sideEffects=None,groups=infrastructure.cluster.x-k8s.io,resources=harvestermachines,verbs=create;update,versions=v1alpha1,name=vharvestermachine.kb.io,admissionReviewVersions=v1,serviceName=caphv-webhook-service,serviceNamespace=caphv-system

var _ admission.Validator[*HarvesterMachine] = &HarvesterMachineValidator{}

// ValidateCreate implements admission.Validator.
func (v *HarvesterMachineValidator) ValidateCreate(_ context.Context, obj *HarvesterMachine) (admission.Warnings, error) {
	return validateHarvesterMachine(obj)
}

// ValidateUpdate implements admission.Validator.
func (v *HarvesterMachineValidator) ValidateUpdate(_ context.Context, _, newObj *HarvesterMachine) (admission.Warnings, error) {
	return validateHarvesterMachine(newObj)
}

// ValidateDelete implements admission.Validator.
func (v *HarvesterMachineValidator) ValidateDelete(_ context.Context, _ *HarvesterMachine) (admission.Warnings, error) {
	return nil, nil
}

func validateHarvesterMachine(r *HarvesterMachine) (admission.Warnings, error) {
	var errs []string

	if r.Spec.CPU == 0 {
		errs = append(errs, "spec.cpu must be greater than 0")
	}

	if r.Spec.Memory == "" {
		errs = append(errs, "spec.memory is required")
	} else {
		_, err := resource.ParseQuantity(r.Spec.Memory)
		if err != nil {
			errs = append(errs, fmt.Sprintf("spec.memory %q is not a valid resource quantity: %v", r.Spec.Memory, err))
		}
	}

	if r.Spec.SSHUser == "" {
		errs = append(errs, "spec.sshUser is required")
	}

	if r.Spec.SSHKeyPair == "" {
		errs = append(errs, "spec.sshKeyPair is required")
	}

	if len(r.Spec.Volumes) == 0 {
		errs = append(errs, "spec.volumes must contain at least one volume")
	}

	for i, vol := range r.Spec.Volumes {
		if vol.VolumeType != "image" && vol.VolumeType != "storageClass" {
			errs = append(errs, fmt.Sprintf("spec.volumes[%d].volumeType must be 'image' or 'storageClass'", i))
		}

		if vol.VolumeType == "image" && vol.ImageName == "" {
			errs = append(errs, fmt.Sprintf("spec.volumes[%d].imageName is required when volumeType is 'image'", i))
		}

		if vol.VolumeType == "storageClass" && vol.StorageClass == "" {
			errs = append(errs, fmt.Sprintf("spec.volumes[%d].storageClass is required when volumeType is 'storageClass'", i))
		}
	}

	if len(r.Spec.Networks) == 0 {
		errs = append(errs, "spec.networks must contain at least one network")
	}

	if r.Spec.NetworkConfig != nil {
		if r.Spec.NetworkConfig.Address == "" {
			errs = append(errs, "spec.networkConfig.address is required when networkConfig is set")
		}

		if r.Spec.NetworkConfig.Gateway == "" {
			errs = append(errs, "spec.networkConfig.gateway is required when networkConfig is set")
		} else if net.ParseIP(r.Spec.NetworkConfig.Gateway) == nil {
			errs = append(errs, fmt.Sprintf("spec.networkConfig.gateway %q is not a valid IP address", r.Spec.NetworkConfig.Gateway))
		}
	}

	errs = append(errs, validateMachineVMNetworkConfig(r)...)

	if fw := r.Spec.Firmware; fw != nil && fw.SecureBoot && !fw.EFI {
		errs = append(errs, "spec.firmware.secureBoot requires spec.firmware.efi to be true")
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("validation failed for HarvesterMachine %s/%s: %s",
			r.Namespace, r.Name, strings.Join(errs, "; "))
	}

	return nil, nil
}

// validateMachineVMNetworkConfig checks the machine-level pool-based network
// configuration override.
func validateMachineVMNetworkConfig(r *HarvesterMachine) []string {
	vmCfg := r.Spec.VMNetworkConfig
	if vmCfg == nil {
		return nil
	}

	var errs []string

	if r.Spec.NetworkConfig != nil {
		errs = append(errs, "spec.vmNetworkConfig and spec.networkConfig are mutually exclusive")
	}

	if vmCfg.IPPool != nil {
		errs = append(errs, "spec.vmNetworkConfig.ipPool is not supported at the machine level; use ipPoolRef or ipPoolRefs")
	}

	if len(vmCfg.GetIPPoolRefs()) == 0 {
		errs = append(errs, "spec.vmNetworkConfig requires one of ipPoolRef or ipPoolRefs")
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

	return errs
}
