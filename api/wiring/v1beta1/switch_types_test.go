package v1beta1_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func withName[T kclient.Object](name string, obj T) T {
	obj.SetName(name)
	obj.SetNamespace(kmetav1.NamespaceDefault)

	return obj
}

func swb(name string, f ...func(sw *wiringapi.Switch)) *wiringapi.Switch {
	sw := withName(name, &wiringapi.Switch{
		Spec: wiringapi.SwitchSpec{
			Profile:    switchprofile.VS.Name,
			Role:       wiringapi.SwitchRoleMixedLeaf,
			ASN:        65101,
			IP:         "172.30.0.9/21",
			VTEPIP:     "172.30.12.0/32",
			ProtocolIP: "172.30.8.2/32",
		},
	})

	for _, fn := range f {
		fn(sw)
	}

	return sw
}

func withObjs(base []kclient.Object, objs ...kclient.Object) []kclient.Object {
	return append(slices.Clone(base), objs...)
}

/*
 * Some notes:
 * - it's important to clone a slice when extending base with extra objects to avoid mutating base slice
 * - we aren't adding obj to validate into init objects as at a time of validate on create obj isn't in apiserver yet
 * - different FabricConfigs could be used, but the "default" one from the fabricator should be used by default
 * - that's optional (simple version is to use bool error just to check if error happened), but more advanced is using
 *   assert.ErrorIs allows to not only check presence of error, but also its type, but it requires to always use a
 *   custom error type to ensure the error is of the expected type; in order to do that one of the following options
 *   could be used:
 *   - if need to create an error, use `fmt.Errorf("error message: %w", ErrInvalidSwitch)`
 *   - if wrapping existing error, use `fmt.Errorf("error message: %w", errors.Join(err, ErrInvalidSwitch))`
 */

func TestSwitchValidate(t *testing.T) {
	base := []kclient.Object{
		withName("group-1", &wiringapi.SwitchGroup{}),
		withName("default", &wiringapi.VLANNamespace{
			Spec: wiringapi.VLANNamespaceSpec{
				Ranges: []meta.VLANRange{{From: 1000, To: 2999}},
			},
		}),
	}

	tests := []struct {
		name string
		sw   wiringapi.Switch
		objs []kclient.Object
		err  error
	}{
		{
			name: "test-swgr-present",
			sw: *swb("test", func(sw *wiringapi.Switch) {
				sw.Spec.Groups = []string{"group-1"}
			}),
			objs: base,
		},
		{
			name: "test-swgr-not-present",
			sw: *swb("test", func(sw *wiringapi.Switch) {
				sw.Spec.Groups = []string{"group-1", "group-2"}
			}),
			objs: base,
			err:  wiringapi.ErrInvalidSwitch,
		},
		{
			name: "test-overlap-vlanns",
			sw: *swb("test", func(sw *wiringapi.Switch) {
				sw.Spec.VLANNamespaces = []string{"default", "test"}
			}),
			objs: withObjs(base, withName("test", &wiringapi.VLANNamespace{
				Spec: wiringapi.VLANNamespaceSpec{
					Ranges: []meta.VLANRange{{From: 999, To: 1042}}, // overlaps with default
				},
			})),
			err: wiringapi.ErrInvalidSwitch,
		},
	}

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme), "should add wiringapi to scheme")

	// some defaults from fabricator for testing
	fabricCfg := &meta.FabricConfig{
		BaseVPCCommunity:    "50000:0",
		ControlVIP:          "172.30.0.1/32",
		DefaultMaxPathsEBGP: 64,
		ESLAGESIPrefix:      "00:f2:00:00:",
		ESLAGMACBase:        "f2:00:00:00:00:00",
		FabricMode:          meta.FabricModeSpineLeaf,
		FabricMTU:           9100,
		FabricSubnet:        "172.30.128.0/17",
		GatewayASN:          65534,
		MCLAGSessionSubnet:  "172.30.95.0/31",
		ProtocolSubnet:      "172.30.8.0/22",
		ReservedSubnets: []string{
			"172.30.0.0/21",
			"172.30.128.0/17",
			"172.30.8.0/22",
			"172.30.12.0/22",
			"172.30.96.0/19",
			"172.30.95.0/31",
		},
		ServerFacingMTUOffset: 64,
		VPCIRBVLANRanges: []meta.VLANRange{
			{From: 3000, To: 3999},
		},
		VPCLoopbackSubnet: "172.30.96.0/19",
		VPCPeeringVLANRanges: []meta.VLANRange{
			{From: 100, To: 3999},
		},
		VTEPSubnet: "172.30.12.0/22",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()

			kube := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.objs...).
				Build()

			profiles := switchprofile.NewDefaultSwitchProfiles()
			require.NoError(t, profiles.RegisterAll(ctx, kube, fabricCfg), "should register all profiles")
			require.NoError(t, profiles.Enforce(ctx, kube, fabricCfg, false), "should enforce all profiles")
			require.True(t, profiles.IsInitialized(), "default profiles should be initialized")

			tt.sw.Default()
			_, actual := tt.sw.Validate(ctx, kube, fabricCfg)
			assert.ErrorIs(t, actual, tt.err, "validate should return expected error")
		})
	}
}
