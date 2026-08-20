//go:build e2e
// +build e2e

package importgitops

import (
	. "github.com/onsi/ginkgo/v2"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/envtest/komega"

	"github.com/rancher/turtles/test/e2e"
	"github.com/rancher/turtles/test/e2e/specs"

	"github.com/harvester/cluster-api-provider-harvester/test/certification/suites"
)

// The Turtles integration spec: provisions a real cluster on Harvester from the
// ClusterClass template, waits for it to become Available, verifies the automatic
// Rancher import (cattle-cluster-agent deployed downstream, fleet-managed), and
// verifies the cluster deletes without stalling.
var _ = Describe("[Harvester] [RKE2] Create and delete CAPI cluster functionality should work with namespace auto-import", func() {
	BeforeEach(func() {
		komega.SetClient(bootstrapClusterProxy.GetClient())
		komega.SetContext(ctx)
	})

	specs.CreateUsingGitOpsSpec(ctx, func() specs.CreateUsingGitOpsSpecInput {
		return specs.CreateUsingGitOpsSpecInput{
			E2EConfig:                 e2e.LoadE2EConfig(),
			BootstrapClusterProxy:     bootstrapClusterProxy,
			ClusterTemplate:           suites.HarvesterRKE2Topology,
			ClusterName:               "caphv-gitops",
			ClusterctlBinaryPath:      "clusterctl",
			ControlPlaneMachineCount:  ptr.To(1),
			WorkerMachineCount:        ptr.To(1),
			LabelNamespace:            true,
			RancherManagedFleet:       true,
			RancherServerURL:          hostName,
			CAPIClusterCreateWaitName: "wait-rancher",
			DeleteClusterWaitName:     "wait-delete-cluster",
		}
	})
})
