// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	"golang.org/x/time/rate"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const testRelay = "10.0.0.1"

// relayed makes req look like it came through the leaf relay for obj
func relayed(req *dhcpv4.DHCPv4, obj *dhcpapi.DHCPSubnet) *dhcpv4.DHCPv4 {
	req.GatewayIPAddr = net.ParseIP(testRelay)
	req.UpdateOption(dhcpv4.OptRelayAgentInfo(
		dhcpv4.OptGeneric(dhcpv4.AgentCircuitIDSubOption, []byte(obj.Spec.CircuitID)),
		dhcpv4.OptGeneric(dhcpv4.VirtualSubnetSelectionSubOption, []byte("\x00"+vrfPrefix+obj.Spec.VRF)),
	))

	return req
}

func TestHandler4NAK(t *testing.T) {
	obj := testLeaseSubnet()
	obj.Status.Allocated = map[string]dhcpapi.DHCPAllocated{mac(9): {IP: "10.0.0.100"}}
	vip := net.ParseIP("172.30.0.1").To4()
	s := &Server{
		Config:         *testCfg,
		subnets:        map[string]*subnet{subnetKeyFrom(obj): newSubnet(obj)},
		relayAllowlist: map[string]struct{}{testRelay: {}},
		controlVIP:     vip,
	}

	req := relayed(request(t, 0, "10.0.0.100", ""), obj)
	resp, err := dhcpv4.NewReplyFromRequest(req)
	require.NoError(t, err)

	handler, err := s.setupDHCP4Plugin(t.Context())()
	require.NoError(t, err)

	got, _ := handler(req, resp)
	require.NotNil(t, got)
	assert.Equal(t, dhcpv4.MessageTypeNak, got.MessageType())
	assert.True(t, got.YourIPAddr.IsUnspecified())
	assert.Equal(t, vip, got.ServerIdentifier())
	assert.Len(t, s.subnets[subnetKeyFrom(obj)].obj.Load().Status.Allocated, 1, "nothing is recorded on NAK")
}

func TestHandler4Timeout(t *testing.T) {
	cfg := *testCfg
	cfg.RequestTimeout.Duration = time.Nanosecond
	obj := testLeaseSubnet()
	s := &Server{
		Config:         cfg,
		subnets:        map[string]*subnet{subnetKeyFrom(obj): newSubnet(obj)},
		relayAllowlist: map[string]struct{}{testRelay: {}},
		controlVIP:     net.ParseIP("172.30.0.1").To4(),
	}

	handler, err := s.setupDHCP4Plugin(t.Context())()
	require.NoError(t, err)

	req := relayed(request(t, 0, "", ""), obj)
	resp, err := dhcpv4.NewReplyFromRequest(req)
	require.NoError(t, err)

	got, stop := handler(req, resp)
	assert.Nil(t, got, "request should be dropped instead of replying")
	assert.True(t, stop)
	assert.EqualValues(t, 1, s.budget.expired.Load())
}

type logRecords struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *logRecords) Enabled(context.Context, slog.Level) bool { return true }
func (h *logRecords) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *logRecords) WithGroup(string) slog.Handler            { return h }

func (h *logRecords) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.records = append(h.records, r.Clone())

	return nil
}

func (h *logRecords) take() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()

	res := h.records
	h.records = nil

	return res
}

func captureLogs(t *testing.T) *logRecords {
	t.Helper()

	h := &logRecords{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })

	return h
}

func recordAttrs(r slog.Record) map[string]string {
	res := map[string]string{}
	r.Attrs(func(a slog.Attr) bool {
		res[a.Key] = a.Value.String()

		return true
	})

	return res
}

