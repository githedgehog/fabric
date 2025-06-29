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

package gen

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/samber/lo"
	"go.githedgehog.com/fabric/api/meta"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/ctrl/switchprofile"
)

const (
	catalogFileName          = "profiles.md"
	supportedDevicesFileName = "supported-devices.md"

	catalogPageHeader = `# Switch Catalog

The following is a list of all supported switches with their supported capabilities and configuration. Please, make sure
to use the version of documentation that matches your environment to get an up-to-date list of supported switches, their
features and port naming scheme.

`

	portsTableHeader = `
| Port | Label | Type | Group | Default | Supported |
|------|-------|------|-------|---------|-----------|
`

	switchTableHeader = `
| Switch | Supported Roles | Silicon | Ports |
|--------|-----------------|---------|-------|
`

	roleNote = `
!!! note
    - Switches that support **leaf** role could be used for the collapsed-core topology as well
    - Switches with **leaf (l3-only)** role only support L3 VPC modes
    - Switches with **leaf (limited)** role does not support some leaf features and are not supported in the
      collapsed-core topology
`
)

func GenerateProfilesRef(ctx context.Context, targetDir string) error {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("creating target directory %s: %w", targetDir, err)
	}

	def := switchprofile.NewDefaultSwitchProfiles()
	err := def.RegisterAll(ctx, nil, &meta.FabricConfig{})
	if err != nil {
		return fmt.Errorf("registering default switch profiles: %w", err)
	}

	sps := def.List()
	slices.SortFunc(sps, func(a, b *wiringapi.SwitchProfile) int {
		return strings.Compare(a.Name, b.Name)
	})

	slog.Debug("Loaded Switch Profiles", "count", len(sps))

	resSupported := switchTableHeader
	resCatalogSwitches := switchTableHeader
	for _, sp := range sps {
		if sp.Name == switchprofile.VS.Name {
			continue
		}

		nameSummary := sp.Spec.GetNameSummary()
		portSummary, err := sp.Spec.GetPortsShortSummary()
		if err != nil {
			return fmt.Errorf("getting ports summary: %w", err)
		}

		if !sp.Spec.Features.ACLs {
			return fmt.Errorf("switch profile %s does not support ACLs which makes it not suitable for any role", sp.Name) //nolint:goerr113
		}

		roles := getRolesHint(sp)

		slog.Debug("Adding Profile to supported", "name", nameSummary, "silicon", sp.Spec.SwitchSilicon, "roles", roles, "ports", portSummary)

		resSupported += fmt.Sprintf("| %s | %s | %s | %s |\n",
			nameSummary,
			roles,
			sp.Spec.SwitchSilicon,
			portSummary,
		)

		resCatalogSwitches += fmt.Sprintf("| %s | %s | %s | %s |\n",
			fmt.Sprintf("[%s](#%s)", nameSummary, strings.ToLower(strings.ReplaceAll(sp.Spec.DisplayName, " ", "-"))),
			roles,
			sp.Spec.SwitchSilicon,
			portSummary,
		)
	}
	resSupported += roleNote
	resCatalogSwitches += roleNote

	resCatalog := catalogPageHeader + resCatalogSwitches + "\n\n"
	for _, sp := range sps {
		name := sp.Name

		resCatalog += "## " + sp.Spec.DisplayName + "\n\n"
		resCatalog += "Profile Name (to use in switch object `.spec.profile`): **" + name + "**\n\n"

		if len(sp.Spec.OtherNames) > 0 {
			resCatalog += "Other names: " + strings.Join(sp.Spec.OtherNames, ", ") + "\n\n"
		}

		if sp.Name == switchprofile.VS.Name {
			resCatalog += "This is a virtual switch profile. It's for testing/demo purpose only with limited features and performance.\n\n"
		}

		roles := getRolesHint(sp)
		if len(roles) > 0 {
			resCatalog += "**Supported roles**: " + roles + "\n\n"
		}

		resCatalog += "Switch Silicon: **" + sp.Spec.SwitchSilicon + "**\n\n"

		portSummary, err := sp.Spec.GetPortsShortSummary()
		if err != nil {
			return fmt.Errorf("getting ports summary: %w", err)
		}
		resCatalog += "Ports Summary: **" + portSummary + "**\n\n"

		if sp.Spec.Notes != "" {
			resCatalog += "Notes: " + sp.Spec.Notes + "\n\n"
		}

		resCatalog += "**Supported features:**\n\n"
		resCatalog += "- Subinterfaces: " + strconv.FormatBool(sp.Spec.Features.Subinterfaces) + "\n"
		resCatalog += "- ACLs: " + strconv.FormatBool(sp.Spec.Features.ACLs) + "\n"
		resCatalog += "- L2VNI: " + strconv.FormatBool(sp.Spec.Features.L2VNI) + "\n"
		resCatalog += "- L3VNI: " + strconv.FormatBool(sp.Spec.Features.L3VNI) + "\n"
		resCatalog += "- RoCE: " + strconv.FormatBool(sp.Spec.Features.RoCE) + "\n"
		resCatalog += "- MCLAG: " + strconv.FormatBool(sp.Spec.Features.MCLAG) + "\n"
		resCatalog += "- ESLAG: " + strconv.FormatBool(sp.Spec.Features.ESLAG) + "\n"
		resCatalog += "- ECMP RoCE QPN hashing: " + strconv.FormatBool(sp.Spec.Features.ECMPRoCEQPN) + "\n"
		resCatalog += "\n"

		resCatalog += "**Available Ports:**\n\n"
		resCatalog += "Label column is a port label on a physical switch.\n"
		resCatalog += portsTableHeader

		portsSummary, err := sp.Spec.GetPortsSummary()
		if err != nil {
			return fmt.Errorf("getting ports summary: %w", err)
		}

		for _, port := range portsSummary {
			resCatalog += fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
				port.Name, port.Label, port.Type, port.Group, port.Default, port.Supported)
		}

		resCatalog += "\n\n"
	}

	supportedDevicesFile := filepath.Join(targetDir, supportedDevicesFileName)
	slog.Info("Writing supported-devices file", "file", supportedDevicesFile)
	if err := os.WriteFile(supportedDevicesFile, []byte(resSupported), 0o600); err != nil {
		return fmt.Errorf("writing to file %s: %w", supportedDevicesFile, err)
	}

	catalogFile := filepath.Join(targetDir, catalogFileName)
	slog.Info("Writing profiles file", "file", catalogFile)
	if err := os.WriteFile(catalogFile, []byte(resCatalog), 0o600); err != nil {
		return fmt.Errorf("writing to file %s: %w", catalogFile, err)
	}

	return nil
}

// TODO replace with a proper roles handling in switch profiles
func getRolesHint(sp *wiringapi.SwitchProfile) string {
	roles := []string{}
	if sp.Name != switchprofile.EdgecoreEPS203.Name {
		roles = append(roles, "spine")
	}

	f := sp.Spec.Features

	switch {
	case f.L2VNI && f.L3VNI && f.Subinterfaces:
		roles = append(roles, "leaf")
	case !f.L2VNI && f.L3VNI && f.Subinterfaces:
		roles = append(roles, "leaf (l3-only)")
	case f.L2VNI && f.L3VNI && !f.Subinterfaces:
		roles = append(roles, "leaf (limited)")
	}

	return strings.Join(lo.Map(roles, func(item string, _ int) string {
		return fmt.Sprintf("**%s**", item)
	}), ", ")
}
