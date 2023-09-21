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
				Hostname: ygot.String("switch-1"),
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
		Summary: fmt.Sprintf("PortChannel %s (%s, %s)", name, description, trunkVLANRange),
		Path:    "/interfaces/interface",
		Value: &oc.OpenconfigInterfaces_Interfaces{
			Interface: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface{
				name: {
					Name: ygot.String(name),
					Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Config{
						Name:        ygot.String(name),
						Description: ygot.String(description),
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
		Summary: fmt.Sprintf("L3 PortChannel %s (%s)", name, description),
		Path:    "/interfaces/interface",
		Value: &oc.OpenconfigInterfaces_Interfaces{
			Interface: map[string]*oc.OpenconfigInterfaces_Interfaces_Interface{
				name: {
					Name: ygot.String(name),
					Config: &oc.OpenconfigInterfaces_Interfaces_Interface_Config{
						Name:        ygot.String(name),
						Description: ygot.String(description),
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
