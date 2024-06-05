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

package profilesref

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
)

const (
	header = `# Switch Profiles Catalog

The following is a list of all supported switches. Please, make sure to use correct version of documentation to get an
up-to-date list of supported switches, their features and port naming scheme.

`

	portsHeader = `
| Port | Label | Type | Group | Default | Supported |
|------|-------|------|-------|---------|-----------|
`
)

func Generate(ctx context.Context, target string) error {
	res := header

	def := switchprofile.NewDefaultSwitchProfiles()
	err := def.RegisterAll(ctx, nil, &meta.FabricConfig{})
	if err != nil {
		return errors.Wrapf(err, "failed to register default switch profiles")
	}

	profiles := def.List()
	slices.SortFunc(profiles, func(a, b *wiringapi.SwitchProfile) int {
		return strings.Compare(a.Name, b.Name)
	})

	for _, profile := range profiles {
		name := profile.Name

		res += "## " + profile.Spec.DisplayName + "\n\n"
		res += "Profile Name (to use in switch.spec.profile): **" + name + "**\n\n"

		if len(profile.Spec.OtherNames) > 0 {
			res += "Other names: " + strings.Join(profile.Spec.OtherNames, ", ") + "\n\n"
		}

		res += "**Supported features:**\n\n"
		res += "- Subinterfaces: " + strconv.FormatBool(profile.Spec.Features.Subinterfaces) + "\n"
		res += "- VXLAN: " + strconv.FormatBool(profile.Spec.Features.VXLAN) + "\n"
		res += "- ACLs: " + strconv.FormatBool(profile.Spec.Features.ACLs) + "\n"
		res += "\n"

		res += "**Available Ports:**\n\n"
		res += "Label column is a port label on a physical switch.\n"
		res += portsHeader

		portsSummary, err := profile.Spec.GetPortsSummary()
		if err != nil {
			return errors.Wrapf(err, "failed to get ports summary")
		}

		for _, port := range portsSummary {
			res += fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
				port.Name, port.Label, port.Type, port.Group, port.Default, port.Supported)
		}

		res += "\n\n"
	}

	return errors.Wrapf(os.WriteFile(target, []byte(res), 0o600), "failed to write profiles reference")
}
