package gnmi

import (
	"fmt"
	"strings"

	"github.com/openconfig/ygot/ygot"
	"go.githedgehog.com/fabric/pkg/agent/gnmi/bcom/oc"
)

type Entry struct {
	Summary string
	Path    string
	Value   ygot.ValidatedGoStruct
}

func EntHostname(hostname string) *Entry {
	return &Entry{
		Summary: fmt.Sprintf("Hostname %s", hostname),
		Path:    "/openconfig-system:system/config",
		Value: &oc.OpenconfigSystem_System{
			Config: &oc.OpenconfigSystem_System_Config{
				Hostname: ygot.String(hostname),
			},
		},
	}
}

func EntDisableZtp() *Entry {
	return &Entry{
		Summary: "No ZTP",
		Path:    "/ztp/config",
		Value: &oc.OpenconfigZtp_Ztp{
			Config: &oc.OpenconfigZtp_Ztp_Config{
				AdminMode: ygot.Bool(false),
			},
		},
	}
}

func EntUser(username, passwdOrHash, role string, sshKey string) *Entry {
	var passwd, passwdHash *string                                         // TODO drop password support after agent generates and encodes it
	if len(passwdOrHash) == 63 && strings.HasPrefix(passwdOrHash, "$5$") { // TODO better check for hash
		passwdHash = ygot.String(passwdOrHash)
	} else {
		passwd = ygot.String(passwdOrHash)
	}

	var sshKeyVal *string
	if sshKey != "" {
		sshKeyVal = ygot.String(sshKey)
	}

	return &Entry{
		Summary: fmt.Sprintf("User %s (%s)", username, role),
		Path:    fmt.Sprintf("/openconfig-system:system/aaa/authentication/users/user[username=%s]", username),
		Value: &oc.OpenconfigSystem_System_Aaa_Authentication_Users{
			User: map[string]*oc.OpenconfigSystem_System_Aaa_Authentication_Users_User{
				username: {
					Username: ygot.String(username),
					Config: &oc.OpenconfigSystem_System_Aaa_Authentication_Users_User_Config{
						Username:       ygot.String(username),
						Password:       passwd,
						PasswordHashed: passwdHash,
						Role:           oc.UnionString(role),
						SshKey:         sshKeyVal,
					},
				},
			},
		},
	}
}

func EntPortChannel(name, description, trunkVLANRange string) *Entry {
	return &Entry{
		Summary: fmt.Sprintf("%s (%s, %s)", name, description, trunkVLANRange),
		Path:    "/interfaces/interface",
		Value: &oc.OpenconfigInterfaces_Interfaces{
			Interface: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface{
				name: {
					Name: ygot.String(name),
					Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Config{
						Name:        ygot.String(name),
						Description: ygot.String(description),
						Enabled:     ygot.Bool(true),
					},
					Aggregation: &oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation{
						Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation_Config{},
						SwitchedVlan: &oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation_SwitchedVlan{
							Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation_SwitchedVlan_Config{
								TrunkVlans: []oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation_SwitchedVlan_Config_TrunkVlans_Union{
									oc.UnionString(trunkVLANRange),
								},
							},
						},
					},
				},
			},
		},
	}
}

func EntL3PortChannel(name, description string) *Entry {
	return &Entry{
		Summary: fmt.Sprintf("%s L3 (%s)", name, description),
		Path:    "/interfaces/interface",
		Value: &oc.OpenconfigInterfaces_Interfaces{
			Interface: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface{
				name: {
					Name: ygot.String(name),
					Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Config{
						Name:        ygot.String(name),
						Description: ygot.String(description),
						Enabled:     ygot.Bool(true),
					},
					Aggregation: &oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation{
						Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Aggregation_Config{},
					},
				},
			},
		},
	}
}

func EntPortChannelMember(pChan, member string) *Entry {
	return &Entry{
		Summary: fmt.Sprintf("%s member %s", pChan, member),
		Path:    fmt.Sprintf("/interfaces/interface[name=%s]/", member),
		Value: &oc.OpenconfigInterfaces_Interfaces{
			Interface: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface{
				member: {
					Name: ygot.String(member),
					Ethernet: &oc.OpenconfigInterfaces_Interfaces_Interface_Ethernet{
						Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Ethernet_Config{
							AggregateId: ygot.String(pChan),
						},
					},
				},
			},
		},
	}
}

