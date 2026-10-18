package v1alpha1

import (
	"encoding/json"
	"errors"
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

// templateMetadata returns spec.template.metadata of a serialized template, and whether the
// field is present at all.
func templateMetadata(t *testing.T, obj any) (map[string]any, bool) {
	t.Helper()

	raw, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}

	var decoded struct {
		Spec struct {
			Template map[string]json.RawMessage `json:"template"`
		} `json:"spec"`
	}

	err = json.Unmarshal(raw, &decoded)
	if err != nil {
		t.Fatal(err)
	}

	metadata, found := decoded.Spec.Template["metadata"]
	if !found {
		return nil, false
	}

	value := map[string]any{}

	err = json.Unmarshal(metadata, &value)
	if err != nil {
		t.Fatal(err)
	}

	return value, true
}

// TestTemplateConversionOmitsEmptyTemplateMetadata guards the objects the conversion
// webhook returns for templates. spec.template.metadata is a CAPI ObjectMeta, whose schema
// requires at least one property: serialized as an empty object, a template created or read
// through v1alpha1 could no longer be cloned by the CAPI topology controller ("should have
// at least 1 properties"). An empty metadata must be omitted, a set one preserved.
func TestTemplateConversionOmitsEmptyTemplateMetadata(t *testing.T) {
	// Each case converts a v1alpha1 template carrying the given template labels to the hub
	// (the stored version) and back (what the webhook serves through v1alpha1).
	cases := map[string]func(labels map[string]string) (stored, served any, err error){
		"HarvesterMachineTemplate": func(labels map[string]string) (any, any, error) {
			spoke := &HarvesterMachineTemplate{}
			spoke.Spec.Template.ObjectMeta.Labels = labels
			spoke.Spec.Template.Spec.CPU = 2

			hub, back := &infrav1.HarvesterMachineTemplate{}, &HarvesterMachineTemplate{}

			return hub, back, errors.Join(spoke.ConvertTo(hub), back.ConvertFrom(hub))
		},
		"HarvesterClusterTemplate": func(labels map[string]string) (any, any, error) {
			spoke := &HarvesterClusterTemplate{}
			spoke.Spec.Template.ObjectMeta.Labels = labels
			spoke.Spec.Template.Spec.TargetNamespace = "default"

			hub, back := &infrav1.HarvesterClusterTemplate{}, &HarvesterClusterTemplate{}

			return hub, back, errors.Join(spoke.ConvertTo(hub), back.ConvertFrom(hub))
		},
	}

	for kind, roundTrip := range cases {
		t.Run(kind+" without template metadata", func(t *testing.T) {
			stored, served, err := roundTrip(nil)
			if err != nil {
				t.Fatal(err)
			}

			for name, obj := range map[string]any{"v1beta1 (stored)": stored, "v1alpha1 (served)": served} {
				if metadata, found := templateMetadata(t, obj); found {
					t.Errorf("%s: empty spec.template.metadata serialized as %v, want it omitted", name, metadata)
				}
			}
		})

		t.Run(kind+" with template labels", func(t *testing.T) {
			stored, served, err := roundTrip(map[string]string{"example.com/role": "worker"})
			if err != nil {
				t.Fatal(err)
			}

			for name, obj := range map[string]any{"v1beta1 (stored)": stored, "v1alpha1 (served)": served} {
				if metadata, found := templateMetadata(t, obj); !found || metadata["labels"] == nil {
					t.Errorf("%s: spec.template.metadata lost its labels: %v", name, metadata)
				}
			}
		})
	}
}

// controlPlaneEndpoint returns spec.controlPlaneEndpoint (or spec.template.spec.controlPlaneEndpoint
// for a template) of a serialized object, and whether the field is present at all.
func controlPlaneEndpoint(t *testing.T, obj any) (map[string]any, bool) {
	t.Helper()

	raw, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}

	var decoded struct {
		Spec struct {
			ControlPlaneEndpoint map[string]any `json:"controlPlaneEndpoint"`
			Template             struct {
				Spec struct {
					ControlPlaneEndpoint map[string]any `json:"controlPlaneEndpoint"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"spec"`
	}

	err = json.Unmarshal(raw, &decoded)
	if err != nil {
		t.Fatal(err)
	}

	if decoded.Spec.ControlPlaneEndpoint != nil {
		return decoded.Spec.ControlPlaneEndpoint, true
	}

	return decoded.Spec.Template.Spec.ControlPlaneEndpoint, decoded.Spec.Template.Spec.ControlPlaneEndpoint != nil
}

// TestConversionOmitsEmptyControlPlaneEndpoint guards the objects the conversion webhook
// returns. spec.controlPlaneEndpoint is a CAPI APIEndpoint, whose schema requires at least one
// property: serialized as an empty object, a cluster template without an endpoint created or
// read through v1alpha1 could no longer be cloned by the CAPI topology controller ("should
// have at least 1 properties"). An empty endpoint must be omitted, a set one preserved.
func TestConversionOmitsEmptyControlPlaneEndpoint(t *testing.T) {
	// Each case converts a v1alpha1 object with the given endpoint host to the hub (the
	// stored version) and back (what the webhook serves through v1alpha1).
	cases := map[string]func(host string) (stored, served any, err error){
		"HarvesterCluster": func(host string) (any, any, error) {
			spoke := &HarvesterCluster{}
			spoke.Spec.ControlPlaneEndpoint.Host = host

			hub, back := &infrav1.HarvesterCluster{}, &HarvesterCluster{}

			return hub, back, errors.Join(spoke.ConvertTo(hub), back.ConvertFrom(hub))
		},
		"HarvesterClusterTemplate": func(host string) (any, any, error) {
			spoke := &HarvesterClusterTemplate{}
			spoke.Spec.Template.Spec.ControlPlaneEndpoint.Host = host

			hub, back := &infrav1.HarvesterClusterTemplate{}, &HarvesterClusterTemplate{}

			return hub, back, errors.Join(spoke.ConvertTo(hub), back.ConvertFrom(hub))
		},
	}

	for kind, roundTrip := range cases {
		t.Run(kind+" without endpoint", func(t *testing.T) {
			stored, served, err := roundTrip("")
			if err != nil {
				t.Fatal(err)
			}

			for name, obj := range map[string]any{"v1beta1 (stored)": stored, "v1alpha1 (served)": served} {
				if endpoint, found := controlPlaneEndpoint(t, obj); found {
					t.Errorf("%s: empty controlPlaneEndpoint serialized as %v, want it omitted", name, endpoint)
				}
			}
		})

		t.Run(kind+" with an endpoint", func(t *testing.T) {
			stored, served, err := roundTrip("172.16.3.46")
			if err != nil {
				t.Fatal(err)
			}

			for name, obj := range map[string]any{"v1beta1 (stored)": stored, "v1alpha1 (served)": served} {
				if endpoint, found := controlPlaneEndpoint(t, obj); !found || endpoint["host"] != "172.16.3.46" {
					t.Errorf("%s: controlPlaneEndpoint lost its host: %v", name, endpoint)
				}
			}
		})
	}
}
