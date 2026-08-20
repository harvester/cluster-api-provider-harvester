//go:build e2e
// +build e2e

package rancherturtles

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	turtlesv1 "github.com/rancher/turtles/api/v1alpha1"

	"github.com/harvester/cluster-api-provider-harvester/test/certification/suites"
)

// This suite certifies the CAPHV release under the FULL targeted Rancher + Turtles stack:
// Rancher's system chart controller installs the released Turtles/CAPI core/RKE2
// providers, and CAPHV is installed on top as a Turtles CAPIProvider. It deliberately
// does NOT provision a workload cluster on Harvester (that is the separate on-demand e2e
// suite) — no Harvester endpoint is required.
var _ = Describe("[Certification] CAPHV under the targeted Rancher + Turtles stack",
	Label(suites.FullTestLabel), func() {
		It("installs the CAPHV CAPIProvider Ready with the targeted version", func() {
			cl := bootstrapClusterProxy.GetClient()
			intervals := e2eConfig.GetIntervals("default", "wait-controllers")

			provider := &turtlesv1.CAPIProvider{}
			By("Waiting for the harvester CAPIProvider to be Ready")
			Eventually(func(g Gomega) {
				g.Expect(cl.Get(ctx, types.NamespacedName{Name: "harvester", Namespace: "caphv-system"}, provider)).
					To(Succeed())
				g.Expect(meta.IsStatusConditionTrue(provider.Status.Conditions, "Ready")).To(BeTrue(),
					"caphv-system/harvester should have condition Ready=True, conditions: %v", provider.Status.Conditions)
			}, intervals...).Should(Succeed(), "the harvester CAPIProvider never became Ready")

			installed := provider.Status.InstalledVersion
			Expect(installed).ToNot(BeNil(), "the harvester CAPIProvider must report an installed version")
			Expect(*installed).To(Equal(e2eConfig.GetVariableOrEmpty(caphvVersionVar)),
				"the harvester CAPIProvider must install the targeted version")
			GinkgoWriter.Printf("CAPIProvider caphv-system/harvester installed version: %s\n", *installed)
		})

		It("runs the caphv-controller-manager healthy under the Rancher-managed CAPI core", func() {
			By("Waiting for the caphv-controller-manager deployment to be Available")
			waitForDeploymentAvailableOn(bootstrapClusterProxy, "caphv-controller-manager", "caphv-system",
				e2eConfig.GetIntervals("default", "wait-controllers")...)
		})

		It("registers the CAPHV CRDs with a CAPI contract label the core accepts", func() {
			cl := bootstrapClusterProxy.GetClient()

			crd := &apiextensionsv1.CustomResourceDefinition{}
			Expect(cl.Get(ctx, types.NamespacedName{Name: "harvesterclusters.infrastructure.cluster.x-k8s.io"}, crd)).
				To(Succeed(), "HarvesterCluster CRD must be registered")

			// The CRD must advertise the CAPI contract it implements via a cluster.x-k8s.io/<version>
			// label, otherwise the core controllers will not drive it.
			var contracts []string
			for k := range crd.Labels {
				if strings.HasPrefix(k, "cluster.x-k8s.io/v1") {
					contracts = append(contracts, k)
				}
			}
			Expect(contracts).ToNot(BeEmpty(),
				"HarvesterCluster CRD must carry a cluster.x-k8s.io/<contract> label")
			GinkgoWriter.Printf("CAPHV HarvesterCluster CRD contract labels: %v\n", contracts)
		})

		It("records the certified Rancher-managed ecosystem versions", func() {
			cl := bootstrapClusterProxy.GetClient()

			// The Turtles system chart version installed by Rancher.
			setting := &unstructured.Unstructured{}
			setting.SetGroupVersionKind(schema.GroupVersionKind{
				Group: "management.cattle.io", Version: "v3", Kind: "Setting",
			})
			if err := cl.Get(ctx, types.NamespacedName{Name: "rancher-turtles-version"}, setting); err == nil {
				GinkgoWriter.Printf("Rancher setting rancher-turtles-version: %v\n", setting.Object["value"])
			} else {
				GinkgoWriter.Printf("rancher-turtles-version setting not readable: %v\n", err)
			}

			// The CAPI core version the Turtles system chart manages.
			core := &turtlesv1.CAPIProvider{}
			Expect(cl.Get(ctx, types.NamespacedName{Name: "cluster-api", Namespace: "cattle-capi-system"}, core)).
				To(Succeed(), "the Rancher-managed CAPI core CAPIProvider must exist")
			Expect(meta.IsStatusConditionTrue(core.Status.Conditions, "Ready")).To(BeTrue(),
				"the Rancher-managed CAPI core must be Ready, conditions: %v", core.Status.Conditions)
			if core.Status.InstalledVersion != nil {
				GinkgoWriter.Printf("Rancher-managed CAPI core installed version: %s\n", *core.Status.InstalledVersion)
			}
		})
	})
