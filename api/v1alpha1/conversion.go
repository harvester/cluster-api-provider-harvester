package v1alpha1

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/conversion"

	infrav1 "github.com/harvester/cluster-api-provider-harvester/api/v1beta1"
)

// v1alpha1 is a spoke of the v1beta1 hub. The schemas are identical except for the
// deprecated terminal failure fields (status.failureReason/failureMessage), which only
// exist in v1alpha1 and are NOT carried across conversion: the controller stopped
// writing them in v0.4.0 and failures surface through the conditions instead.

func convertVMNetworkConfigTo(src *VMNetworkConfig) *infrav1.VMNetworkConfig {
	if src == nil {
		return nil
	}

	dst := infrav1.VMNetworkConfig{
		IPPoolRef:  src.IPPoolRef,
		IPPoolRefs: src.IPPoolRefs,
		Gateway:    src.Gateway,
		SubnetMask: src.SubnetMask,
		DNSServers: src.DNSServers,
		DNSSearch:  src.DNSSearch,
	}

	if src.IPPool != nil {
		pool := infrav1.IpPool(*src.IPPool)
		dst.IPPool = &pool
	}

	return &dst
}

func convertVMNetworkConfigFrom(src *infrav1.VMNetworkConfig) *VMNetworkConfig {
	if src == nil {
		return nil
	}

	dst := VMNetworkConfig{
		IPPoolRef:  src.IPPoolRef,
		IPPoolRefs: src.IPPoolRefs,
		Gateway:    src.Gateway,
		SubnetMask: src.SubnetMask,
		DNSServers: src.DNSServers,
		DNSSearch:  src.DNSSearch,
	}

	if src.IPPool != nil {
		pool := IpPool(*src.IPPool)
		dst.IPPool = &pool
	}

	return &dst
}

func convertClusterSpecTo(src *HarvesterClusterSpec) infrav1.HarvesterClusterSpec {
	dst := infrav1.HarvesterClusterSpec{
		Server:               src.Server,
		IdentitySecret:       infrav1.SecretKey(src.IdentitySecret),
		ControlPlaneEndpoint: src.ControlPlaneEndpoint,
		TargetNamespace:      src.TargetNamespace,
		UpdateCloudProviderConfig: infrav1.UpdateCloudProviderConfig(
			src.UpdateCloudProviderConfig),
		LoadBalancerConfig: infrav1.LoadBalancerConfig{
			IPAMType:    infrav1.IPAMType(src.LoadBalancerConfig.IPAMType),
			IpPoolRef:   src.LoadBalancerConfig.IpPoolRef,
			IpPool:      infrav1.IpPool(src.LoadBalancerConfig.IpPool),
			Description: src.LoadBalancerConfig.Description,
		},
	}

	if src.LoadBalancerConfig.Listeners != nil {
		dst.LoadBalancerConfig.Listeners = make([]infrav1.Listener, len(src.LoadBalancerConfig.Listeners))
		for i, l := range src.LoadBalancerConfig.Listeners {
			dst.LoadBalancerConfig.Listeners[i] = infrav1.Listener(l)
		}
	}

	dst.VMNetworkConfig = convertVMNetworkConfigTo(src.VMNetworkConfig)

	return dst
}

func convertClusterSpecFrom(src *infrav1.HarvesterClusterSpec) HarvesterClusterSpec {
	dst := HarvesterClusterSpec{
		Server:                    src.Server,
		IdentitySecret:            SecretKey(src.IdentitySecret),
		ControlPlaneEndpoint:      src.ControlPlaneEndpoint,
		TargetNamespace:           src.TargetNamespace,
		UpdateCloudProviderConfig: UpdateCloudProviderConfig(src.UpdateCloudProviderConfig),
		LoadBalancerConfig: LoadBalancerConfig{
			IPAMType:    IPAMType(src.LoadBalancerConfig.IPAMType),
			IpPoolRef:   src.LoadBalancerConfig.IpPoolRef,
			IpPool:      IpPool(src.LoadBalancerConfig.IpPool),
			Description: src.LoadBalancerConfig.Description,
		},
	}

	if src.LoadBalancerConfig.Listeners != nil {
		dst.LoadBalancerConfig.Listeners = make([]Listener, len(src.LoadBalancerConfig.Listeners))
		for i, l := range src.LoadBalancerConfig.Listeners {
			dst.LoadBalancerConfig.Listeners[i] = Listener(l)
		}
	}

	dst.VMNetworkConfig = convertVMNetworkConfigFrom(src.VMNetworkConfig)

	return dst
}