func TestHandler4Log(t *testing.T) {
	obj := testLeaseSubnet()
	kube, sn, _ := newFakeKube(t, testLeaseSubnet(), nil)
	newServer := func(kube kclient.Client) *Server {
		return &Server{
			Config:         *testCfg,
			kube:           kube,
			subnets:        map[string]*subnet{subnetKeyFrom(obj): sn},
			relayAllowlist: map[string]struct{}{testRelay: {}},
			controlVIP:     net.ParseIP("172.30.0.1").To4(),
		}
	}
	logs := captureLogs(t)

	handle := func(t *testing.T, s *Server, req *dhcpv4.DHCPv4) (slog.Record, map[string]string) {
		t.Helper()

		handler, err := s.setupDHCP4Plugin(t.Context())()
		require.NoError(t, err)
		resp, err := dhcpv4.NewReplyFromRequest(req)
		require.NoError(t, err)

		logs.take()
		handler(req, resp)
		records := logs.take()
		require.Len(t, records, 1, "exactly one line per request")

		return records[0], recordAttrs(records[0])
	}

	s := newServer(kube)
	for _, tt := range []struct {
		name  string
		req   func(t *testing.T) *dhcpv4.DHCPv4
		msg   string
		level slog.Level
		attrs map[string]string
	}{
		{
			name:  "offer",
			req:   func(*testing.T) *dhcpv4.DHCPv4 { return req(0) },
			msg:   "Offered",
			attrs: map[string]string{"type": "discover", "subnet": "a", "ip": "10.0.0.100"},
		},
		{
			name:  "ack with a write",
			req:   func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 0, "10.0.0.100", "") },
			msg:   "Acked",
			attrs: map[string]string{"type": "request", "ip": "10.0.0.100", "written": "true"},
		},
		{
			name:  "ack covered by the stored lease",
			req:   func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 0, "", "10.0.0.100") },
			msg:   "Acked",
			attrs: map[string]string{"ip": "10.0.0.100", "written": "false"},
		},
		{
			name: "offer of another IP than requested",
			req: func(*testing.T) *dhcpv4.DHCPv4 {
				discover := req(1)
				discover.UpdateOption(dhcpv4.OptRequestedIPAddress(net.ParseIP("10.0.0.100")))

				return discover
			},
			msg:   "Offered",
			attrs: map[string]string{"ip": "10.0.0.101", "reason": "requested 10.0.0.100 used by " + mac(0)},
		},
		{
			name:  "nak",
			req:   func(t *testing.T) *dhcpv4.DHCPv4 { return request(t, 2, "10.0.0.100", "") },
			msg:   "NAKed",
			attrs: map[string]string{"subnet": "a", "reason": "requested IP unavailable: 10.0.0.100 used by " + mac(0)},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			record, attrs := handle(t, s, relayed(tt.req(t), obj))
			assert.Equal(t, tt.msg, record.Message)
			assert.Equal(t, slog.LevelInfo, record.Level)
			assert.Contains(t, attrs, "took")
			for key, value := range tt.attrs {
				assert.Equal(t, value, attrs[key], key)
			}
		})
	}

	t.Run("over the update budget", func(t *testing.T) {
		s := newServer(withUpdateBudget(kube, rate.NewLimiter(rate.Every(time.Hour), 0), &budgetStats{}))

		record, attrs := handle(t, s, relayed(request(t, 3, "", ""), obj))
		assert.Equal(t, "Dropped", record.Message)
		assert.Equal(t, slog.LevelDebug, record.Level)
		assert.Equal(t, "over the update budget", attrs["reason"])
	})

	t.Run("management subnet over a relay", func(t *testing.T) {
		s := newServer(kube)
		s.subnets = map[string]*subnet{
			subnetKey("", ""): newSubnet(&dhcpapi.DHCPSubnet{ObjectMeta: kmetav1.ObjectMeta{Name: dhcpapi.ManagementSubnet}}),
		}
		discover := req(0)
		discover.GatewayIPAddr = net.ParseIP(testRelay)

		record, attrs := handle(t, s, discover)
		assert.Equal(t, "Dropped", record.Message)
		assert.Equal(t, slog.LevelWarn, record.Level)
		assert.Equal(t, "management subnet over a relay", attrs["reason"])
		assert.Equal(t, testRelay, attrs["giaddr"])
	})

	t.Run("VPC subnet without a relay", func(t *testing.T) {
		discover := relayed(req(0), obj)
		discover.GatewayIPAddr = net.IPv4zero

		record, attrs := handle(t, newServer(kube), discover)
		assert.Equal(t, "Dropped", record.Message)
		assert.Equal(t, slog.LevelWarn, record.Level)
		assert.Equal(t, "VPC subnet without a relay", attrs["reason"])
		assert.Equal(t, "a", attrs["subnet"])
	})

	t.Run("no subnet", func(t *testing.T) {
		s := newServer(kube)
		s.subnets = map[string]*subnet{}

		record, attrs := handle(t, s, req(0))
		assert.Equal(t, "Dropped", record.Message)
		assert.Equal(t, slog.LevelInfo, record.Level)
		assert.Equal(t, "no subnet", attrs["reason"])
	})

	t.Run("unknown relay", func(t *testing.T) {
		discover := req(0)
		discover.GatewayIPAddr = net.ParseIP("192.168.99.1")

		record, attrs := handle(t, newServer(kube), discover)
		assert.Equal(t, "Dropped", record.Message)
		assert.Equal(t, slog.LevelError, record.Level)
		assert.Equal(t, "relay validation failed", attrs["reason"])
		assert.Equal(t, "192.168.99.1", attrs["giaddr"])
	})

	t.Run("duplicate subnets for a Cumulus relay", func(t *testing.T) {
		s := newServer(kube)
		s.subnets = map[string]*subnet{
			"a": newSubnet(testSubnet("a", "Vlan1000", "1")),
			"b": newSubnet(testSubnet("b", "Vlan1000", "1")),
		}
		discover := req(0)
		discover.UpdateOption(dhcpv4.OptRelayAgentInfo(
			dhcpv4.OptGeneric(dhcpv4.AgentCircuitIDSubOption, []byte("vlan1000")),
			dhcpv4.OptGeneric(dhcpv4.AgentRemoteIDSubOption, []byte("cumulus:leaf-01")),
		))

		record, attrs := handle(t, s, discover)
		assert.Equal(t, "Dropped", record.Message)
		assert.Equal(t, slog.LevelWarn, record.Level)
		assert.Contains(t, attrs["reason"], "duplicate subnets for Cumulus dhcp-relay")
	})
}

