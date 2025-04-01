// Copyright 2023 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package switchprofile_test

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDefaultSwitchProfiles(t *testing.T) {
	ctx := t.Context()

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))

	kube := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects().
		Build()

	profiles := switchprofile.NewDefaultSwitchProfiles()
	require.NoError(t, profiles.RegisterAll(ctx, kube, &meta.FabricConfig{}))

	for _, sp := range profiles.List() {
		require.NotEmpty(t, sp.Name, "switch profile name should be set")
		require.NotNil(t, profiles.Get(sp.Name), "switch profile %q should be registered", sp.Name)
		require.Equal(t, kmetav1.NamespaceDefault, sp.Namespace, "switch profile %q should be in default namespace", sp.Name)

		_, err := sp.Validate(ctx, kube, &meta.FabricConfig{})
		require.NoError(t, err, "switch profile %q should be valid", sp.Name)
	}
}

func TestDefaultSwitchProfilesEnforcement(t *testing.T) {
	ctx := t.Context()

	scheme := runtime.NewScheme()
	require.NoError(t, wiringapi.AddToScheme(scheme))

	kube := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(&wiringapi.SwitchProfile{
			ObjectMeta: kmetav1.ObjectMeta{
				Name: "some-profile",
			},
		}).
		Build()

	profiles := switchprofile.NewDefaultSwitchProfiles()
	require.NoError(t, profiles.RegisterAll(ctx, kube, &meta.FabricConfig{}))
	require.NoError(t, profiles.Enforce(ctx, kube, &meta.FabricConfig{}, false))
	require.True(t, profiles.IsInitialized())

	actualList := &wiringapi.SwitchProfileList{}
	require.NoError(t, kube.List(ctx, actualList))

	expected := []wiringapi.SwitchProfile{}
	for _, sp := range profiles.List() {
		expected = append(expected, *sp)
	}
	sort.Slice(expected, func(i, j int) bool {
		return expected[i].Name < expected[j].Name
	})

	actual := []wiringapi.SwitchProfile{}
	for _, sp := range actualList.Items {
		sp.ResourceVersion = ""
		sp.Default()
		actual = append(actual, sp)
	}
	sort.Slice(actual, func(i, j int) bool {
		return actual[i].Name < actual[j].Name
	})

	require.Equal(t, expected, actual, "only default switch profiles should be present")
}
