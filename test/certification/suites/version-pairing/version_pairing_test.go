//go:build e2e
// +build e2e

package versionpairing

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/types"
	operatorv1 "sigs.k8s.io/cluster-api-operator/api/v1alpha2"

	"github.com/harvester/cluster-api-provider-harvester/test/certification/suites"
)

// This suite certifies the *version pairing*: that the CAPHV release under test installs
// cleanly through cluster-api-operator and is compatible with the targeted CAPI core and
// RKE2 providers. It deliberately does NOT provision a workload cluster on Harvester (that
// is the separate on-demand e2e suite) — no Rancher and no Harvester endpoint are required,
// so it runs on a standard CI runner.
var _ = Describe("[Certification] CAPHV version compatibility with the targeted CAPI ecosystem",
	Label(suites.ShortTestLabel), func() {
		It("installs all four providers Ready, with the targeted versions", func() {
			cl := bootstrapClusterProxy.GetClient()
			intervals := e2eConfig.GetIntervals("default", "wait-controllers")

			for _, p := range []struct {
				name        string
				namespace   string
				provider    operatorv1.GenericProvider
				wantVersion string
			}{
				{"cluster-api", "capi-system", &operatorv1.CoreProvider{}, e2eConfig.GetVariableOrEmpty(capiVersionVar)},
				{"rke2", "rke2-bootstrap-system", &operatorv1.BootstrapProvider{}, ""},
				{"rke2", "rke2-control-plane-system", &operatorv1.ControlPlaneProvider{}, ""},
				{"harvester", "caphv-system", &operatorv1.InfrastructureProvider{}, e2eConfig.GetVariableOrEmpty(caphvVersionVar)},
			} {
				By("Waiting for provider " + p.namespace + "/" + p.name + " to be Ready")
				waitForProviderReady(cl, p.name, p.namespace, p.provider, intervals...)

				installed := p.provider.GetStatus().InstalledVersion
				Expect(installed).ToNot(BeNil(), "provider %s/%s must report an installed version", p.namespace, p.name)
				GinkgoWriter.Printf("provider %s/%s installed version: %s\n", p.namespace, p.name, *installed)

				if p.wantVersion != "" {
					Expect(*installed).To(Equal(p.wantVersion),
						"provider %s/%s must install the targeted version", p.namespace, p.name)
				}
			}
		})

		It("runs the caphv-controller-manager healthy under the CAPI core", func() {
			By("Waiting for the caphv-controller-manager deployment to be Available")
			waitForDeploymentAvailable("caphv-controller-manager", "caphv-system",
				e2eConfig.GetIntervals("default", "wait-controllers")...)
		})

		It("registers the CAPHV CRDs with a CAPI contract label the core accepts", func() {
			cl := bootstrapClusterProxy.GetClient()

			crd := &apiextensionsv1.CustomResourceDefinition{}
			Expect(cl.Get(ctx, types.NamespacedName{Name: "harvesterclusters.infrastructure.cluster.x-k8s.io"}, crd)).
				To(Succeed(), "HarvesterCluster CRD must be registered")

			// The CRD must advertise the CAPI contract it implements via a cluster.x-k8s.io/<version>
			// label, otherwise the core controllers will not drive it. We assert the presence of at
			// least one such label rather than a fixed version, so the check stays valid across
			// releases (v0.3.1 is labelled v1beta1; newer builds add v1beta2).
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
	})
