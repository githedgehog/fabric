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

func EntUser(username, passwdOrHash, role string) *Entry {
	var passwd, passwdHash *string                                         // TODO drop password support after agent generates and encodes it
	if len(passwdOrHash) == 63 && strings.HasPrefix(passwdOrHash, "$5$") { // TODO better check for hash
		passwdHash = ygot.String(passwdOrHash)
	} else {
		passwd = ygot.String(passwdOrHash)
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
						// SshKey: // TODO
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

func EntVrf(vpc string) *Entry {
	vrf := fmt.Sprintf("Vrf%s", vpc)
	return &Entry{
		Summary: fmt.Sprintf("VRF %s", vrf),
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

func EntVLANInterface(vlanID uint16, vpc string) *Entry {
	vlan := fmt.Sprintf("Vlan%d", vlanID)
	return &Entry{
		Summary: fmt.Sprintf("%s (%s)", vlan, vpc),
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

func EntVrfMember(vpc string, vlanID uint16) *Entry {
	vrf := fmt.Sprintf("Vrf%s", vpc)
	vlan := fmt.Sprintf("Vlan%d", vlanID)
	return &Entry{
		Summary: fmt.Sprintf("%s member %s", vrf, vlan),
		Path:    fmt.Sprintf("/network-instances/network-instance[name=%s]/interfaces/interface[id=%s]", vrf, vlan),
		Value: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces{
			Interface: map[string]*oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces_Interface{
				vlan: {
					Id: ygot.String(vlan),
					Config: &oc.OpenconfigNetworkInstance_NetworkInstances_NetworkInstance_Interfaces_Interface_Config{
						Id: ygot.String(vlan),
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
