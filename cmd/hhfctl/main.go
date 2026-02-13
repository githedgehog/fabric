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

package main

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"github.com/pkg/errors"
	"github.com/urfave/cli/v2"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	"go.githedgehog.com/fabric/pkg/hhfctl"
	"go.githedgehog.com/fabric/pkg/hhfctl/inspect"
	"go.githedgehog.com/fabric/pkg/util/pointer"
	"go.githedgehog.com/fabric/pkg/version"
	gwapi "go.githedgehog.com/gateway/api/gateway/v1alpha1"
	"k8s.io/klog/v2"
	kctrl "sigs.k8s.io/controller-runtime"
)

func setupLogger(verbose bool) error {
	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}

	logW := os.Stderr

	slog.SetDefault(slog.New(tint.NewHandler(logW, &tint.Options{
		Level:      logLevel,
		TimeFormat: time.TimeOnly,
		NoColor:    !isatty.IsTerminal(logW.Fd()),
	})))

	kubeHandler := tint.NewHandler(logW, &tint.Options{
		Level:      slog.LevelInfo,
		TimeFormat: time.TimeOnly,
		NoColor:    !isatty.IsTerminal(logW.Fd()),
	})
	kctrl.SetLogger(logr.FromSlogHandler(kubeHandler))
	klog.SetSlogLogger(slog.New(kubeHandler))

	return nil
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var verbose bool
	verboseFlag := &cli.BoolFlag{
		Name:        "verbose",
		Aliases:     []string{"v"},
		Usage:       "verbose output (includes debug)",
		Value:       true, // TODO disable debug by default
		Destination: &verbose,
	}

	var name string
	nameFlag := &cli.StringFlag{
		Name:        "name",
		Aliases:     []string{"n"},
		Usage:       "name",
		Destination: &name,
	}

	var username string
	usernameFlag := &cli.StringFlag{
		Name:        "username",
		Aliases:     []string{"u"},
		Usage:       "username",
		Destination: &username,
		Value:       "admin",
	}

	var yes bool
	yesFlag := &cli.BoolFlag{
		Name:        "yes",
		Aliases:     []string{"y"},
		Usage:       "assume yes",
		Destination: &yes,
	}
	yesCheck := func(_ *cli.Context) error {
		if !yes {
			return cli.Exit("Potentially dangerous operation. Please confirm with --yes if you're sure.", 1)
		}

		return nil
	}

	var printYaml bool
	printYamlFlag := &cli.BoolFlag{
		Name:        "print",
		Aliases:     []string{"p"},
		Usage:       "print object yaml",
		Destination: &printYaml,
	}

	outputTypes := []string{}
	for _, t := range inspect.OutputTypes {
		outputTypes = append(outputTypes, string(t))
	}

	var output string
	outputFlag := &cli.StringFlag{
		Name:        "output",
		Aliases:     []string{"o"},
		Usage:       "output format, one of " + strings.Join(outputTypes, ", "),
		Value:       "text",
		Destination: &output,
	}

	appName := "hhfctl"
	usage := "Hedgehog Fabric API CLI client"
	if len(os.Args) > 0 {
		if strings.HasSuffix(os.Args[0], "kubectl-fabric") {
			appName = "kubectl fabric"
			usage = "Hedgehog Fabric API kubectl plugin"
		} else if strings.HasSuffix(os.Args[0], "fabric") {
			appName = "fabric"
		}
	}

	cli.VersionFlag.(*cli.BoolFlag).Aliases = []string{"V"}
	app := &cli.App{
		Name:                   appName,
		Usage:                  usage,
		Version:                version.Version,
		Suggest:                true,
		UseShortOptionHandling: true,
		EnableBashCompletion:   true,
		Flags: []cli.Flag{
			verboseFlag,
		},
		Commands: []*cli.Command{
			{
				Name:  "vpc",
				Usage: "VPC commands",
				Flags: []cli.Flag{
					verboseFlag,
				},
				Subcommands: []*cli.Command{
					{
						Name:  "create",
						Usage: "Create vpc",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							&cli.StringFlag{
								Name:     "subnet",
								Usage:    "subnet",
								Required: true,
							},
							&cli.StringFlag{
								Name:  "vlan",
								Usage: "vlan",
							},
							&cli.BoolFlag{
								Name:  "dhcp",
								Usage: "enable dhcp",
							},
							&cli.StringFlag{
								Name:    "dhcp-range-start",
								Aliases: []string{"dhcp-start"},
								Usage:   "dhcp range start",
							},
							&cli.StringFlag{
								Name:    "dhcp-range-end",
								Aliases: []string{"dhcp-end"},
								Usage:   "dhcp range end",
							},
							&cli.StringFlag{
								Name:    "dhcp-lease-time",
								Aliases: []string{"dhcp-lease"},
								Usage:   "dhcp lease time in seconds",
							},
							&cli.StringFlag{
								Name:    "vpc-mode",
								Aliases: []string{"mode"},
								Usage:   "vpc mode, e.g. empty for l2vni (default), l3vni etc.",
							},
							&cli.BoolFlag{
								Name:    "dhcp-disable-default-route",
								Aliases: []string{"dhcp-no-default"},
								Usage:   "disable default route advertisement in dhcp",
							},
							&cli.StringSliceFlag{
								Name:  "dhcp-advertised-routes",
								Usage: "custom routes to advertise in dhcp, in the format prefix-gateway, e.g. 8.8.8.0/24-192.168.1.1",
							},
							&cli.BoolFlag{
								Name:  "host-bgp",
								Usage: "mark the subnet as dedicated to BGP speakers",
							},
							printYamlFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							cliRoutes := cCtx.StringSlice("dhcp-advertised-routes")
							advertisedRoutes := make([]vpcapi.VPCDHCPRoute, 0, len(cliRoutes))
							for _, route := range cliRoutes {
								parts := strings.Split(route, "-")
								if len(parts) != 2 {
									return cli.Exit(fmt.Sprintf("invalid dhcp-advertised-routes format: %s, expected prefix-gateway", route), 1)
								}
								advertisedRoutes = append(advertisedRoutes, vpcapi.VPCDHCPRoute{
									Destination: parts[0],
									Gateway:     parts[1],
								})
							}

							return errors.Wrapf(hhfctl.VPCCreate(ctx, printYaml, &hhfctl.VPCCreateOptions{
								Name:   name,
								Subnet: cCtx.String("subnet"),
								VLAN:   uint16(cCtx.Uint("vlan")), //nolint:gosec
								DHCP: vpcapi.VPCDHCP{
									Enable: cCtx.Bool("dhcp"),
									Range: &vpcapi.VPCDHCPRange{
										Start: cCtx.String("dhcp-range-start"),
										End:   cCtx.String("dhcp-range-end"),
									},
									Options: &vpcapi.VPCDHCPOptions{
										PXEURL:              cCtx.String("dhcp-pxe-url"),
										LeaseTimeSeconds:    uint32(cCtx.Uint("dhcp-lease-time")), //nolint:gosec
										DisableDefaultRoute: cCtx.Bool("dhcp-disable-default-route"),
										AdvertisedRoutes:    advertisedRoutes,
									},
								},
								Mode:    vpcapi.VPCMode(cCtx.String("vpc-mode")),
								HostBGP: cCtx.Bool("host-bgp"),
							}), "failed to create vpc")
						},
					},
					{
						Name:  "attach",
						Usage: "Attach connection to vpc",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							&cli.StringFlag{
								Name:     "vpc-subnet",
								Aliases:  []string{"subnet"},
								Usage:    "vpc/subnet",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "connection",
								Aliases:  []string{"conn"},
								Usage:    "connection",
								Required: true,
							},
							&cli.BoolFlag{
								Name:     "nativeVLAN",
								Aliases:  []string{"vlan"},
								Usage:    "set to True for untagged traffic, otherwise traffic is tagged",
								Required: false,
								Value:    false,
							},
							printYamlFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(hhfctl.VPCAttach(ctx, printYaml, &hhfctl.VPCAttachOptions{
								Name:       name,
								VPCSubnet:  cCtx.String("vpc-subnet"),
								Connection: cCtx.String("connection"),
								NativeVLAN: cCtx.Bool("nativeVLAN"),
							}), "failed to attach connection to vpc")
						},
					},
					{
						Name:    "peer",
						Aliases: []string{"peering"},
						Usage:   "Enable peering between vpcs",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							&cli.StringSliceFlag{
								Name:     "vpc",
								Usage:    "vpc",
								Required: true,
							},
							&cli.StringFlag{
								Name:  "remote",
								Usage: "SwitchGroup name for remote peering",
							},
							printYamlFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(hhfctl.VPCPeer(ctx, printYaml, &hhfctl.VPCPeerOptions{
								Name:   name,
								VPCs:   cCtx.StringSlice("vpc"),
								Remote: cCtx.String("remote"),
							}), "failed to peer vpcs")
						},
					},
					{
						Name:    "gateway-peer",
						Aliases: []string{"gateway-peering", "gw-peer", "gw-peering"},
						Usage:   "Enable peering via the gateway between two vpcs, or a vpc and an external (use the 'ext.' prefix for externals)",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							&cli.StringFlag{
								Name:     "vpc-1",
								Usage:    "name of the first vpc for the peering",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "vpc-2",
								Usage:    "name of the second vpc for the peering",
								Required: true,
							},
							&cli.StringSliceFlag{
								Name:    "vpc-1-ip",
								Aliases: []string{"ip-1"},
								Usage:   "CIDR to expose for vpc-1",
							},
							&cli.StringSliceFlag{
								Name:    "vpc-1-subnet",
								Aliases: []string{"subnet-1"},
								Usage:   "subnet to expose for vpc-1",
							},
							&cli.StringSliceFlag{
								Name:    "vpc-1-as",
								Aliases: []string{"as-1"},
								Usage:   "CIDR to use for the As range for vpc-1",
							},
							&cli.BoolFlag{
								Name:    "vpc-1-default",
								Aliases: []string{"default-1"},
								Usage:   "expose all prefixes that are not explicitly caught by other peerings",
							},
							&cli.StringFlag{
								Name:    "vpc-1-nat",
								Aliases: []string{"nat-1"},
								Usage:   "nat type for vpc-1, one of static|masquerade|port-forward",
							},
							&cli.StringSliceFlag{
								Name:    "vpc-1-pf",
								Aliases: []string{"pf-1"},
								Usage:   "port forwarding rules for vpc-1",
							},
							&cli.StringSliceFlag{
								Name:    "vpc-2-ip",
								Aliases: []string{"ip-2"},
								Usage:   "CIDR to expose for vpc-2",
							},
							&cli.StringSliceFlag{
								Name:    "vpc-2-subnet",
								Aliases: []string{"subnet-2"},
								Usage:   "subnet to expose for vpc-2",
							},
							&cli.StringSliceFlag{
								Name:    "vpc-2-as",
								Aliases: []string{"as-2"},
								Usage:   "CIDR to use for the As range for vpc-2",
							},
							&cli.BoolFlag{
								Name:    "vpc-2-default",
								Aliases: []string{"default-2"},
								Usage:   "expose all prefixes that are not explicitly caught by other peerings",
							},
							&cli.StringFlag{
								Name:    "vpc-2-nat",
								Aliases: []string{"nat-2"},
								Usage:   "nat type for vpc-2, one of static|masquerade|port-forward",
							},
							&cli.StringSliceFlag{
								Name:    "vpc-2-pf",
								Aliases: []string{"pf-2"},
								Usage:   "port forwarding rules for vpc-2",
							},
							printYamlFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							options := &hhfctl.VPCGwPeerOptions{
								Name: name,
								VPC1: cCtx.String("vpc-1"),
								VPC2: cCtx.String("vpc-2"),
								VPC1Expose: gwapi.PeeringEntryExpose{
									IPs:                []gwapi.PeeringEntryIP{},
									DefaultDestination: cCtx.Bool("vpc-1-default"),
								},
								VPC2Expose: gwapi.PeeringEntryExpose{
									IPs:                []gwapi.PeeringEntryIP{},
									DefaultDestination: cCtx.Bool("vpc-2-default"),
								},
							}
							// TODO: taken from fabricator, should place this somewhere else (gateway?) and deduplicate
							parsePFRules := func(rules []string) ([]gwapi.PeeringNATPortForwardEntry, error) {
								out := make([]gwapi.PeeringNATPortForwardEntry, 0, len(rules))
								for idx, rule := range rules {
									rule = strings.TrimSpace(rule)
									if rule == "" {
										return nil, fmt.Errorf("invalid port-forward rule at index %d: should not be empty", idx) //nolint:goerr113
									}

									kv := strings.Split(rule, "=")
									if len(kv) != 2 {
										return nil, fmt.Errorf("invalid port-forward rule %q at index %d: must be in format [proto/]port=as", rule, idx) //nolint:goerr113
									}
									left := strings.TrimSpace(kv[0])
									right := strings.TrimSpace(kv[1])
									if left == "" || right == "" {
										return nil, fmt.Errorf("invalid port-forward rule %q at index %d: port and as must be non-empty", rule, idx) //nolint:goerr113
									}

									entry := gwapi.PeeringNATPortForwardEntry{
										Protocol: gwapi.PeeringNATProtocolAny,
									}

									// [proto/]port
									if strings.Contains(left, "/") {
										portParts := strings.Split(left, "/")
										if len(portParts) != 2 {
											return nil, fmt.Errorf("invalid port-forward rule %q at index %d: left side must be in format proto/port", rule, idx) //nolint:goerr113
										}
										proto := strings.TrimSpace(portParts[0])
										port := strings.TrimSpace(portParts[1])
										if proto == "" || port == "" {
											return nil, fmt.Errorf("invalid port-forward rule %q at index %d: proto and port must be non-empty", rule, idx) //nolint:goerr113
										}
										switch proto {
										case string(gwapi.PeeringNATProtocolTCP):
											entry.Protocol = gwapi.PeeringNATProtocolTCP
										case string(gwapi.PeeringNATProtocolUDP):
											entry.Protocol = gwapi.PeeringNATProtocolUDP
										case string(gwapi.PeeringNATProtocolAny):
											entry.Protocol = gwapi.PeeringNATProtocolAny
										default:
											return nil, fmt.Errorf("invalid port-forward rule %q at index %d: unknown protocol %q (supported: tcp, udp)", rule, idx, proto) //nolint:goerr113
										}
										entry.Port = port
									} else {
										entry.Port = left
									}

									entry.As = right

									// only the most basic of validation, let's not duplicate code; alternatively, let's make the validation function in gwapi public
									if strings.Contains(entry.Port, ",") || strings.TrimSpace(entry.Port) != entry.Port || entry.Port == "" {
										return nil, fmt.Errorf("invalid port %q in port-forward rule %q at index %d", entry.Port, rule, idx) //nolint:goerr113
									}
									if strings.Contains(entry.As, ",") || strings.TrimSpace(entry.As) != entry.As || entry.As == "" {
										return nil, fmt.Errorf("invalid as %q in port-forward rule %q at index %d", entry.As, rule, idx) //nolint:goerr113
									}

									out = append(out, entry)
								}

								return out, nil
							}

							ips1 := cCtx.StringSlice("vpc-1-ip")
							if len(ips1) > 0 {
								for _, cidr := range ips1 {
									options.VPC1Expose.IPs = append(options.VPC1Expose.IPs, gwapi.PeeringEntryIP{CIDR: cidr})
								}
							}
							subnets1 := cCtx.StringSlice("vpc-1-subnet")
							if len(subnets1) > 0 {
								for _, subnet := range subnets1 {
									options.VPC1Expose.IPs = append(options.VPC1Expose.IPs, gwapi.PeeringEntryIP{VPCSubnet: subnet})
								}
							}
							as1 := cCtx.StringSlice("vpc-1-as")
							if len(as1) > 0 {
								options.VPC1Expose.As = []gwapi.PeeringEntryAs{}
								for _, as := range as1 {
									options.VPC1Expose.As = append(options.VPC1Expose.As, gwapi.PeeringEntryAs{CIDR: as})
								}
							}
							nat1 := cCtx.String("vpc-1-nat")
							switch nat1 {
							case "":
							case "static":
								options.VPC1Expose.NAT = &gwapi.PeeringNAT{Static: &gwapi.PeeringNATStatic{}}
							case "masquerade":
								options.VPC1Expose.NAT = &gwapi.PeeringNAT{Masquerade: &gwapi.PeeringNATMasquerade{}}
							case "port-forward", "portforward":
								options.VPC1Expose.NAT = &gwapi.PeeringNAT{PortForward: &gwapi.PeeringNATPortForward{}}
							default:
								return cli.Exit(fmt.Sprintf("invalid nat type for vpc-1: %s, expected one of static, masquerade, port-forward", nat1), 1)
							}
							pf1 := cCtx.StringSlice("vpc-1-pf")
							if len(pf1) > 0 {
								if options.VPC1Expose.NAT == nil || options.VPC1Expose.NAT.PortForward == nil {
									return cli.Exit("port-forward rules specified for vpc-1 but nat type is not port-forward", 1)
								}
								pfRules, err := parsePFRules(pf1)
								if err != nil {
									return cli.Exit(fmt.Sprintf("invalid port-forward rule for vpc-1: %v", err), 1)
								}
								options.VPC1Expose.NAT.PortForward.Ports = pfRules
							} else if options.VPC1Expose.NAT != nil && options.VPC1Expose.NAT.PortForward != nil {
								return cli.Exit("nat type for vpc-1 is port-forward but no port-forward rules specified", 1)
							}
							ips2 := cCtx.StringSlice("vpc-2-ip")
							if len(ips2) > 0 {
								for _, cidr := range ips2 {
									options.VPC2Expose.IPs = append(options.VPC2Expose.IPs, gwapi.PeeringEntryIP{CIDR: cidr})
								}
							}
							subnets2 := cCtx.StringSlice("vpc-2-subnet")
							if len(subnets2) > 0 {
								for _, subnet := range subnets2 {
									options.VPC2Expose.IPs = append(options.VPC2Expose.IPs, gwapi.PeeringEntryIP{VPCSubnet: subnet})
								}
							}
							as2 := cCtx.StringSlice("vpc-2-as")
							if len(as2) > 0 {
								options.VPC2Expose.As = []gwapi.PeeringEntryAs{}
								for _, as := range as2 {
									options.VPC2Expose.As = append(options.VPC2Expose.As, gwapi.PeeringEntryAs{CIDR: as})
								}
							}
							nat2 := cCtx.String("vpc-2-nat")
							switch nat2 {
							case "":
							case "static":
								options.VPC2Expose.NAT = &gwapi.PeeringNAT{Static: &gwapi.PeeringNATStatic{}}
							case "masquerade":
								options.VPC2Expose.NAT = &gwapi.PeeringNAT{Masquerade: &gwapi.PeeringNATMasquerade{}}
							case "port-forward", "portforward":
								options.VPC2Expose.NAT = &gwapi.PeeringNAT{PortForward: &gwapi.PeeringNATPortForward{}}
							default:
								return cli.Exit(fmt.Sprintf("invalid nat type for vpc-2: %s, expected one of static, masquerade, port-forward", nat2), 1)
							}
							pf2 := cCtx.StringSlice("vpc-2-pf")
							if len(pf2) > 0 {
								if options.VPC2Expose.NAT == nil || options.VPC2Expose.NAT.PortForward == nil {
									return cli.Exit("port-forward rules specified for vpc-2 but nat type is not port-forward", 1)
								}
								pfRules, err := parsePFRules(pf2)
								if err != nil {
									return cli.Exit(fmt.Sprintf("invalid port-forward rule for vpc-2: %v", err), 1)
								}
								options.VPC2Expose.NAT.PortForward.Ports = pfRules
							} else if options.VPC2Expose.NAT != nil && options.VPC2Expose.NAT.PortForward != nil {
								return cli.Exit("nat type for vpc-2 is port-forward but no port-forward rules specified", 1)
							}

							return errors.Wrapf(hhfctl.VPCGwPeer(ctx, printYaml, options), "failed to peer vpcs via the gateway")
						},
					},
					{
						Name:  "wipe",
						Usage: "Delete all vpcs, their peerings (incl. external) and attachments",
						Flags: []cli.Flag{
							yesFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							if err := yesCheck(cCtx); err != nil {
								return wrapErrWithPressToContinue(err)
							}

							return wrapErrWithPressToContinue(errors.Wrapf(hhfctl.VPCWipe(ctx), "failed to wipe vpcs"))
						},
					},
					{
						Name:  "cleanup-leases",
						Usage: "Cleanup dhcp leases for specified vpc subnet with expiry older than the specified age",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "vpc",
								Usage: "VPC name",
							},
							&cli.StringFlag{
								Name:  "subnet",
								Usage: "Subnet name",
							},
							&cli.StringFlag{
								Name:    "older-than",
								Aliases: []string{"older"},
								Usage:   "Age in 'duration' format: e.g. 3600s, 60m, 1h",
								Value:   "1s",
							},
							&cli.BoolFlag{
								Name:  "dry-run",
								Usage: "Dry run",
							},
							yesFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							dryRun := cCtx.Bool("dry-run")

							if !dryRun {
								if err := yesCheck(cCtx); err != nil {
									return wrapErrWithPressToContinue(err)
								}
							}

							return wrapErrWithPressToContinue(errors.Wrapf(hhfctl.DHCPSubnetCleanup(ctx, hhfctl.DHCPSubnetCleanupOptions{
								VPC:       cCtx.String("vpc"),
								Subnet:    cCtx.String("subnet"),
								OlderThan: cCtx.String("older-than"),
								DryRun:    dryRun,
							}), "failed to cleanup dhcp leases"))
						},
					},
					{
						Name:  "static-lease",
						Usage: "Add a static lease to the specified vpc subnet, MAC and IP",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "vpc",
								Usage: "VPC name",
							},
							&cli.StringFlag{
								Name:  "subnet",
								Usage: "Subnet name",
							},
							&cli.StringFlag{
								Name:  "mac",
								Usage: "MAC address",
							},
							&cli.StringFlag{
								Name:  "ip",
								Usage: "IP address, use empty string to remove static lease",
							},
							yesFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							dryRun := cCtx.Bool("dry-run")

							if !dryRun {
								if err := yesCheck(cCtx); err != nil {
									return wrapErrWithPressToContinue(err)
								}
							}

							return wrapErrWithPressToContinue(errors.Wrapf(hhfctl.DHCPSubnetStaticLease(ctx, hhfctl.DHCPSubnetStaticLeaseOpts{
								VPC:    cCtx.String("vpc"),
								Subnet: cCtx.String("subnet"),
								MAC:    cCtx.String("mac"),
								IP:     cCtx.String("ip"),
							}), "failed to cleanup dhcp leases"))
						},
					},
				},
			},
			{
				Name:    "switch",
				Aliases: []string{"sw"},
				Usage:   "Switch commands",
				Flags: []cli.Flag{
					verboseFlag,
				},
				Subcommands: []*cli.Command{
					{
						Name:  "ip",
						Usage: "Get switch management IP address",
						Flags: []cli.Flag{
							usernameFlag,
							verboseFlag,
							nameFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(_ *cli.Context) error {
							return errors.Wrapf(hhfctl.SwitchIP(ctx, name), "failed to get switch IP address")
						},
					},
					{
						Name:  "ssh",
						Usage: "SSH into the switch (only from control nodes, using mgmt network)",
						Flags: []cli.Flag{
							usernameFlag,
							verboseFlag,
							nameFlag,
							&cli.StringFlag{
								Name:    "run",
								Aliases: []string{"r"},
								Usage:   "command to run",
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return wrapErrWithPressToContinue(errors.Wrapf(hhfctl.SwitchSSH(ctx, name, username, cCtx.String("run")), "failed to ssh into the switch"))
						},
					},
					{
						Name:  "serial",
						Usage: "Run serial console for the switch (only if it's specified in the switch annotations)",
						Flags: []cli.Flag{
							usernameFlag,
							verboseFlag,
							nameFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(_ *cli.Context) error {
							return wrapErrWithPressToContinue(errors.Wrapf(hhfctl.SwitchSerial(ctx, name), "failed to run serial for the switch"))
						},
					},
					{
						Name:  "reboot",
						Usage: "Reboot the switch (only works if switch is healthy and sends heartbeats)",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							yesFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							if err := yesCheck(cCtx); err != nil {
								return wrapErrWithPressToContinue(err)
							}

							return wrapErrWithPressToContinue(errors.Wrapf(hhfctl.SwitchReboot(ctx, name), "failed to reboot switch"))
						},
					},
					{
						Name:  "power-reset",
						Usage: "Power reset the switch (UNSAFE, skips graceful shutdown, only works if switch is healthy and sends heartbeats)",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							yesFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							if err := yesCheck(cCtx); err != nil {
								return wrapErrWithPressToContinue(err)
							}

							return wrapErrWithPressToContinue(errors.Wrapf(hhfctl.SwitchPowerReset(ctx, name), "failed to power reset switch"))
						},
					},
					{
						Name:  "reinstall",
						Usage: "Reinstall the switch (reboot into ONIE, only works if switch is healthy and sends heartbeats)",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							yesFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							if err := yesCheck(cCtx); err != nil {
								return wrapErrWithPressToContinue(err)
							}

							return wrapErrWithPressToContinue(errors.Wrapf(hhfctl.SwitchReinstall(ctx, name), "failed to reinstall switch"))
						},
					},
					{
						Name:  "roce",
						Usage: "Set RoCE mode on the switch (automatically reboots switch)",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							yesFlag,
							&cli.BoolFlag{
								Name:  "set",
								Usage: "Enable or disable RoCE mode, keep empty to toggle",
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							if err := yesCheck(cCtx); err != nil {
								return wrapErrWithPressToContinue(err)
							}

							var value *bool
							if cCtx.IsSet("set") {
								value = pointer.To(cCtx.Bool("set"))
							}

							return wrapErrWithPressToContinue(errors.Wrapf(hhfctl.SwitchRoCE(ctx, name, value), "failed to set roce mode"))
						},
					},
					{
						Name:  "ecmp-roce-qpn",
						Usage: "Set ECMP RoCE QPN hashing",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							yesFlag,
							&cli.BoolFlag{
								Name:  "set",
								Usage: "Enable or disable ECMP RoCE QPN hashing, keep empty to toggle",
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							if err := yesCheck(cCtx); err != nil {
								return wrapErrWithPressToContinue(err)
							}

							var value *bool
							if cCtx.IsSet("set") {
								value = pointer.To(cCtx.Bool("set"))
							}

							return wrapErrWithPressToContinue(errors.Wrapf(hhfctl.SwitchECMPRoCEQPN(ctx, name, value), "failed to set ecmp roce qpn"))
						},
					},
				},
			},
			{
				Name:    "connection",
				Aliases: []string{"conn"},
				Usage:   "Connection commands",
				Flags: []cli.Flag{
					verboseFlag,
				},
				Subcommands: []*cli.Command{
					{
						Name:        "get",
						Usage:       "Get connections",
						ArgsUsage:   " <type>",
						Description: "Available types: management, fabric, and vpc-loopback",
						Flags: []cli.Flag{
							verboseFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(hhfctl.ConnectionGet(ctx, &hhfctl.ConnectionGetOptions{
								Type: cCtx.Args().First(),
							}), "failed to get connections")
						},
					},
				},
			},
			{
				Name:    "switchgroup",
				Aliases: []string{"sg"},
				Usage:   "SwitchGroup commands",
				Flags: []cli.Flag{
					verboseFlag,
				},
				Subcommands: []*cli.Command{
					{
						Name:  "create",
						Usage: "Create SwitchGroup",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							printYamlFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(_ *cli.Context) error {
							return errors.Wrapf(hhfctl.SwitchGroupCreate(ctx, printYaml, &hhfctl.SwitchGroupCreateOptions{
								Name: name,
							}), "failed to create SwitchGroup")
						},
					},
				},
			},
			{
				Name:    "external",
				Aliases: []string{"ext"},
				Usage:   "External commands",
				Flags: []cli.Flag{
					verboseFlag,
				},
				Subcommands: []*cli.Command{
					{
						Name:  "create",
						Usage: "Create External",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							printYamlFlag,
							&cli.StringFlag{
								Name:    "ipv4-namespace",
								Aliases: []string{"ipns"},
								Usage:   "ipv4 namespace",
							},
							&cli.StringFlag{
								Name:    "inbound-community",
								Aliases: []string{"in"},
								Usage:   "inbound community",
							},
							&cli.StringFlag{
								Name:    "outbound-community",
								Aliases: []string{"out"},
								Usage:   "outbound community",
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(hhfctl.ExternalCreate(ctx, printYaml, &hhfctl.ExternalCreateOptions{
								Name:              name,
								IPv4Namespace:     cCtx.String("ipv4-namespace"),
								InboundCommunity:  cCtx.String("inbound-community"),
								OutboundCommunity: cCtx.String("outbound-community"),
							}), "failed to create External")
						},
					},
					{
						Name:    "peer",
						Aliases: []string{"peering"},
						Usage:   "Enable peering between external and vpc",
						Flags: []cli.Flag{
							verboseFlag,
							nameFlag,
							printYamlFlag,
							&cli.StringFlag{
								Name:  "vpc",
								Usage: "vpc name",
							},
							&cli.StringFlag{
								Name:    "external",
								Aliases: []string{"ext"},
								Usage:   "external name",
							},
							&cli.StringSliceFlag{
								Name:    "vpc-subnet",
								Aliases: []string{"subnet"},
								Usage:   "vpc subnets to enable peering for",
								Value:   cli.NewStringSlice("default"),
							},
							&cli.StringSliceFlag{
								Name:    "external-prefix",
								Aliases: []string{"prefix"},
								Usage:   "external prefixes to enable peering for, e.g. 0.0.0.0/0 for default route",
								Value:   cli.NewStringSlice("0.0.0.0/0"),
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(hhfctl.ExternalPeering(ctx, printYaml, &hhfctl.ExternalPeeringOptions{
								VPC:              cCtx.String("vpc"),
								VPCSubnets:       cCtx.StringSlice("vpc-subnet"),
								External:         cCtx.String("external"),
								ExternalPrefixes: cCtx.StringSlice("external-prefix"),
							}), "failed to enable peering between external and vpc")
						},
					},
				},
			},
			{
				Name:  "wiring",
				Usage: "general wiring diagram helpers",
				Flags: []cli.Flag{
					verboseFlag,
				},
				Subcommands: []*cli.Command{
					{
						Name:  "export",
						Usage: "export wiring diagram (incl. switches, connections, vpcs, externals, etc.)",
						Flags: []cli.Flag{
							verboseFlag,
							&cli.BoolFlag{
								Name:  "vpcs",
								Usage: "include VPCs",
								Value: true,
							},
							&cli.BoolFlag{
								Name:  "externals",
								Usage: "include Externals",
								Value: true,
							},
							&cli.BoolFlag{
								Name:  "switch-profiles",
								Usage: "include SwitchProfiles (may cause issues on importing)",
								Value: false,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(hhfctl.WiringExport(ctx, hhfctl.WiringExportOptions{
								VPCs:           cCtx.Bool("vpcs"),
								Externals:      cCtx.Bool("externals"),
								SwitchProfiles: cCtx.Bool("switch-profiles"),
							}), "failed to export wiring")
						},
					},
				},
			},
			{
				Name:    "inspect",
				Aliases: []string{"i"},
				Usage:   "Inspect Fabric API Objects and Primitives",
				Flags: []cli.Flag{
					verboseFlag,
				},
				Subcommands: []*cli.Command{
					{
						Name:  "fabric",
						Usage: "Inspect Fabric (overall control nodes and switches overview incl. status, serials, etc.)",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(_ *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.Fabric, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.FabricIn{}, os.Stdout), "failed to inspect Fabric")
						},
					},
					{
						Name:  "switch",
						Usage: "Inspect Switch (status, used ports, counters, etc.)",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringFlag{
								Name:     "name",
								Aliases:  []string{"n"},
								Usage:    "switch name",
								Required: true,
							},
							&cli.BoolFlag{
								Name:    "details",
								Aliases: []string{"d"},
								Usage:   "include detailed information (e.g. firmware versions)",
							},
							&cli.BoolFlag{
								Name:    "ports",
								Aliases: []string{"p"},
								Usage:   "include ports and breakouts information ",
							},
							&cli.BoolFlag{
								Name:    "transceivers",
								Aliases: []string{"t"},
								Usage:   "include transceivers information",
							},
							&cli.BoolFlag{
								Name:    "counters",
								Aliases: []string{"c"},
								Usage:   "include counters",
							},
							&cli.BoolFlag{
								Name:    "lasers",
								Aliases: []string{"l"},
								Usage:   "include laser details",
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.Switch, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.SwitchIn{
								Name:         cCtx.String("name"),
								Transceivers: cCtx.Bool("transceivers"),
								Ports:        cCtx.Bool("ports"),
								Details:      cCtx.Bool("details"),
								Counters:     cCtx.Bool("counters"),
								Lasers:       cCtx.Bool("lasers"),
							}, os.Stdout), "failed to inspect Switch")
						},
					},
					{
						Name:    "port",
						Aliases: []string{"switchport"},
						Usage:   "Inspect Switch Port (connection if used in one, counters, VPC and External attachments, etc.)",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringFlag{
								Name:     "name",
								Aliases:  []string{"n"},
								Usage:    "full switch port name (<switch-name>/<port-name>, e.g. 's5248-02/E1/2')",
								Required: true,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.Port, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.PortIn{
								Port: cCtx.String("name"),
							}, os.Stdout), "failed to inspect Switch Port")
						},
					},
					{
						Name:  "server",
						Usage: "Inspect Server (connection if used in one, VPC attachments, etc.)",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringFlag{
								Name:     "name",
								Aliases:  []string{"n"},
								Usage:    "server name",
								Required: true,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.Server, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.ServerIn{
								Name: cCtx.String("name"),
							}, os.Stdout), "failed to inspect Server")
						},
					},
					{
						Name:    "connection",
						Aliases: []string{"conn"},
						Usage:   "Inspect Connection (incl. VPC and External attachments, Loobpback Workaround usage, etc.)",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringFlag{
								Name:     "name",
								Aliases:  []string{"n"},
								Usage:    "connection name",
								Required: true,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.Connection, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.ConnectionIn{
								Name: cCtx.String("name"),
							}, os.Stdout), "failed to inspect Connection")
						},
					},
					{
						Name:    "vpc",
						Aliases: []string{"subnet", "vpcsubnet"},
						Usage:   "Inspect VPC/VPCSubnet (incl. where is it attached and what's reachable from it)",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringFlag{
								Name:     "name",
								Aliases:  []string{"n"},
								Usage:    "VPC name (if no subnet specified, will inspect all subnets)",
								Required: true,
							},
							&cli.StringFlag{
								Name:    "subnet",
								Aliases: []string{"s"},
								Usage:   "Subnet name (without VPC) to only inspect this subnet",
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.VPC, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.VPCIn{
								Name:   cCtx.String("name"),
								Subnet: cCtx.String("subnet"),
							}, os.Stdout), "failed to inspect VPC")
						},
					},
					{
						Name:  "bgp",
						Usage: "Inspect BGP neighbors",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringSliceFlag{
								Name:    "switch-name",
								Aliases: []string{"name", "n"},
								Usage:   "Switch names to inspect BGP neighbors for (if not specified, will inspect all switches)",
							},
							&cli.BoolFlag{
								Name:  "strict",
								Usage: "strict BGP check (will fail if any neighbor is missing, not expected or not established)",
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.BGP, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.BGPIn{
								Switches: cCtx.StringSlice("switch-name"),
								Strict:   cCtx.Bool("strict"),
							}, os.Stdout), "failed to inspect BGP")
						},
					},
					{
						Name:  "lldp",
						Usage: "Inspect LLDP neighbors",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringSliceFlag{
								Name:    "switch-name",
								Aliases: []string{"name", "n"},
								Usage:   "Switch names to inspect LLDP neighbors for (if not specified, will inspect all switches)",
							},
							&cli.BoolFlag{
								Name:  "strict",
								Usage: "strict LLDP check (will fail if any neighbor is missing or not as expected ignoring external ones)",
							},
							&cli.BoolFlag{
								Name:  "fabric",
								Usage: "include fabric neighbors (fabric, mclag-domain and vpcloopback connections)",
								Value: true,
							},
							&cli.BoolFlag{
								Name:  "external",
								Usage: "include external neighbors (external and staticexternal connections)",
								Value: true,
							},
							&cli.BoolFlag{
								Name:  "server",
								Usage: "include server neighbors (unbundled, bundled, eslag and mclag connections)",
								Value: true,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.LLDP, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.LLDPIn{
								Switches: cCtx.StringSlice("switch-name"),
								Strict:   cCtx.Bool("strict"),
								Fabric:   cCtx.Bool("fabric"),
								External: cCtx.Bool("external"),
								Server:   cCtx.Bool("server"),
							}, os.Stdout), "failed to inspect LLDP")
						},
					},
					{
						Name:  "ip",
						Usage: "Inspect IP Address (incl. IPv4Namespace, VPCSubnet and DHCPLease or External/StaticExternal usage)",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringFlag{
								Name:     "address",
								Aliases:  []string{"a", "addr"},
								Usage:    "IP address to inspect",
								Required: true,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.IP, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.IPIn{
								IP: cCtx.String("address"),
							}, os.Stdout), "failed to inspect IP address")
						},
					},
					{
						Name:  "mac",
						Usage: "Inspect MAC Address (incl. switch ports and DHCP leases)",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringFlag{
								Name:     "address",
								Aliases:  []string{"a", "addr"},
								Usage:    "MAC address",
								Required: true,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.MAC, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.MACIn{
								Value: cCtx.String("address"),
							}, os.Stdout), "failed to inspect MAC Address")
						},
					},
					{
						Name:  "access",
						Usage: "Inspect access between pair of IPs, Server names or VPCSubnets (everything except external IPs will be translated to VPCSubnets)",
						Flags: []cli.Flag{
							verboseFlag,
							outputFlag,
							&cli.StringFlag{
								Name:     "source",
								Aliases:  []string{"s", "src"},
								Usage:    "Source IP (only from VPC subnets), full VPC subnet name (<vpc-name>/<subnet-name>) or Server Name",
								Required: true,
							},
							&cli.StringFlag{
								Name:     "destination",
								Aliases:  []string{"d", "dest"},
								Usage:    "Destination IP (from VPC subnets, Externals or StaticExternals), full VPC subnet name (<vpc-name>/<subnet-name>) or Server Name",
								Required: true,
							},
						},
						Before: func(_ *cli.Context) error {
							return setupLogger(verbose)
						},
						Action: func(cCtx *cli.Context) error {
							return errors.Wrapf(inspect.Run(ctx, inspect.Access, inspect.Args{
								Verbose: verbose,
								Output:  inspect.OutputType(output),
							}, inspect.AccessIn{
								Source:      cCtx.String("source"),
								Destination: cCtx.String("destination"),
							}, os.Stdout), "failed to inspect access")
						},
					},
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		slog.Error("Failed", "err", err.Error())
		os.Exit(1) //nolint:gocritic
	}
}

func wrapErrWithPressToContinue(err error) error {
	if err == nil {
		return nil
	}

	if strings.Contains(os.Getenv("_"), "k9s") {
		slog.Error("Failed", "err", err.Error())
		slog.Warn("Press Enter to continue...")
		_, _ = fmt.Scanln()
	}

	return err
}
