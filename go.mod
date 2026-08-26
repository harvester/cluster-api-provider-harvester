module github.com/harvester/cluster-api-provider-harvester

go 1.26.4

require (
	github.com/containernetworking/cni v1.3.0
	github.com/containernetworking/plugins v1.9.0
	github.com/go-logr/logr v1.4.4
	github.com/harvester/harvester v1.3.2
	github.com/harvester/harvester-load-balancer v1.8.1
	github.com/k8snetworkplumbingwg/network-attachment-definition-client v1.7.7
	github.com/kubernetes-csi/external-snapshotter/client/v4 v4.2.0
	github.com/longhorn/longhorn-manager v1.13.0-dev-20260809
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/ginkgo/v2 v2.32.0
	github.com/onsi/gomega v1.42.1
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.93.0
	github.com/prometheus/client_golang v1.24.1
	github.com/rancher/rancher/pkg/apis v0.0.0
	github.com/rancher/system-upgrade-controller/pkg/apis v0.0.0-20250701000733-99a03a0d61aa
	github.com/zach-klippenstein/goregen v0.0.0-20160303162051-795b5e3961ea
	gopkg.in/yaml.v2 v2.4.0
	k8s.io/api v0.36.3
	k8s.io/apimachinery v0.36.3
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/utils v0.0.0-20260707023825-cf1189d6abe3
	kubevirt.io/api v1.9.0
	sigs.k8s.io/cluster-api v1.13.4
	sigs.k8s.io/controller-runtime v0.24.1
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730
	sigs.k8s.io/randfill v1.0.0
	sigs.k8s.io/yaml v1.6.0
)

require (
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/pkg/errors v0.9.1
	go.opentelemetry.io/otel v1.43.0 // indirect
	go.opentelemetry.io/otel/trace v1.43.0 // indirect
)

require (
	github.com/Masterminds/semver/v3 v3.4.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/coreos/go-iptables v0.8.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/emicklei/go-restful/v3 v3.13.0 // indirect
	github.com/evanphx/json-patch v5.9.11+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fxamacker/cbor/v2 v2.9.2 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/go-logr/zapr v1.3.0 // indirect
	github.com/go-openapi/jsonpointer v0.22.4 // indirect
	github.com/go-openapi/jsonreference v0.21.4 // indirect
	github.com/go-openapi/swag v0.25.4 // indirect
	github.com/go-openapi/swag/cmdutils v0.25.4 // indirect
	github.com/go-openapi/swag/conv v0.25.4 // indirect
	github.com/go-openapi/swag/fileutils v0.25.4 // indirect
	github.com/go-openapi/swag/jsonname v0.25.4 // indirect
	github.com/go-openapi/swag/jsonutils v0.25.4 // indirect
	github.com/go-openapi/swag/loading v0.25.4 // indirect
	github.com/go-openapi/swag/mangling v0.25.4 // indirect
	github.com/go-openapi/swag/netutils v0.25.4 // indirect
	github.com/go-openapi/swag/stringutils v0.25.4 // indirect
	github.com/go-openapi/swag/typeutils v0.25.4 // indirect
	github.com/go-openapi/swag/yamlutils v0.25.4 // indirect
	github.com/gobuffalo/flect v1.0.3 // indirect
	github.com/google/gnostic-models v0.7.1 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/gxui v0.0.0-20151028112939-f85e0a97b3a4 // indirect
	github.com/google/pprof v0.0.0-20260402051712-545e8a4df936 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/kubereboot/kured v1.13.1 // indirect
	github.com/moby/spdystream v0.5.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/openshift/custom-resource-status v1.1.2 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.70.1 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	github.com/rancher/aks-operator v1.12.0 // indirect
	github.com/rancher/eks-operator v1.12.0 // indirect
	github.com/rancher/fleet/pkg/apis v0.13.0 // indirect
	github.com/rancher/gke-operator v1.12.0 // indirect
	github.com/rancher/lasso v0.2.9 // indirect
	github.com/rancher/norman v0.7.0 // indirect
	github.com/rancher/rke v1.8.0-rc.4 // indirect
	github.com/rancher/wrangler v1.1.2 // indirect
	github.com/rancher/wrangler/v3 v3.7.0 // indirect
	github.com/safchain/ethtool v0.6.2 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/smartystreets/goconvey v1.8.1 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/vishvananda/netlink v1.3.1 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.uber.org/zap v1.27.1 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/term v0.45.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools v0.47.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.5.0 // indirect
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af // indirect
	gopkg.in/evanphx/json-patch.v4 v4.13.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	k8s.io/apiextensions-apiserver v0.36.3 // indirect
	k8s.io/apiserver v0.36.3 // indirect
	k8s.io/component-base v0.36.3 // indirect
	k8s.io/klog/v2 v2.140.0 // indirect
	k8s.io/kube-openapi v0.0.0-20260603220949-865597e52e25 // indirect
	k8s.io/kubernetes v1.36.3 // indirect
	kubevirt.io/containerized-data-importer-api v1.64.0 // indirect
	kubevirt.io/controller-lifecycle-operator-sdk/api v0.2.4 // indirect
	sigs.k8s.io/knftables v0.0.21 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.4.2 // indirect
)

replace (
	github.com/rancher/rancher/pkg/apis => github.com/rancher/rancher/pkg/apis v0.0.0-20240919204204-3da2ae0cabd1
	github.com/rancher/rancher/pkg/client => github.com/rancher/rancher/pkg/client v0.0.0-20240919204204-3da2ae0cabd1
)

// kube-openapi pin aligned with k8s.io/apimachinery v0.34.8 expectation
// (uses structured-merge-diff/v6 to match the newer apimachinery internals).
replace k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20250910181357-589584f1c912

replace github.com/harvester/harvester => github.com/harvester/harvester v1.4.0-dev-20240719

// Pin all k8s.io modules to v0.34.x — required since transitive deps
// (kubevirt, harvester, longhorn) pull k8s.io/api >= v0.36 which requires
// Keeping v0.35 aligned with controller-runtime v0.23.3 and CAPI v1.13.4
// (both shipped against k8s.io v0.35); some transitive deps pull newer.
replace (
	k8s.io/api => k8s.io/api v0.35.4
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.35.4
	k8s.io/apimachinery => k8s.io/apimachinery v0.35.4
	k8s.io/apiserver => k8s.io/apiserver v0.35.4
	k8s.io/client-go => k8s.io/client-go v0.35.4
	k8s.io/component-base => k8s.io/component-base v0.35.4
	k8s.io/kubernetes => k8s.io/kubernetes v1.35.4
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.23.3
)