func EntInterfaceIP(iface, ip string, prefixLen uint8) *Entry {
	return &Entry{
		Summary: fmt.Sprintf("%s IP %s", iface, ip),
		Path:    fmt.Sprintf("/interfaces/interface[name=%s]/subinterfaces/subinterface[index=%d]/openconfig-if-ip:ipv4", iface, 0),
		Value: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface{
			Ipv4: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4{
				Addresses: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_Addresses{
					Address: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_Addresses_Address{
						ip: {
							Ip: ygot.String(ip),
							Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Subinterfaces_Subinterface_Ipv4_Addresses_Address_Config{
								Ip:           ygot.String(ip),
								PrefixLength: ygot.Uint8(prefixLen),
								Secondary:    ygot.Bool(false),
							},
						},
					},
				},
			},
		},
	}
}

func EntMCLAGDomain(id uint32, sourceIP, peerIP, peerLink string) *Entry {
	return &Entry{
		Summary: fmt.Sprintf("MCLAG domain %d", id),
		Path:    "/mclag/mclag-domains/",
		Value: &oc.OpenconfigMclag_Mclag{
			MclagDomains: &oc.OpenconfigMclag_Mclag_MclagDomains{
				MclagDomain: map[uint32]*oc.OpenconfigMclag_Mclag_MclagDomains_MclagDomain{
					id: {
						Config: &oc.OpenconfigMclag_Mclag_MclagDomains_MclagDomain_Config{
							DomainId:      ygot.Uint32(id),
							SourceAddress: ygot.String(strings.SplitN(sourceIP, "/", 2)[0]), // TODO is it good enough?
							PeerAddress:   ygot.String(strings.SplitN(peerIP, "/", 2)[0]),
							PeerLink:      ygot.String(peerLink),
						},
						DomainId: ygot.Uint32(id),
					},
				},
			},
		},
	}
}

func EntMCLAGMember(domainID uint32, member string) *Entry {
	return &Entry{
		Summary: fmt.Sprintf("MCLAG %d member %s", domainID, member),
		Path:    "/mclag/interfaces",
		Value: &oc.OpenconfigMclag_Mclag{
			Interfaces: &oc.OpenconfigMclag_Mclag_Interfaces{
				Interface: map[string]*oc.OpenconfigMclag_Mclag_Interfaces_Interface{
					member: {
						Name: ygot.String(member),
						Config: &oc.OpenconfigMclag_Mclag_Interfaces_Interface_Config{
							MclagDomainId: ygot.Uint32(domainID),
						},
					},
				},
			},
		},
	}
}

func EntVrf(vrf string) *Entry {
	return &Entry{
		Summary: vrf,
		Path:    fmt.Sprintf("/openconfig-network-instance:network-instances/network-instance[name=%s]", vrf),
		Value: &oc.OpenconfigNetworkInstance_NetworkInstances{
			NetworkInstance: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance{
				vrf: {
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Config{
						Name:    ygot.String(vrf),
						Enabled: ygot.Bool(true),
					},
					Name: ygot.String(vrf),
				},
			},
		},
	}
}

