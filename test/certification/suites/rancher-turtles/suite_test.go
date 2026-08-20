//go:build e2e
// +build e2e

package rancherturtles

import (
	"context"
	"strconv"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/json"
	capiframework "sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/rancher/turtles/test/e2e"
	turtlesframework "github.com/rancher/turtles/test/framework"
	"github.com/rancher/turtles/test/testenv"

	"github.com/harvester/cluster-api-provider-harvester/test/certification/suites"
)

var (
	ctx = context.Background()

	e2eConfig             *clusterctl.E2EConfig
	setupClusterResult    *testenv.SetupTestClusterResult
	bootstrapClusterProxy capiframework.ClusterProxy
)

func TestCAPHVRancherTurtlesCertification(t *testing.T) {
	RegisterFailHandler(Fail)

	ctrl.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	RunSpecs(t, "caphv-rancher-turtles-certification")
}

// SynchronizedBeforeSuite stands up a self-contained management cluster running the FULL
// targeted Rancher, and lets Rancher's system chart controller install the released
// Rancher Turtles, the CAPI core and the RKE2 providers — exactly the stack a Rancher
// user gets out of the box (verified against a real Rancher 2.14.1 install). Only the
// CAPHV CAPIProvider is applied on top.
//
// Note: the upstream Turtles e2e flow additionally deploys a Gitea and pushes a locally
// built rancher/charts tree to it — that is only needed to test UNRELEASED Turtles
// charts. For certifying released version pairings, the default chart repository that
// Rancher ships with is the representative source, so no Gitea is involved here.
var _ = SynchronizedBeforeSuite(
	func() []byte {
		e2eConfig = e2e.LoadE2EConfig()
		e2eConfig.ManagementClusterName = e2eConfig.ManagementClusterName + "-caphv-cert-a"

		By("Setting up the management cluster (kind)")
		setupClusterResult = testenv.SetupTestCluster(ctx, testenv.SetupTestClusterInput{
			E2EConfig:             e2eConfig,
			Scheme:                e2e.InitScheme(),
			CustomClusterProvider: suites.KindBootstrapCluster,
		})
		proxy := setupClusterResult.BootstrapClusterProxy

		By("Deploying cert-manager")
		testenv.DeployCertManager(ctx, testenv.DeployCertManagerInput{
			BootstrapClusterProxy: proxy,
		})

		By("Deploying the nginx ingress (isolated mode)")
		testenv.RancherDeployIngress(ctx, testenv.RancherDeployIngressInput{
			BootstrapClusterProxy:    proxy,
			CustomIngress:            e2e.NginxIngress,
			DefaultIngressClassPatch: e2e.IngressClassPatch,
		})

		By("Deploying Rancher " + e2eConfig.GetVariableOrEmpty(rancherVersionVar))
		testenv.DeployRancher(ctx, testenv.DeployRancherInput{
			BootstrapClusterProxy:   proxy,
			RancherIngressClassName: "nginx",
			RancherPatches:          [][]byte{e2e.RancherSettingPatch},
			RancherWaitInterval:     e2eConfig.GetIntervals("default", "wait-rancher"),
			ControllerWaitInterval:  e2eConfig.GetIntervals("default", "wait-controllers"),
		})

		// Rancher's system chart controller installs the released Turtles, which in turn
		// brings up the CAPI core — but NOT the RKE2 providers (verified by a real run:
		// a fresh Rancher leaves them out; they must be declared as CAPIProviders, as
		// the upstream Turtles e2e does).
		By("Waiting for the Rancher-managed Turtles + CAPI core to come up")
		for _, nn := range []struct{ name, namespace string }{
			{"rancher-turtles-controller-manager", e2e.NewRancherTurtlesNamespace},
			{"capi-controller-manager", "cattle-capi-system"},
		} {
			waitForDeploymentAvailableOn(proxy, nn.name, nn.namespace,
				e2eConfig.GetIntervals("default", "wait-rancher")...)
		}

		By("Deploying the RKE2 providers and the CAPHV " + e2eConfig.GetVariableOrEmpty(caphvVersionVar) + " CAPIProvider")
		for _, template := range [][]byte{suites.CAPIProviderRKE2Turtles, suites.CAPIProviderHarvesterTurtles} {
			Expect(turtlesframework.ApplyFromTemplate(ctx, turtlesframework.ApplyFromTemplateInput{
				Proxy:    proxy,
				Template: template,
			})).To(Succeed(), "Failed to apply CAPIProvider manifest")
		}

		By("Waiting for the RKE2 provider deployments to be available")
		for _, nn := range []struct{ name, namespace string }{
			{"rke2-bootstrap-controller-manager", "rke2-bootstrap-system"},
			{"rke2-control-plane-controller-manager", "rke2-control-plane-system"},
		} {
			waitForDeploymentAvailableOn(proxy, nn.name, nn.namespace,
				e2eConfig.GetIntervals("default", "wait-controllers")...)
		}

		data, err := json.Marshal(e2e.Setup{
			ClusterName:    setupClusterResult.ClusterName,
			KubeconfigPath: setupClusterResult.KubeconfigPath,
		})
		Expect(err).ToNot(HaveOccurred())

		return data
	},
	func(sharedData []byte) {
		setup := e2e.Setup{}
		Expect(json.Unmarshal(sharedData, &setup)).To(Succeed())

		// LoadE2EConfig also exports the config variables into this process' environment.
		e2eConfig = e2e.LoadE2EConfig()

		bootstrapClusterProxy = capiframework.NewClusterProxy(
			setup.ClusterName,
			setup.KubeconfigPath,
			e2e.InitScheme(),
		)
		Expect(bootstrapClusterProxy).ToNot(BeNil(), "cluster proxy should not be nil")
	},
)

var _ = SynchronizedAfterSuite(
	func() {},
	func() {
		if setupClusterResult == nil {
			return
		}

		skipCleanup, _ := strconv.ParseBool(e2eConfig.GetVariableOrEmpty(e2e.SkipResourceCleanupVar))
		if skipCleanup {
			return
		}

		testenv.CleanupTestCluster(ctx, testenv.CleanupTestClusterInput{
			SetupTestClusterResult: *setupClusterResult,
		})
	},
)

// Names of the e2e config variables describing the certified version pairing.
const (
	caphvVersionVar   = "CAPHV_VERSION"
	rancherVersionVar = "RANCHER_VERSION"
)

// waitForDeploymentAvailableOn waits on the given proxy (usable before the package-level
// bootstrapClusterProxy is set in the all-processes setup).
func waitForDeploymentAvailableOn(proxy capiframework.ClusterProxy, name, namespace string, intervals ...interface{}) {
	capiframework.WaitForDeploymentsAvailable(ctx, capiframework.WaitForDeploymentsAvailableInput{
		Getter: proxy.GetClient(),
		Deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		}},
	}, intervals...)
}
