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
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/maruel/natural"
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
    - Switches with **leaf (l3-only)** role only support L3 VPC modes
    - Switches with **leaf (limited)** role does not support some leaf features
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

	var resSupported strings.Builder
	resSupported.WriteString(switchTableHeader)
	var resCatalogSwitches strings.Builder
	resCatalogSwitches.WriteString(switchTableHeader)
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
			return fmt.Errorf("switch profile %s does not support ACLs which makes it not suitable for any role", sp.Name) //nolint:err113
		}

		roles := getRolesHint(sp)

		slog.Debug("Adding Profile to supported", "name", nameSummary, "silicon", sp.Spec.SwitchSilicon, "roles", roles, "ports", portSummary)

		resSupported.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			nameSummary,
			roles,
			sp.Spec.SwitchSilicon,
			portSummary,
		))

		resCatalogSwitches.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
			fmt.Sprintf("[%s](#%s)", nameSummary, strings.ToLower(strings.ReplaceAll(sp.Spec.DisplayName, " ", "-"))),
			roles,
			sp.Spec.SwitchSilicon,
			portSummary,
		))
	}
	resSupported.WriteString(roleNote)
	resCatalogSwitches.WriteString(roleNote)
	featureMatrix := generateFeatureMatrix(sps)

	var resCatalog strings.Builder
	resCatalog.WriteString(catalogPageHeader + resCatalogSwitches.String() + "\n\n" + featureMatrix + "\n\n")

	resCatalog.WriteString("## Pipeline Configuration\n\n")
	resCatalog.WriteString("Some switches use hardware pipelines to organize ports. Each pipeline has a maximum number of logical ports it can support. ")
	resCatalog.WriteString("When using port breakout modes, the total number of logical ports (after breakout) within a pipeline cannot exceed the pipeline's max ports limit. ")
	resCatalog.WriteString("For example, with MaxPorts=18, a pipeline with 4 physical ports cannot break out all 4 to 8x100G (requiring 32 ports total).\n\n")

	for _, sp := range sps {
		name := sp.Name

		resCatalog.WriteString("## " + sp.Spec.DisplayName + "\n\n")
		resCatalog.WriteString("Profile Name (to use in switch object `.spec.profile`): **" + name + "**\n\n")

		if len(sp.Spec.OtherNames) > 0 {
			resCatalog.WriteString("Other names: " + strings.Join(sp.Spec.OtherNames, ", ") + "\n\n")
		}

		if sp.Name == switchprofile.VS.Name {
			resCatalog.WriteString("This is a virtual switch profile. It's for testing/demo purpose only with limited features and performance.\n\n")
		}

		roles := getRolesHint(sp)
		if len(roles) > 0 {
			resCatalog.WriteString("**Supported roles**: " + roles + "\n\n")
		}

		resCatalog.WriteString("Switch Silicon: **" + sp.Spec.SwitchSilicon + "**\n\n")

		portSummary, err := sp.Spec.GetPortsShortSummary()
		if err != nil {
			return fmt.Errorf("getting ports summary: %w", err)
		}
		resCatalog.WriteString("Ports Summary: **" + portSummary + "**\n\n")

		if sp.Spec.Notes != "" {
			resCatalog.WriteString("Notes: " + sp.Spec.Notes + "\n\n")
		}

		resCatalog.WriteString("**Supported features:**\n\n")
		resCatalog.WriteString("- Subinterfaces: " + strconv.FormatBool(sp.Spec.Features.Subinterfaces) + "\n")
		resCatalog.WriteString("- ACLs: " + strconv.FormatBool(sp.Spec.Features.ACLs) + "\n")
		resCatalog.WriteString("- L2VNI: " + strconv.FormatBool(sp.Spec.Features.L2VNI) + "\n")
		resCatalog.WriteString("- L3VNI: " + strconv.FormatBool(sp.Spec.Features.L3VNI) + "\n")
		resCatalog.WriteString("- RoCE: " + strconv.FormatBool(sp.Spec.Features.RoCE) + "\n")
		resCatalog.WriteString("- MCLAG: " + strconv.FormatBool(sp.Spec.Features.MCLAG) + "\n")
		resCatalog.WriteString("- ESLAG: " + strconv.FormatBool(sp.Spec.Features.ESLAG) + "\n")
		resCatalog.WriteString("- ECMP RoCE QPN hashing: " + strconv.FormatBool(sp.Spec.Features.ECMPRoCEQPN) + "\n")
		resCatalog.WriteString("\n")

		if sp.Spec.MaxPorts > 0 && len(sp.Spec.Pipelines) > 0 {
			resCatalog.WriteString("**Hardware Resources:**\n\n")
			resCatalog.WriteString(fmt.Sprintf("- Maximum ports supported: **%d**\n", sp.Spec.MaxPorts))
			resCatalog.WriteString(fmt.Sprintf("- Number of pipelines: **%d**\n\n", len(sp.Spec.Pipelines)))

			resCatalog.WriteString("**Pipelines:**\n\n")
			resCatalog.WriteString("| Pipeline | Max Ports | Front Panel Ports |\n")
			resCatalog.WriteString("|----------|-----------|-------------------|\n")

			pipelineKeys := lo.Keys(sp.Spec.Pipelines)
			slices.SortFunc(pipelineKeys, natural.Compare)

			pipelinePorts := make(map[string][]string)
			for _, port := range sp.Spec.Ports {
				if port.Pipeline != "" && !port.Management {
					pipelinePorts[port.Pipeline] = append(pipelinePorts[port.Pipeline], port.Label)
				}
			}

			for _, ports := range pipelinePorts {
				slices.SortFunc(ports, natural.Compare)
			}

			for _, pipelineKey := range pipelineKeys {
				pipeline := sp.Spec.Pipelines[pipelineKey]
				ports := pipelinePorts[pipelineKey]
				portsStr := strings.Join(ports, ", ")
				if portsStr == "" {
					portsStr = "-"
				}
				resCatalog.WriteString(fmt.Sprintf("| %s | %d | %s |\n",
					pipelineKey, pipeline.MaxPorts, portsStr))
			}
			resCatalog.WriteString("\n")
		}

		resCatalog.WriteString("**Available Ports:**\n\n")
		resCatalog.WriteString("Label column is a port label on a physical switch.\n")
		resCatalog.WriteString(portsTableHeader)

		portsSummary, err := sp.Spec.GetPortsSummary()
		if err != nil {
			return fmt.Errorf("getting ports summary: %w", err)
		}

		for _, port := range portsSummary {
			resCatalog.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n",
				port.Name, port.Label, port.Type, port.Group, port.Default, port.Supported))
		}

		resCatalog.WriteString("\n\n")
	}

	supportedDevicesFile := filepath.Join(targetDir, supportedDevicesFileName)
	slog.Info("Writing supported-devices file", "file", supportedDevicesFile)
	if err := os.WriteFile(supportedDevicesFile, []byte(resSupported.String()), 0o600); err != nil {
		return fmt.Errorf("writing to file %s: %w", supportedDevicesFile, err)
	}

	catalogFile := filepath.Join(targetDir, catalogFileName)
	slog.Info("Writing profiles file", "file", catalogFile)
	if err := os.WriteFile(catalogFile, []byte(resCatalog.String()), 0o600); err != nil {
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

const (
	materialCheckIcon = ":material-check:"
	materialCloseIcon = ":material-close:"
)

func generateFeatureMatrix(sps []*wiringapi.SwitchProfile) string {
	featuresType := reflect.TypeFor[wiringapi.SwitchProfileFeatures]()

	fieldDisplayNames := map[string]string{
		"ECMPRoCEQPN": "QPN",
	}

	var fieldNames []string
	for field := range featuresType.Fields() {
		if field.Type.Kind() == reflect.Bool && field.PkgPath == "" {
			fieldNames = append(fieldNames, field.Name)
		}
	}

	var result strings.Builder
	result.WriteString("## Switch Feature Matrix\n\n")
	result.WriteString("The following table shows which features are supported by each switch profile:\n\n")

	result.WriteString("| Switch Profile")
	for _, fieldName := range fieldNames {
		displayName := fieldDisplayNames[fieldName]
		if displayName == "" {
			displayName = fieldName
		}
		result.WriteString(" | " + displayName)
	}
	result.WriteString(" |\n")

	result.WriteString("|---")
	for range fieldNames {
		result.WriteString("|:---:")
	}
	result.WriteString("|\n")

	for _, sp := range sps {
		if strings.HasSuffix(sp.Name, "-clsp") {
			continue
		}

		nameSummary := sp.Spec.GetNameSummary()
		anchor := strings.ToLower(strings.ReplaceAll(sp.Spec.DisplayName, " ", "-"))

		result.WriteString(fmt.Sprintf("| [%s](#%s)", nameSummary, anchor))

		featuresValue := reflect.ValueOf(sp.Spec.Features)
		for _, fieldName := range fieldNames {
			fieldValue := featuresValue.FieldByName(fieldName).Bool()
			icon := materialCloseIcon
			if fieldValue {
				icon = materialCheckIcon
			}
			result.WriteString(" | " + icon)
		}
		result.WriteString(" |\n")
	}

	return result.String() + "\n\n"
}
