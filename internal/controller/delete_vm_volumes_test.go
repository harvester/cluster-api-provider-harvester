package controller

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubevirtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	infrav1 "github.com/harvester/cluster-api-provider-harvester/api/v1beta1"
	hvfake "github.com/harvester/cluster-api-provider-harvester/pkg/clientset/versioned/fake"
)

// Harvester creates the PVCs of a VM from its volumeClaimTemplates annotation without
// owner reference: they are only deleted with the VM when they are listed in the
// removedPersistentVolumeClaims annotation before the VM is deleted.
var _ = Describe("Deleting the volumes of a machine VM", func() {
	pvcVolume := func(name, claim string, hotpluggable bool) kubevirtv1.Volume {
		return kubevirtv1.Volume{
			Name: name,
			VolumeSource: kubevirtv1.VolumeSource{
				PersistentVolumeClaim: &kubevirtv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{ClaimName: claim},
					Hotpluggable:                      hotpluggable,
				},
			},
		}
	}

	deleteScope := func(hvClient *hvfake.Clientset) Scope {
		logger := log.FromContext(context.TODO())

		return Scope{
			Ctx: context.TODO(),
			HarvesterMachine: &infrav1.HarvesterMachine{
				ObjectMeta: metav1.ObjectMeta{Name: "cp-0", Namespace: "test-ns", Finalizers: []string{infrav1.MachineFinalizer}},
			},
			Cluster:          &clusterv1.Cluster{ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "test-ns"}},
			HarvesterCluster: &infrav1.HarvesterCluster{Spec: infrav1.HarvesterClusterSpec{TargetNamespace: "default"}},
			HarvesterClient:  hvClient,
			Logger:           &logger,
		}
	}

	It("lists the VM PVCs for removal before deleting the VM in the foreground", func() {
		vm := &kubevirtv1.VirtualMachine{
			ObjectMeta: metav1.ObjectMeta{Name: "cp-0", Namespace: "default", Annotations: map[string]string{}},
			Spec: kubevirtv1.VirtualMachineSpec{Template: &kubevirtv1.VirtualMachineInstanceTemplateSpec{
				Spec: kubevirtv1.VirtualMachineInstanceSpec{Volumes: []kubevirtv1.Volume{
					pvcVolume("disk-0", "cp-0-disk-0-abcde", false),
					pvcVolume("disk-1", "cp-0-disk-1-fghij", false),
					pvcVolume("hotplug", "shared-data", true),
					{Name: "cloudinitdisk", VolumeSource: kubevirtv1.VolumeSource{
						CloudInitNoCloud: &kubevirtv1.CloudInitNoCloudSource{},
					}},
				}},
			}},
		}
		hvClient := hvfake.NewSimpleClientset(vm)

		// Keep the VM present after the delete call, as KubeVirt does while the VMI stops.
		var deleteOptions metav1.DeleteOptions

		hvClient.PrependReactor("delete", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
			deleteAction, ok := action.(k8stesting.DeleteActionImpl)
			Expect(ok).To(BeTrue())

			deleteOptions = deleteAction.DeleteOptions

			return true, nil, nil
		})

		r := &HarvesterMachineReconciler{}
		result, err := r.ReconcileDelete(deleteScope(hvClient))
		Expect(err).ToNot(HaveOccurred())
		Expect(result.RequeueAfter).To(BeNumerically(">", 0))

		annotated, err := hvClient.KubevirtV1().VirtualMachines("default").Get(context.TODO(), "cp-0", metav1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())
		Expect(annotated.Annotations).To(HaveKeyWithValue(vmAnnotationRemovedPVCs, "cp-0-disk-0-abcde,cp-0-disk-1-fghij"),
			"hotplugged volumes are not part of the machine and stay")

		Expect(deleteOptions.PropagationPolicy).ToNot(BeNil())
		Expect(*deleteOptions.PropagationPolicy).To(Equal(metav1.DeletePropagationForeground))

		// Harvester reads the annotation when the VM is deleted: it must be set first.
		verbs := []string{}

		for _, action := range hvClient.Actions() {
			if action.GetResource().Resource == "virtualmachines" && (action.GetVerb() == "patch" || action.GetVerb() == "delete") {
				verbs = append(verbs, action.GetVerb())
			}
		}

		Expect(verbs).To(Equal([]string{"patch", "delete"}))
	})

	It("keeps the finalizer when a leftover PVC of a deleted VM cannot be deleted", func() {
		pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: "cp-0-disk-0-abcde", Namespace: "default"}}
		hvClient := hvfake.NewSimpleClientset(pvc)
		hvClient.PrependReactor("delete", "persistentvolumeclaims", func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("admission webhook denied the request")
		})

		scope := deleteScope(hvClient)

		r := &HarvesterMachineReconciler{}
		_, err := r.ReconcileDelete(scope)
		Expect(err).To(MatchError(ContainSubstring("cp-0-disk-0-abcde")))
		Expect(scope.HarvesterMachine.Finalizers).To(ContainElement(infrav1.MachineFinalizer))
	})
})