func TestHandler4Drops(t *testing.T) {
	mac, err := net.ParseMAC("0c:20:12:fe:00:01")
	require.NoError(t, err)

	for _, tt := range []struct {
		name    string
		giaddr  string
		subnets map[string]*dhcpapi.DHCPSubnet
	}{
		{
			name:   "unknown giaddr",
			giaddr: "192.168.99.1",
		},
		{
			name:   "no subnet",
			giaddr: "0.0.0.0",
		},
		{
			name:   "management subnet for a non-onie device",
			giaddr: "0.0.0.0",
			subnets: map[string]*dhcpapi.DHCPSubnet{
				subnetKey("", ""): {ObjectMeta: kmetav1.ObjectMeta{Name: dhcpapi.ManagementSubnet}},
			},
		},
		{
			name:   "management subnet over a relay",
			giaddr: testRelay,
			subnets: map[string]*dhcpapi.DHCPSubnet{
				subnetKey("", ""): {ObjectMeta: kmetav1.ObjectMeta{Name: dhcpapi.ManagementSubnet}},
			},
		},
		{
			name:   "VPC subnet without a relay",
			giaddr: "0.0.0.0",
			subnets: map[string]*dhcpapi.DHCPSubnet{
				subnetKey("", ""): testLeaseSubnet(),
			},
		},
		{
			name:   "handling error",
			giaddr: testRelay,
			subnets: map[string]*dhcpapi.DHCPSubnet{
				subnetKey("", ""): {
					ObjectMeta: kmetav1.ObjectMeta{Name: "broken"},
					Spec: dhcpapi.DHCPSubnetSpec{
						CIDRBlock: "10.0.0.0/24",
						StartIP:   "not-an-ip",
						EndIP:     "10.0.0.250",
					},
				},
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			subnets := map[string]*subnet{}
			for key, obj := range tt.subnets {
				subnets[key] = newSubnet(obj)
			}
			s := &Server{
				Config:         *testCfg,
				subnets:        subnets,
				relayAllowlist: map[string]struct{}{testRelay: {}},
				controlVIP:     net.ParseIP("172.30.0.1"),
			}

			req, err := dhcpv4.NewDiscovery(mac)
			require.NoError(t, err)
			req.GatewayIPAddr = net.ParseIP(tt.giaddr)
			resp, err := dhcpv4.NewReplyFromRequest(req)
			require.NoError(t, err)

			handler, err := s.setupDHCP4Plugin(t.Context())()
			require.NoError(t, err)

			got, stop := handler(req, resp)
			assert.Nil(t, got, "request should be dropped instead of replying")
			assert.True(t, stop)
		})
	}
}
