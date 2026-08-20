//go:build e2e
// +build e2e

package versionpairing

import (
	"context"
	"strconv"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/json"
	operatorv1 "sigs.k8s.io/cluster-api-operator/api/v1alpha2"
	opframework "sigs.k8s.io/cluster-api-operator/test/framework"
	capiframework "sigs.k8s.io/cluster-api/test/framework"
	"sigs.k8s.io/cluster-api/test/framework/clusterctl"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

func TestCAPHVCertification(t *testing.T) {
	RegisterFailHandler(Fail)

	ctrl.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	RunSpecs(t, "caphv-version-pairing-certification")
}

// SynchronizedBeforeSuite stands up a self-contained management cluster and installs the
// ecosystem the CAPHV release under test is certified against, mirroring the validated
// recipe in hack/tier-c-smoke.sh (keep them in sync):
// kind -> cert-manager -> cluster-api-operator -> CAPI core (wait Ready) -> RKE2 + CAPHV
// providers. No Rancher, no Turtles and no Harvester endpoint are required: Rancher Turtles
// 0.26+ cannot run without a full Rancher (its controller watches management.cattle.io CRDs),
// so this tier certifies the deeper CAPHV<->CAPI contract through the Rancher-independent
// cluster-api-operator instead.
var _ = SynchronizedBeforeSuite(
	func() []byte {
		e2eConfig = e2e.LoadE2EConfig()
		e2eConfig.ManagementClusterName = e2eConfig.ManagementClusterName + "-caphv-cert"

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

		By("Installing cluster-api-operator " + e2eConfig.GetVariableOrEmpty(capiOperatorVersionVar))
		deployCAPIOperator(e2eConfig, proxy)

		By("Deploying the CAPI core provider " + e2eConfig.GetVariableOrEmpty(capiVersionVar))
		applyProviderTemplate(proxy, suites.CoreProviderCAPI)

		// The recipe waits for the core provider to be fully Ready before applying the
		// dependent providers, so their controllers never start without the CAPI CRDs.
		By("Waiting for the CAPI core provider to be Ready")
		waitForProviderReady(proxy.GetClient(), "cluster-api", "capi-system",
			&operatorv1.CoreProvider{}, e2eConfig.GetIntervals("default", "wait-controllers")...)

		By("Deploying the RKE2 providers and the CAPHV " + e2eConfig.GetVariableOrEmpty(caphvVersionVar) + " infrastructure provider")
		applyProviderTemplate(proxy, suites.RKE2Providers)
		applyProviderTemplate(proxy, suites.InfrastructureProviderHarvester)

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
	caphvVersionVar         = "CAPHV_VERSION"
	capiVersionVar          = "CAPI_VERSION"
	capiOperatorVersionVar  = "CAPI_OPERATOR_VERSION"
	capiOperatorRepoNameVar = "CAPI_OPERATOR_REPO_NAME"
	capiOperatorURLVar      = "CAPI_OPERATOR_URL"
	capiOperatorPathVar     = "CAPI_OPERATOR_PATH"
)

// deployCAPIOperator installs the cluster-api-operator helm chart. The chart is
// schema-validated and takes NO value overrides (it requires cert-manager to be
// pre-installed and does not bundle it).
func deployCAPIOperator(config *clusterctl.E2EConfig, proxy capiframework.ClusterProxy) {
	helmBinaryPath := config.GetVariableOrEmpty(e2e.HelmBinaryPathVar)

	repoAdd := &opframework.HelmChart{
		BinaryPath:      helmBinaryPath,
		Name:            config.GetVariableOrEmpty(capiOperatorRepoNameVar),
		Path:            config.GetVariableOrEmpty(capiOperatorURLVar),
		Commands:        opframework.Commands(opframework.Repo, opframework.Add),
		AdditionalFlags: opframework.Flags("--force-update"),
		Kubeconfig:      proxy.GetKubeconfigPath(),
	}
	_, err := repoAdd.Run(nil)
	Expect(err).ToNot(HaveOccurred(), "Failed to add the cluster-api-operator chart repo")

	install := &opframework.HelmChart{
		BinaryPath: helmBinaryPath,
		Name:       "capi-operator",
		Path:       config.GetVariableOrEmpty(capiOperatorPathVar),
		Kubeconfig: proxy.GetKubeconfigPath(),
		Wait:       true,
		AdditionalFlags: opframework.Flags(
			"--namespace", "capi-operator-system",
			"--create-namespace",
			"--version", config.GetVariableOrEmpty(capiOperatorVersionVar),
			"--timeout", "5m",
		),
	}
	_, err = install.Run(nil)
	Expect(err).ToNot(HaveOccurred(), "Failed to install the cluster-api-operator chart")
}

// applyProviderTemplate renders an envsubst provider manifest (${CAPI_VERSION},
// ${CAPHV_VERSION}, ${CAPHV_COMPONENTS_URL} come from the e2e config variables
// exported to the environment) and applies it to the management cluster.
// The apply retries: the operator chart install waits for the Deployment to be
// Available, but its admission webhook endpoint can lag behind by a few
// seconds (connection refused), especially on slow CI runners.
func applyProviderTemplate(proxy capiframework.ClusterProxy, template []byte) {
	Eventually(func() error {
		return turtlesframework.ApplyFromTemplate(ctx, turtlesframework.ApplyFromTemplateInput{
			Proxy:    proxy,
			Template: template,
		})
	}, "2m", "10s").Should(Succeed(), "Failed to apply provider manifest")
}

// waitForProviderReady blocks until the given cluster-api-operator provider reports
// condition Ready=True (the operator has fetched, customized and installed it).
func waitForProviderReady(cl client.Client, name, namespace string, provider operatorv1.GenericProvider, intervals ...interface{}) {
	Eventually(func(g Gomega) {
		g.Expect(cl.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, provider)).To(Succeed())
		g.Expect(meta.IsStatusConditionTrue(provider.GetConditions(), "Ready")).To(BeTrue(),
			"%s/%s should have condition Ready=True, conditions: %v", namespace, name, provider.GetConditions())
	}, intervals...).Should(Succeed(), "provider %s/%s never became Ready", namespace, name)
}

// waitForDeploymentAvailable is a thin wrapper over the CAPI framework deployment wait.
func waitForDeploymentAvailable(name, namespace string, intervals ...interface{}) {
	capiframework.WaitForDeploymentsAvailable(ctx, capiframework.WaitForDeploymentsAvailableInput{
		Getter: bootstrapClusterProxy.GetClient(),
		Deployment: &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		}},
	}, intervals...)
}
