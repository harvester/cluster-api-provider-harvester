package v1alpha1

import (
	"testing"

	"sigs.k8s.io/randfill"

	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"

	utilconversion "sigs.k8s.io/cluster-api/util/conversion"

	infrav1 "github.com/harvester/cluster-api-provider-harvester/api/v1beta1"
)

// fuzzFuncs pins the deprecated terminal failure fields to their zero value: they are
// deliberately dropped from v1beta1 and not preserved across conversion (the controller
// stopped writing them in v0.4.0; failures surface through the conditions).
func fuzzFuncs(_ runtimeserializer.CodecFactory) []any {
	return []any{
		func(status *HarvesterClusterStatus, c randfill.Continue) {
			c.FillNoCustom(status)
			status.FailureReason = ""  //nolint:staticcheck // deliberate: dropped in v1beta1
			status.FailureMessage = "" //nolint:staticcheck // deliberate: dropped in v1beta1
		},
		func(status *HarvesterMachineStatus, c randfill.Continue) {
			c.FillNoCustom(status)
			status.FailureReason = ""  //nolint:staticcheck // deliberate: dropped in v1beta1
			status.FailureMessage = "" //nolint:staticcheck // deliberate: dropped in v1beta1
		},
	}
}

// TestFuzzyConversion proves the v1alpha1 <-> v1beta1 conversion is lossless in both
// directions for everything v1beta1 retains.
func TestFuzzyConversion(t *testing.T) {
	t.Run("for HarvesterCluster", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &infrav1.HarvesterCluster{},
		Spoke:       &HarvesterCluster{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{fuzzFuncs},
	}))
	t.Run("for HarvesterMachine", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:         &infrav1.HarvesterMachine{},
		Spoke:       &HarvesterMachine{},
		FuzzerFuncs: []fuzzer.FuzzerFuncs{fuzzFuncs},
	}))
	t.Run("for HarvesterClusterTemplate", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &infrav1.HarvesterClusterTemplate{},
		Spoke: &HarvesterClusterTemplate{},
	}))
	t.Run("for HarvesterMachineTemplate", utilconversion.FuzzTestFunc(utilconversion.FuzzTestFuncInput{
		Hub:   &infrav1.HarvesterMachineTemplate{},
		Spoke: &HarvesterMachineTemplate{},
	}))
}