func EntVrfBGP(vrf string, bgpASN uint32, networks []string, neighbor string, remoteAS uint32) *Entry {
	summary := fmt.Sprintf("%s BGP %d", vrf, bgpASN)

	networkImportCheck := true

	var neighborsConf *oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors
	if neighbor != "" {
		summary += fmt.Sprintf(" neighbor %s", neighbor)
		networkImportCheck = false
		neighborsConf = &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors{
			Neighbor: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor{
				neighbor: {
					NeighborAddress: &neighbor,
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_Config{
						NeighborAddress: ygot.String(neighbor),
						PeerAs:          ygot.Uint32(remoteAS),
						Enabled:         ygot.Bool(true),
					},
					AfiSafis: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis{
						AfiSafi: map[oc.E_OpenconfigBgpTypes_AFI_SAFI_TYPE]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi{
							oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST: {
								AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
								Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Neighbors_Neighbor_AfiSafis_AfiSafi_Config{
									AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
									Enabled:     ygot.Bool(true),
								},
							},
						},
					},
				},
			},
		}
	}

	var netConf *oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_NetworkConfig
	if len(networks) > 0 {
		summary += fmt.Sprintf(" networks %s", strings.Join(networks, ","))
		networkImportCheck = false
		netConf = &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_NetworkConfig{
			Network: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_NetworkConfig_Network{},
		}
		for _, network := range networks {
			netConf.Network[network] = &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_NetworkConfig_Network{
				Prefix: ygot.String(network),
				Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_NetworkConfig_Network_Config{
					Prefix: ygot.String(network),
				},
			}
		}
	}

	return &Entry{
		Summary: summary,
		Path:    fmt.Sprintf("/openconfig-network-instance:network-instances/network-instance[name=%s]", vrf),
		Value: &oc.OpenconfigNetworkInstance_NetworkInstances{
			NetworkInstance: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance{
				vrf: {
					// Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Config{
					// 	Name:    ygot.String(vrf),
					// 	Enabled: ygot.Bool(true),
					// },
					Name: ygot.String(vrf),
					Protocols: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols{
						Protocol: map[oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol{
							{
								Identifier: oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
								Name:       "bgp",
							}: {
								Identifier: oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
								Name:       ygot.String("bgp"),
								Bgp: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp{
									Global: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global{
										Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_Config{
											As:                 ygot.Uint32(bgpASN),
											NetworkImportCheck: ygot.Bool(networkImportCheck),
										},
										AfiSafis: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis{
											AfiSafi: map[oc.E_OpenconfigBgpTypes_AFI_SAFI_TYPE]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi{
												oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST: {
													AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
													Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_Config{
														AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
													},
													NetworkConfig: netConf,
												},
											},
										},
									},
									Neighbors: neighborsConf,
								},
							},
						},
					},
				},
			},
		},
	}
}

func EntVLANInterface(vlanID uint16, description string) *Entry {
	vlan := fmt.Sprintf("Vlan%d", vlanID)
	return &Entry{
		Summary: fmt.Sprintf("%s (%s)", vlan, description),
		Path:    "/interfaces/interface",
		Value: &oc.OpenconfigInterfaces_Interfaces{
			Interface: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface{
				vlan: {
					Name: ygot.String(vlan),
					Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Config{
						Name:    ygot.String(vlan),
						Enabled: ygot.Bool(true),
					},
				},
			},
		},
	}
}

func EntVLANVrfMember(vrf string, vlanID uint16) *Entry {
	vlan := fmt.Sprintf("Vlan%d", vlanID)
	return EntVrfMember(vrf, vlan)
}

func EntVrfMember(vrf string, member string) *Entry {
	return &Entry{
		Summary: fmt.Sprintf("%s member %s", vrf, member),
		Path:    fmt.Sprintf("/network-instances/network-instance[name=%s]/interfaces/interface[id=%s]", vrf, member),
		Value: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces{
			Interface: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces_Interface{
				member: {
					Id: ygot.String(member),
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces_Interface_Config{
						Id: ygot.String(member),
					},
				},
			},
		},
	}
}

func EntVLANInterfaceConf(vlanID uint16, ip string, prefixLen uint8) *Entry {
	vlan := fmt.Sprintf("Vlan%d", vlanID)
	return &Entry{
		Summary: fmt.Sprintf("%s conf %s/%d", vlan, ip, prefixLen),
		Path:    fmt.Sprintf("/interfaces/interface[name=%s]", vlan),
		Value: &oc.OpenconfigInterfaces_Interfaces{
			Interface: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface{
				vlan: {
					Name: ygot.String(vlan),
					RoutedVlan: &oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan{
						Ipv4: &oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4{
							Addresses: &oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_Addresses{
								Address: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_Addresses_Address{
									ip: {
										Ip: ygot.String(ip),
										Config: &oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_Addresses_Address_Config{
											Ip:           ygot.String(ip),
											PrefixLength: ygot.Uint8(prefixLen),
										},
									},
								},
							},
							Config: &oc.OpenconfigInterfaces_Interfaces_Interface_RoutedVlan_Ipv4_Config{
								Enabled: ygot.Bool(true),
							},
						},
					},
				},
			},
		},
	}
}

func EntDHCPRelay(vlanID uint16, relayAddr, source string) *Entry {
	vlan := fmt.Sprintf("Vlan%d", vlanID)
	return &Entry{
		Summary: fmt.Sprintf("DHCP relay %s %s %s", vlan, relayAddr, source),
		Path:    fmt.Sprintf("/relay-agent/dhcp/interfaces/interface[id=%s]", vlan),
		Value: &oc.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces{
			Interface: map[string]*oc.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces_Interface{
				vlan: {
					Id: ygot.String(vlan),
					AgentInformationOption: &oc.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces_Interface_AgentInformationOption{
						Config: &oc.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces_Interface_AgentInformationOption_Config{
							LinkSelect: oc.OpenconfigRelayAgentExt_Mode_ENABLE,
							VrfSelect:  oc.OpenconfigRelayAgentExt_Mode_ENABLE,
						},
					},
					Config: &oc.OpenconfigRelayAgent_RelayAgent_Dhcp_Interfaces_Interface_Config{
						HelperAddress: []string{relayAddr},
						SrcIntf:       ygot.String(source),
					},
				},
			},
		},
	}
}

func EntInterfaceUP(iface string) *Entry {
	return &Entry{
		Summary: fmt.Sprintf("%s up", iface),
		Path:    fmt.Sprintf("/interfaces/interface[name=%s]", iface),
		Value: &oc.OpenconfigInterfaces_Interfaces{
			Interface: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface{
				iface: {
					Name: ygot.String(iface),
					Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Config{
						Name:    ygot.String(iface),
						Enabled: ygot.Bool(true),
						// TODO add description
					},
				},
			},
		},
	}
}

func EntPortGroupSpeed(group string, description string, speed oc.E_OpenconfigIfEthernet_ETHERNET_SPEED) *Entry {
	return &Entry{
		Summary: fmt.Sprintf("Port group %s speed %s (%d)", group, description, speed),
		Path:    "/openconfig-port-group:port-groups/port-group",
		Value: &oc.OpenconfigPortGroup_PortGroups{
			PortGroup: map[string]*oc.OpenconfigPortGroup_PortGroups_PortGroup{
				group: {
					Id: ygot.String(group),
					Config: &oc.OpenconfigPortGroup_PortGroups_PortGroup_Config{
						Id:    ygot.String(group),
						Speed: speed,
					},
				},
			},
		},
	}
}

func EntBGPRoutingPolicy(name string, communities []oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config_Communities_Union) *Entry {
	statement := &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_OrderedMap{}
	statement.Append(&oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement{
		Name: ygot.String("10"),
		Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Config{
			Name: ygot.String("10"),
		},
		Actions: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions{
			Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_Config{
				PolicyResult: oc.OpenconfigRoutingPolicy_PolicyResultType_ACCEPT_ROUTE,
			},
			BgpActions: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions{
				SetCommunity: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity{
					Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config{
						Method:  oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Config_Method_INLINE,
						Options: oc.OpenconfigBgpPolicy_BgpSetCommunityOptionType_ADD,
					},
					Inline: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline{
						Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements_Statement_Actions_BgpActions_SetCommunity_Inline_Config{
							Communities: communities,
						},
					},
				},
			},
		},
	})

	return &Entry{
		Summary: fmt.Sprintf("Routing policy %s", name),
		Path:    "/routing-policy/policy-definitions",
		Value: &oc.OpenconfigRoutingPolicy_RoutingPolicy{
			PolicyDefinitions: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions{
				PolicyDefinition: map[string]*oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition{
					name: {
						Name: ygot.String(name),
						Config: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Config{
							Name: ygot.String(name),
						},
						Statements: &oc.OpenconfigRoutingPolicy_RoutingPolicy_PolicyDefinitions_PolicyDefinition_Statements{
							Statement: statement,
						},
					},
				},
			},
		},
	}
}

func EntBGPRouteDistribution(vrf string, routingPolicy string) *Entry {
	val := &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections{
		TableConnection: map[oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_Key]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection{
			{
				SrcProtocol:   oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_DIRECTLY_CONNECTED,
				DstProtocol:   oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
				AddressFamily: oc.OpenconfigTypes_ADDRESS_FAMILY_IPV4,
			}: {
				AddressFamily: oc.OpenconfigTypes_ADDRESS_FAMILY_IPV4,
				SrcProtocol:   oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_DIRECTLY_CONNECTED,
				DstProtocol:   oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
				Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_Config{
					AddressFamily: oc.OpenconfigTypes_ADDRESS_FAMILY_IPV4,
					DstProtocol:   oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
					SrcProtocol:   oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_DIRECTLY_CONNECTED,
				},
			},
			{
				SrcProtocol:   oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC,
				DstProtocol:   oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
				AddressFamily: oc.OpenconfigTypes_ADDRESS_FAMILY_IPV4,
			}: {
				AddressFamily: oc.OpenconfigTypes_ADDRESS_FAMILY_IPV4,
				SrcProtocol:   oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC,
				DstProtocol:   oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
				Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_Config{
					AddressFamily: oc.OpenconfigTypes_ADDRESS_FAMILY_IPV4,
					DstProtocol:   oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
					SrcProtocol:   oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC,
				},
			},
		},
	}
	if routingPolicy != "" {
		val.TableConnection[oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_Key{
			SrcProtocol:   oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_DIRECTLY_CONNECTED,
			DstProtocol:   oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
			AddressFamily: oc.OpenconfigTypes_ADDRESS_FAMILY_IPV4,
		}].Config.ImportPolicy = []string{routingPolicy}
		val.TableConnection[oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_TableConnections_TableConnection_Key{
			SrcProtocol:   oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC,
			DstProtocol:   oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
			AddressFamily: oc.OpenconfigTypes_ADDRESS_FAMILY_IPV4,
		}].Config.ImportPolicy = []string{routingPolicy}
	}

	return &Entry{
		Summary: fmt.Sprintf("%s route distribution %s", vrf, routingPolicy),
		Path:    fmt.Sprintf("/network-instances/network-instance[name=%s]/table-connections/table-connection/", vrf),
		Value:   val,
	}
}

func EntVrfImportRoutes(vrf string, importVrfs []string) *Entry {
	// /openconfig-network-instance:network-instances/network-instance[name=VrfVvpc-1]/protocols/protocol[identifier=BGP][name=bgp]/bgp/global/afi-safis/afi-safi[afi-safi-name=IPV4_UNICAST]/openconfig-bgp-ext:import-network-instance/config/name

	return &Entry{
		Summary: fmt.Sprintf("%s import routes %s", vrf, importVrfs),
		Path:    fmt.Sprintf("/network-instances/network-instance[name=%s]/", vrf),
		Value: &oc.OpenconfigNetworkInstance_NetworkInstances{
			NetworkInstance: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance{
				vrf: {
					Name: ygot.String(vrf),
					Protocols: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols{
						Protocol: map[oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Key]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol{
							{
								Identifier: oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
								Name:       "bgp",
							}: {
								Identifier: oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_BGP,
								Name:       ygot.String("bgp"),
								Bgp: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp{
									Global: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global{
										AfiSafis: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis{
											AfiSafi: map[oc.E_OpenconfigBgpTypes_AFI_SAFI_TYPE]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi{
												oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST: {
													AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
													ImportNetworkInstance: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_ImportNetworkInstance{
														Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_ImportNetworkInstance_Config{
															Name: importVrfs,
														},
													},
													Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_Bgp_Global_AfiSafis_AfiSafi_Config{
														AfiSafiName: oc.OpenconfigBgpTypes_AFI_SAFI_TYPE_IPV4_UNICAST,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func EntInterfaceNATZone(iface string, zone uint8) *Entry {
	return &Entry{
		Summary: fmt.Sprintf("%s NAT zone %d", iface, zone),
		Path:    fmt.Sprintf("/openconfig-interfaces:interfaces/interface[name=%s]", iface),
		Value: &oc.OpenconfigInterfaces_Interfaces{
			Interface: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface{
				iface: {
					Name: ygot.String(iface),
					NatZone: &oc.OpenconfigInterfaces_Interfaces_Interface_NatZone{
						Config: &oc.OpenconfigInterfaces_Interfaces_Interface_NatZone_Config{
							NatZone: ygot.Uint8(zone),
						},
					},
				},
			},
		},
	}
}

func EntNATInstance(id uint32, zone uint8, namePrefix string, natRanges []string) *Entry {
	natPools := map[string]*oc.OpenconfigNat_Nat_Instances_Instance_NatPool_NatPoolEntry{}
	aclBindings := map[string]*oc.OpenconfigNat_Nat_Instances_Instance_NatAclPoolBinding_NatAclPoolBindingEntry{}

	for idx, natRange := range natRanges {
		natPoolName := fmt.Sprintf("%s-%d", namePrefix, idx)
		natPools[natPoolName] = &oc.OpenconfigNat_Nat_Instances_Instance_NatPool_NatPoolEntry{
			PoolName: ygot.String(natPoolName),
			Config: &oc.OpenconfigNat_Nat_Instances_Instance_NatPool_NatPoolEntry_Config{
				NatIp:    ygot.String(natRange),
				PoolName: ygot.String(natPoolName),
			},
		}

		aclBindingName := fmt.Sprintf("%s-%d", namePrefix, idx)
		aclBindings[aclBindingName] = &oc.OpenconfigNat_Nat_Instances_Instance_NatAclPoolBinding_NatAclPoolBindingEntry{
			Name: ygot.String(aclBindingName),
			Config: &oc.OpenconfigNat_Nat_Instances_Instance_NatAclPoolBinding_NatAclPoolBindingEntry_Config{
				Name:    ygot.String(aclBindingName),
				NatPool: ygot.String(natPoolName),
				Type:    oc.OpenconfigNat_NAT_TYPE_SNAT,
			},
		}
	}

	return &Entry{
		Summary: fmt.Sprintf("NAT instance %d", id),
		Path:    fmt.Sprintf("/openconfig-nat:nat/instances/instance[id=%d]", id),
		Value: &oc.OpenconfigNat_Nat_Instances{
			Instance: map[uint32]*oc.OpenconfigNat_Nat_Instances_Instance{
				id: {
					Id: ygot.Uint32(id),
					Config: &oc.OpenconfigNat_Nat_Instances_Instance_Config{
						Id:     ygot.Uint32(id),
						Enable: ygot.Bool(true),
					},
					NatPool: &oc.OpenconfigNat_Nat_Instances_Instance_NatPool{
						NatPoolEntry: natPools,
					},
					NatAclPoolBinding: &oc.OpenconfigNat_Nat_Instances_Instance_NatAclPoolBinding{
						NatAclPoolBindingEntry: aclBindings,
					},
				},
			},
		},
	}
}

func EntStaticNAT(id uint32, privateIP, externalIP string) *Entry {
	return &Entry{
		Summary: fmt.Sprintf("Static NAT %d %s <- %s", id, privateIP, externalIP),
		Path:    fmt.Sprintf("/openconfig-nat:nat/instances/instance[id=%d]", id),
		Value: &oc.OpenconfigNat_Nat_Instances{
			Instance: map[uint32]*oc.OpenconfigNat_Nat_Instances_Instance{
				id: {
					Id: ygot.Uint32(id),
					NatMappingTable: &oc.OpenconfigNat_Nat_Instances_Instance_NatMappingTable{
						NatMappingEntry: map[string]*oc.OpenconfigNat_Nat_Instances_Instance_NatMappingTable_NatMappingEntry{
							externalIP: {
								ExternalAddress: ygot.String(externalIP),
								Config: &oc.OpenconfigNat_Nat_Instances_Instance_NatMappingTable_NatMappingEntry_Config{
									ExternalAddress: ygot.String(externalIP),
									InternalAddress: ygot.String(privateIP),
								},
							},
						},
					},
				},
			},
		},
	}
}

func EntStaticRoute(vrf string, prefix, nextHop string) *Entry {
	// {"openconfig-network-instance:static-routes": {"static": [{"prefix": "0.0.0.0/0", "next-hops": {"next-hop": [{"index": "192.168.91.1", "config": {"index": "192.168.91.1", "next-hop": "192.168.91.1"}}]}}]}}

	return &Entry{
		Summary: fmt.Sprintf("%s Static route %s -> %s", vrf, prefix, nextHop),
		Path:    fmt.Sprintf("/openconfig-network-instance:network-instances/network-instance[name=%s]/protocols/protocol[identifier=STATIC][name=static]/static-routes", vrf),
		Value: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol{
			Identifier: oc.OpenconfigPolicyTypes_INSTALL_PROTOCOL_TYPE_STATIC,
			Name:       ygot.String("static"),
			StaticRoutes: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes{
				Static: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static{
					prefix: {
						Prefix: ygot.String(prefix),
						NextHops: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops{
							NextHop: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop{
								nextHop: {
									Index: ygot.String(nextHop),
									Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Protocols_Protocol_StaticRoutes_Static_NextHops_NextHop_Config{
										Index:   ygot.String(nextHop),
										NextHop: oc.UnionString(nextHop),
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