func convertMachineSpecTo(src *HarvesterMachineSpec) infrav1.HarvesterMachineSpec {
	dst := infrav1.HarvesterMachineSpec{
		ProviderID:       src.ProviderID,
		FailureDomain:    src.FailureDomain,
		CPU:              src.CPU,
		Memory:           src.Memory,
		SSHUser:          src.SSHUser,
		SSHKeyPair:       src.SSHKeyPair,
		Networks:         src.Networks,
		NodeAffinity:     src.NodeAffinity,
		WorkloadAffinity: src.WorkloadAffinity,
	}

	if src.Volumes != nil {
		dst.Volumes = make([]infrav1.Volume, 0, len(src.Volumes))
	}

	for _, v := range src.Volumes {
		dst.Volumes = append(dst.Volumes, infrav1.Volume{
			VolumeType:   infrav1.VolumeType(v.VolumeType),
			ImageName:    v.ImageName,
			StorageClass: v.StorageClass,
			VolumeSize:   v.VolumeSize,
			BootOrder:    v.BootOrder,
		})
	}

	if src.NetworkConfig != nil {
		network := infrav1.NetworkConfig(*src.NetworkConfig)
		dst.NetworkConfig = &network
	}

	dst.VMNetworkConfig = convertVMNetworkConfigTo(src.VMNetworkConfig)

	if src.Firmware != nil {
		firmware := infrav1.Firmware(*src.Firmware)
		dst.Firmware = &firmware
	}

	if src.TPM != nil {
		tpm := infrav1.TPM(*src.TPM)
		dst.TPM = &tpm
	}

	return dst
}

func convertMachineSpecFrom(src *infrav1.HarvesterMachineSpec) HarvesterMachineSpec {
	dst := HarvesterMachineSpec{
		ProviderID:       src.ProviderID,
		FailureDomain:    src.FailureDomain,
		CPU:              src.CPU,
		Memory:           src.Memory,
		SSHUser:          src.SSHUser,
		SSHKeyPair:       src.SSHKeyPair,
		Networks:         src.Networks,
		NodeAffinity:     src.NodeAffinity,
		WorkloadAffinity: src.WorkloadAffinity,
	}

	if src.Volumes != nil {
		dst.Volumes = make([]Volume, 0, len(src.Volumes))
	}

	for _, v := range src.Volumes {
		dst.Volumes = append(dst.Volumes, Volume{
			VolumeType:   VolumeType(v.VolumeType),
			ImageName:    v.ImageName,
			StorageClass: v.StorageClass,
			VolumeSize:   v.VolumeSize,
			BootOrder:    v.BootOrder,
		})
	}

	if src.NetworkConfig != nil {
		network := NetworkConfig(*src.NetworkConfig)
		dst.NetworkConfig = &network
	}

	dst.VMNetworkConfig = convertVMNetworkConfigFrom(src.VMNetworkConfig)

	if src.Firmware != nil {
		firmware := Firmware(*src.Firmware)
		dst.Firmware = &firmware
	}

	if src.TPM != nil {
		tpm := TPM(*src.TPM)
		dst.TPM = &tpm
	}

	return dst
}

// ConvertTo converts this HarvesterCluster to the hub version.
func (src *HarvesterCluster) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*infrav1.HarvesterCluster)
	if !ok {
		return errUnexpectedHub(dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = convertClusterSpecTo(&src.Spec)
	dst.Status = infrav1.HarvesterClusterStatus{
		Ready:          src.Status.Ready,
		Conditions:     src.Status.Conditions,
		Initialization: infrav1.Initialization(src.Status.Initialization),
		FailureDomains: src.Status.FailureDomains,
	}

	return nil
}

// ConvertFrom converts the hub version to this HarvesterCluster.
//
//nolint:revive // src/dst receiver names follow the CAPI conversion convention
func (dst *HarvesterCluster) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*infrav1.HarvesterCluster)
	if !ok {
		return errUnexpectedHub(srcRaw)
	}

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = convertClusterSpecFrom(&src.Spec)
	dst.Status = HarvesterClusterStatus{
		Ready:          src.Status.Ready,
		Conditions:     src.Status.Conditions,
		Initialization: Initialization(src.Status.Initialization),
		FailureDomains: src.Status.FailureDomains,
	}

	return nil
}

