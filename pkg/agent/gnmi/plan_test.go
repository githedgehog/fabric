package gnmi

import (
	"flag"
	"os"
	"reflect"
	"testing"

	"github.com/openconfig/ygot/ygot"
	"github.com/pmezard/go-difflib/difflib"
	"gopkg.in/yaml.v2"
)

var update = flag.Bool("update", false, "update the golden files of this test")

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

func TestPlanEntries(t *testing.T) {
	// TODO test agent -> processed plan?

	plan := &Plan{}
	plan.Hostname = "test"
	plan.ManagementIface = "eth0"
	plan.ManagementIP = "192.168.42.1/32"
	plan.PortGroupSpeeds = map[string]string{
		"1": "SPEED_10GB",
	}
	plan.MCLAGDomain = MCLAGDomain{
		ID:       100,
		SourceIP: "172.0.0.0/31",
		PeerIP:   "172.0.0.1/31",
		PeerLink: "PortChannel250",
		Members: []string{
			"PortChannel100",
			"PortChannel101",
		},
	}
	plan.PortChannels = []PortChannel{
		{
			ID:          100,
			Description: "test",
			Members: []string{
				"Ethernet1",
				"Ethernet2",
			},
		},
		{
			ID:          101,
			Description: "test",
			Members: []string{
				"Ethernet3",
				"Ethernet4",
			},
			TrunkVLANRange: ygot.String("2..4094"),
		},
	}
	plan.InterfaceIPs = []InterfaceIP{
		{
			Name: "Ethernet1",
			IP:   "192.168.1.1/24",
		},
	}
	plan.Users = []User{
		{
			Name:     "test",
			Password: "test",
			Role:     "admin",
		},
	}
	plan.VPCs = []VPC{
		{
			Name:      "test1",
			Subnet:    "192.168.2.1/24",
			VLAN:      100,
			DHCP:      true,
			DHCPRelay: "192.168.3.1/24",
			Peers: []string{
				"test2",
			},
		},
	}

	assertPlanGolden(t, "plan_entries.golden", plan, update)
}

func assertPlanGolden(t *testing.T, name string, plan *Plan, update *bool) {
	t.Helper()
	early, apply, err := plan.Entries()
	if err != nil {
		t.Fatal("Error generating entries for plan", err)
	}
	data, err := yaml.Marshal([][]*Entry{early, apply})
	if err != nil {
		t.Fatal("Error marshlling plan entries", err)
	}

	assertGolden(t, name, data, update, 3)
}

func assertGolden(t *testing.T, name string, actual []byte, update *bool, diffContext int) {
	t.Helper()

	file := "testdata/" + name

	if *update {
		err := os.WriteFile(file, actual, 0o644)
		if err != nil {
			t.Fatalf("Error writing golden file %s: %s", file, err)
		}
	}

	expected, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("Errir reading golden file %s: %s", file, err)
	}

	if string(expected) != string(actual) {
		diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
			A:        difflib.SplitLines(string(expected)),
			B:        difflib.SplitLines(string(actual)),
			FromFile: "Expected",
			FromDate: "",
			ToFile:   "Actual",
			ToDate:   "",
			Context:  diffContext,
		})
		if err != nil {
			t.Fatalf("Error generating diff: %s", err)
		}

		t.Fatalf("Golden file %s does not match:\n%s", name, diff)
	}
}

func TestSubnetsToRanges(t *testing.T) {
	tests := []struct {
		name    string
		subnets []string
		want    []string
	}{
		{
			name:    "empty",
			subnets: []string{},
			want:    []string{},
		},
		{
			name:    "simple",
			subnets: []string{"192.168.1.0/24"},
			want:    []string{"192.168.1.0-192.168.1.255"},
		},
		{
			name:    "single",
			subnets: []string{"192.168.1.0/32"},
			want:    []string{"192.168.1.0"},
		},
		{
			name:    "multiple",
			subnets: []string{"192.168.1.0/24", "192.168.1.0/32"},
			want:    []string{"192.168.1.0-192.168.1.255", "192.168.1.0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := subnetsToRanges(tt.subnets)
			if err != nil {
				t.Errorf("subnetsToRanges() error = %v", err)
			} else if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("subnetsToRanges() = %v, want %v", got, tt.want)
			}
		})
	}
}