// ConvertTo converts this HarvesterMachine to the hub version.
func (src *HarvesterMachine) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*infrav1.HarvesterMachine)
	if !ok {
		return errUnexpectedHub(dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = convertMachineSpecTo(&src.Spec)
	dst.Status = infrav1.HarvesterMachineStatus{
		Ready:              src.Status.Ready,
		Conditions:         src.Status.Conditions,
		Addresses:          src.Status.Addresses,
		Initialization:     infrav1.Initialization(src.Status.Initialization),
		AllocatedIPAddress: src.Status.AllocatedIPAddress,
		AllocatedPoolRef:   src.Status.AllocatedPoolRef,
		FailureDomain:      src.Status.FailureDomain,
	}

	return nil
}

// ConvertFrom converts the hub version to this HarvesterMachine.
//
//nolint:revive // src/dst receiver names follow the CAPI conversion convention
func (dst *HarvesterMachine) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*infrav1.HarvesterMachine)
	if !ok {
		return errUnexpectedHub(srcRaw)
	}

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec = convertMachineSpecFrom(&src.Spec)
	dst.Status = HarvesterMachineStatus{
		Ready:              src.Status.Ready,
		Conditions:         src.Status.Conditions,
		Addresses:          src.Status.Addresses,
		Initialization:     Initialization(src.Status.Initialization),
		AllocatedIPAddress: src.Status.AllocatedIPAddress,
		AllocatedPoolRef:   src.Status.AllocatedPoolRef,
		FailureDomain:      src.Status.FailureDomain,
	}

	return nil
}

// ConvertTo converts this HarvesterClusterTemplate to the hub version.
func (src *HarvesterClusterTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*infrav1.HarvesterClusterTemplate)
	if !ok {
		return errUnexpectedHub(dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.Template.ObjectMeta = src.Spec.Template.ObjectMeta
	dst.Spec.Template.Spec = convertClusterSpecTo(&src.Spec.Template.Spec)

	return nil
}

// ConvertFrom converts the hub version to this HarvesterClusterTemplate.
//
//nolint:revive // src/dst receiver names follow the CAPI conversion convention
func (dst *HarvesterClusterTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*infrav1.HarvesterClusterTemplate)
	if !ok {
		return errUnexpectedHub(srcRaw)
	}

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.Template.ObjectMeta = src.Spec.Template.ObjectMeta
	dst.Spec.Template.Spec = convertClusterSpecFrom(&src.Spec.Template.Spec)

	return nil
}

// ConvertTo converts this HarvesterMachineTemplate to the hub version.
func (src *HarvesterMachineTemplate) ConvertTo(dstRaw conversion.Hub) error {
	dst, ok := dstRaw.(*infrav1.HarvesterMachineTemplate)
	if !ok {
		return errUnexpectedHub(dstRaw)
	}

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.Template.ObjectMeta = src.Spec.Template.ObjectMeta
	dst.Spec.Template.Spec = convertMachineSpecTo(&src.Spec.Template.Spec)

	return nil
}

// ConvertFrom converts the hub version to this HarvesterMachineTemplate.
//
//nolint:revive // src/dst receiver names follow the CAPI conversion convention
func (dst *HarvesterMachineTemplate) ConvertFrom(srcRaw conversion.Hub) error {
	src, ok := srcRaw.(*infrav1.HarvesterMachineTemplate)
	if !ok {
		return errUnexpectedHub(srcRaw)
	}

	dst.ObjectMeta = src.ObjectMeta
	dst.Spec.Template.ObjectMeta = src.Spec.Template.ObjectMeta
	dst.Spec.Template.Spec = convertMachineSpecFrom(&src.Spec.Template.Spec)

	return nil
}

func errUnexpectedHub(obj conversion.Hub) error {
	return fmt.Errorf("unexpected hub type %T", obj)
}
