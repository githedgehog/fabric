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

package bcm

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	gnmiproto "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/openconfig/gnmic/pkg/api"
	"github.com/openconfig/ygot/ygot"
	"github.com/stretchr/testify/require"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	"go.githedgehog.com/fabric/pkg/agent/switchstate"
	"go.githedgehog.com/fabric/pkg/util/pointer"
)

// reasonClient serves a canned IF_REASON_EVENT table so transition attribution can be
// tested without a switch. Timestamps are per (interface, event), as on the device.
type reasonClient struct {
	entries map[string]map[string]reasonRow
	gets    int
}

type reasonRow struct {
	reason oc.E_SonicIfDownReason_DownReason
	at     time.Time
}

var _ GNMICClient = (*reasonClient)(nil)

func (c *reasonClient) Set(context.Context, *gnmiproto.SetRequest) error { return nil }

func (c *reasonClient) CallOperation(context.Context, string, []byte) ([]byte, error) {
	return nil, nil
}

func (c *reasonClient) Get(_ context.Context, _ string, dest ygot.ValidatedGoStruct, _ ...api.GNMIOption) error {
	c.gets++

	events, ok := dest.(*oc.SonicIfDownReason_SonicIfDownReason_IF_REASON_EVENT)
	if !ok {
		return nil
	}

	events.IF_REASON_EVENT_LIST = map[oc.SonicIfDownReason_SonicIfDownReason_IF_REASON_EVENT_IF_REASON_EVENT_LIST_Key]*oc.SonicIfDownReason_SonicIfDownReason_IF_REASON_EVENT_IF_REASON_EVENT_LIST{}
	for iface, byEvent := range c.entries {
		for event, row := range byEvent {
			key := oc.SonicIfDownReason_SonicIfDownReason_IF_REASON_EVENT_IF_REASON_EVENT_LIST_Key{Ifname: iface, Event: event}
			events.IF_REASON_EVENT_LIST[key] = &oc.SonicIfDownReason_SonicIfDownReason_IF_REASON_EVENT_IF_REASON_EVENT_LIST{
				Event:     pointer.To(event),
				Ifname:    pointer.To(iface),
				Reason:    row.reason,
				Timestamp: pointer.To(row.at.Format(time.RFC3339Nano)),
			}
		}
	}

	return nil
}

// fire records that event just happened on iface, leaving it the given down reason.
func (c *reasonClient) fire(iface, event string, reason oc.E_SonicIfDownReason_DownReason, at time.Time) {
	if c.entries == nil {
		c.entries = map[string]map[string]reasonRow{}
	}
	if c.entries[iface] == nil {
		c.entries[iface] = map[string]reasonRow{}
	}
	c.entries[iface][event] = reasonRow{reason: reason, at: at}
}

// notif builds the notification shape the switch sends: the interface and the state
// container live in the prefix, the changed leaf in the update path.
func notif(iface, leaf string, val *gnmiproto.TypedValue) *gnmiproto.SubscribeResponse {
	return &gnmiproto.SubscribeResponse{
		Response: &gnmiproto.SubscribeResponse_Update{
			Update: &gnmiproto.Notification{
				Prefix: &gnmiproto.Path{Elem: []*gnmiproto.PathElem{
					{Name: "interfaces"},
					{Name: "interface", Key: map[string]string{"name": iface}},
					{Name: "state"},
				}},
				Update: []*gnmiproto.Update{{
					Path: &gnmiproto.Path{Elem: []*gnmiproto.PathElem{{Name: leaf}}},
					Val:  val,
				}},
			},
		},
	}
}

func strVal(s string) *gnmiproto.TypedValue {
	return &gnmiproto.TypedValue{Value: &gnmiproto.TypedValue_JsonIetfVal{JsonIetfVal: []byte(`"` + s + `"`)}}
}

func uintVal(v uint64) *gnmiproto.TypedValue {
	return &gnmiproto.TypedValue{Value: &gnmiproto.TypedValue_JsonIetfVal{JsonIetfVal: []byte(strconv.FormatUint(v, 10))}}
}

func syncResp() *gnmiproto.SubscribeResponse {
	return &gnmiproto.SubscribeResponse{Response: &gnmiproto.SubscribeResponse_SyncResponse{SyncResponse: true}}
}

// replay feeds a fixed sequence of responses through the watcher, as the subscription would.
func replay(t *testing.T, w *linkEventWatcher, responses ...*gnmiproto.SubscribeResponse) {
	t.Helper()

	err := w.run(t.Context(), func(ctx context.Context, _ []string, onResponse func(*gnmiproto.SubscribeResponse) error) error {
		for _, resp := range responses {
			if err := onResponse(resp); err != nil {
				return err
			}
			_ = ctx
		}

		return nil
	})
	require.NoError(t, err)
}

func TestLinkEventsBaselineIsNotCounted(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		notif("Ethernet0", lastChangeLeaf, uintVal(1000)),
		syncResp(),
	)

	got := w.snapshot()
	require.Contains(t, got, "Ethernet0")
	require.Empty(t, got["Ethernet0"].Reasons, "the initial dump must not be counted as a down")
}

func TestLinkEventsNothingReportedBeforeSync(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	replay(t, w, notif("Ethernet0", operStatusLeaf, strVal("UP")))

	require.Nil(t, w.snapshot(), "an unsynced watcher must report unknown, not zero")
}

// Only downs are counted: the two downs below are the flaps, the up between them is not
// a separate event and counting it would just double every number.
func TestLinkEventsCountsOnlyDowns(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		syncResp(),
		notif("Ethernet0", operStatusLeaf, strVal("DOWN")),
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		notif("Ethernet0", operStatusLeaf, strVal("DOWN")),
	)

	require.Equal(t, uint64(2), sumCounts(w.snapshot()["Ethernet0"].Reasons))
}

// oper-status has several not-up values, and moving between two of them is not a new
// down; only leaving UP is.
func TestLinkEventsCountsLeavingUpOnly(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		syncResp(),
		notif("Ethernet0", operStatusLeaf, strVal("DOWN")),
		notif("Ethernet0", operStatusLeaf, strVal("LOWER_LAYER_DOWN")),
		notif("Ethernet0", operStatusLeaf, strVal("NOT_PRESENT")),
	)

	require.Equal(t, uint64(1), sumCounts(w.snapshot()["Ethernet0"].Reasons))
}

func TestLinkEventsIgnoresRepeatedValue(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		syncResp(),
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
	)

	require.Zero(t, sumCounts(w.snapshot()["Ethernet0"].Reasons), "a repeated report is not a down")
}

// Admin shutdowns need no counter of their own: they arrive as an ADMIN_DOWN reason,
// which is what tells them apart from a cabling fault in the first place.
func TestLinkEventsAdminShutdownIsJustAReason(t *testing.T) {
	t.Parallel()

	client := &reasonClient{}
	w := newLinkEventWatcher(client)

	replay(t, w, notif("Ethernet0", operStatusLeaf, strVal("UP")), syncResp())

	client.fire("Ethernet0", "phy-link-down", oc.SonicIfDownReason_DownReason_ADMIN_DOWN, time.Now())
	replay(t, w, notif("Ethernet0", operStatusLeaf, strVal("DOWN")))

	require.Equal(t, map[string]uint64{"ADMIN_DOWN": 1}, w.snapshot()["Ethernet0"].Reasons)

	// Coming back up adds nothing.
	replay(t, w, notif("Ethernet0", operStatusLeaf, strVal("UP")))
	require.Equal(t, map[string]uint64{"ADMIN_DOWN": 1}, w.snapshot()["Ethernet0"].Reasons)
}

// The per-reason counts are the whole record, so each down must land in exactly one
// bucket: an admin shutdown and a pulled cable report the same event and differ only
func TestLinkEventsAttributionByReason(t *testing.T) {
	t.Parallel()

	base := time.Now().Add(-10 * time.Second)
	client := &reasonClient{}
	w := newLinkEventWatcher(client)
	w.reasonTTL = 0 // each down here stands for one seconds apart on a real link

	client.fire("Ethernet0", "phy-link-down", oc.SonicIfDownReason_DownReason_PHY_LINK_DOWN, base)
	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		syncResp(),
	)

	// A pulled cable, then the link returning.
	client.fire("Ethernet0", "phy-link-down", oc.SonicIfDownReason_DownReason_PHY_LINK_DOWN, base.Add(time.Second))
	replay(t, w, notif("Ethernet0", operStatusLeaf, strVal("DOWN")))

	client.fire("Ethernet0", "phy-link-up", oc.SonicIfDownReason_DownReason_OPER_UP, base.Add(2*time.Second))
	replay(t, w, notif("Ethernet0", operStatusLeaf, strVal("UP")))

	// An admin shutdown drops the PHY as well, so it reports the same event and is only
	// told apart by its reason. Keying the breakdown by event would merge the two.
	client.fire("Ethernet0", "phy-link-down", oc.SonicIfDownReason_DownReason_ADMIN_DOWN, base.Add(3*time.Second))
	replay(t, w, notif("Ethernet0", operStatusLeaf, strVal("DOWN")))

	got := w.snapshot()["Ethernet0"]
	require.Equal(t, uint64(2), sumCounts(got.Reasons))
	// Every bucket is a cause of failure; the link coming back is not one of them.
	require.Equal(t, map[string]uint64{"PHY_LINK_DOWN": 1, "ADMIN_DOWN": 1}, got.Reasons)
	require.Equal(t, "ADMIN_DOWN", got.LastReason)

	var sum uint64
	for _, count := range got.Reasons {
		sum += count
	}
	require.Equal(t, uint64(2), sum, "the reason counts are the record")
}

// A transition the switch reports no cause for still has to be counted, or the buckets
// would stop adding up to the total.
func TestLinkEventsUnattributedDownCounted(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		syncResp(),
		notif("Ethernet0", operStatusLeaf, strVal("DOWN")),
	)

	got := w.snapshot()["Ethernet0"]
	require.Equal(t, uint64(1), sumCounts(got.Reasons))
	require.Equal(t, map[string]uint64{unknownReason: 1}, got.Reasons)
}

// The event table keeps rows from boot and from long-past faults. Charging a transition
// to one of those would turn the breakdown into fiction, so a stale row is not a cause.
func TestLinkEventsIgnoresStaleEventRows(t *testing.T) {
	t.Parallel()

	client := &reasonClient{}
	client.fire("Ethernet0", "initialized", oc.SonicIfDownReason_DownReason_ADMIN_DOWN, time.Now().Add(-9*24*time.Hour))

	w := newLinkEventWatcher(client)
	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		syncResp(),
		notif("Ethernet0", operStatusLeaf, strVal("DOWN")),
	)

	got := w.snapshot()["Ethernet0"]
	require.Equal(t, uint64(1), sumCounts(got.Reasons))
	require.Equal(t, map[string]uint64{unknownReason: 1}, got.Reasons)
	require.Equal(t, unknownReason, got.LastReason, "a stale row must not be reported as the last reason")
}

func TestLinkEventsSeedCarriesTotalsAcrossRestart(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	w.seed(map[string]linkEventSeed{
		"Ethernet0": {
			transitions: agentapi.SwitchStateInterfaceTransitions{
				Reasons: map[string]uint64{"PHY_LINK_DOWN": 40},
			},
			lastChange: 1000,
			oper:       "up",
		},
	})

	// Same last-change as persisted: nothing happened while the agent was down.
	replay(t, w,
		notif("Ethernet0", lastChangeLeaf, uintVal(1000)),
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		syncResp(),
		notif("Ethernet0", operStatusLeaf, strVal("DOWN")),
	)

	got := w.snapshot()["Ethernet0"]
	require.Equal(t, uint64(41), sumCounts(got.Reasons), "lifetime total must carry across a restart")
}

// A restart that straddles a flap cannot recover how many there were, but last-change
// proves at least one happened, and the total must not silently omit it.
func TestLinkEventsSeedDetectsMissedDown(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	w.seed(map[string]linkEventSeed{
		"Ethernet0": {
			transitions: agentapi.SwitchStateInterfaceTransitions{
				Reasons: map[string]uint64{"PHY_LINK_DOWN": 40},
			},
			lastChange: 1000,
			oper:       "up",
		},
	})

	replay(t, w,
		notif("Ethernet0", lastChangeLeaf, uintVal(2000)),
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		syncResp(),
	)

	got := w.snapshot()["Ethernet0"]
	require.Equal(t, uint64(41), sumCounts(got.Reasons))
	require.Equal(t, uint64(1), got.Reasons[unknownReason])
}

// A reconnect re-dumps the current values, which must not be mistaken for new downs.
func TestLinkEventsReconnectDoesNotDisturbCounts(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		syncResp(),
		notif("Ethernet0", operStatusLeaf, strVal("DOWN")),
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
	)
	require.Equal(t, uint64(1), sumCounts(w.snapshot()["Ethernet0"].Reasons))

	// Reconnect: the server dumps current state again and syncs.
	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		syncResp(),
		notif("Ethernet0", operStatusLeaf, strVal("DOWN")),
	)

	require.Equal(t, uint64(2), sumCounts(w.snapshot()["Ethernet0"].Reasons))
}

// A reconnect whose dump shows a different value than we last saw proves one transition
// happened across the gap, and it must be counted exactly once.
func TestLinkEventsReconnectCountsChangedValueOnce(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		syncResp(),
	)

	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("DOWN")),
		syncResp(),
	)

	got := w.snapshot()["Ethernet0"]
	require.Equal(t, uint64(1), sumCounts(got.Reasons))
}

// Shutting a member port takes its PortChannel and the Vlans over it down too, so those
// have to be counted under their own names and attributed independently.
func TestLinkEventsTracksPortChannelsAndVlans(t *testing.T) {
	t.Parallel()

	base := time.Now().Add(-10 * time.Second)
	client := &reasonClient{}
	w := newLinkEventWatcher(client)
	w.reasonTTL = 0 // each down here stands for one seconds apart on a real link

	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		notif("PortChannel1", operStatusLeaf, strVal("UP")),
		notif("Vlan1001", operStatusLeaf, strVal("UP")),
		syncResp(),
	)

	client.fire("Ethernet0", "phy-link-down", oc.SonicIfDownReason_DownReason_ADMIN_DOWN, base.Add(time.Second))
	client.fire("PortChannel1", "all-links-down", oc.SonicIfDownReason_DownReason_ALL_LINKS_DOWN, base.Add(2*time.Second))
	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("DOWN")),
		notif("PortChannel1", operStatusLeaf, strVal("DOWN")),
		notif("Vlan1001", operStatusLeaf, strVal("DOWN")),
	)

	got := w.snapshot()
	require.Equal(t, uint64(1), sumCounts(got["Ethernet0"].Reasons))
	require.Equal(t, map[string]uint64{"ADMIN_DOWN": 1}, got["Ethernet0"].Reasons)
	require.Equal(t, uint64(1), sumCounts(got["PortChannel1"].Reasons))
	require.Equal(t, map[string]uint64{"ALL_LINKS_DOWN": 1}, got["PortChannel1"].Reasons)
	// The switch records no event for the Vlan, so its transition is still counted.
	require.Equal(t, uint64(1), sumCounts(got["Vlan1001"].Reasons))
	require.Equal(t, map[string]uint64{unknownReason: 1}, got["Vlan1001"].Reasons)
}

func TestIfaceAndLeaf(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name      string
		prefix    []*gnmiproto.PathElem
		path      []*gnmiproto.PathElem
		wantIface string
		wantLeaf  string
	}{
		{
			name:      "split between prefix and update",
			prefix:    []*gnmiproto.PathElem{{Name: "interfaces"}, {Name: "interface", Key: map[string]string{"name": "Ethernet0"}}, {Name: "state"}},
			path:      []*gnmiproto.PathElem{{Name: "oper-status"}},
			wantIface: "Ethernet0",
			wantLeaf:  "oper-status",
		},
		{
			name:      "entirely in the update path",
			path:      []*gnmiproto.PathElem{{Name: "interfaces"}, {Name: "interface", Key: map[string]string{"name": "PortChannel1"}}, {Name: "state"}, {Name: "admin-status"}},
			wantIface: "PortChannel1",
			wantLeaf:  "admin-status",
		},
		{
			name:      "no interface key",
			path:      []*gnmiproto.PathElem{{Name: "system"}, {Name: "state"}},
			wantIface: "",
			wantLeaf:  "state",
		},
		{
			name:      "empty",
			wantIface: "",
			wantLeaf:  "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			iface, leaf := ifaceAndLeaf(&gnmiproto.Path{Elem: tt.prefix}, &gnmiproto.Path{Elem: tt.path})
			require.Equal(t, tt.wantIface, iface)
			require.Equal(t, tt.wantLeaf, leaf)
		})
	}
}

func TestLeafValueDecoding(t *testing.T) {
	t.Parallel()

	assertString := func(val *gnmiproto.TypedValue, want string, wantOK bool) {
		t.Helper()
		got, ok := leafString(val)
		require.Equal(t, wantOK, ok)
		require.Equal(t, want, got)
	}
	assertUint := func(val *gnmiproto.TypedValue, want uint64, wantOK bool) {
		t.Helper()
		got, ok := leafUint64(val)
		require.Equal(t, wantOK, ok)
		require.Equal(t, want, got)
	}

	assertString(strVal("UP"), "UP", true)
	assertString(&gnmiproto.TypedValue{Value: &gnmiproto.TypedValue_StringVal{StringVal: "UP"}}, "UP", true)
	assertString(uintVal(7), "", false)

	assertUint(uintVal(1786845115), 1786845115, true)
	assertUint(&gnmiproto.TypedValue{Value: &gnmiproto.TypedValue_UintVal{UintVal: 7}}, 7, true)
	assertUint(&gnmiproto.TypedValue{Value: &gnmiproto.TypedValue_IntVal{IntVal: -1}}, 0, false)
	assertUint(strVal("nope"), 0, false)
}

func TestLinkEventsSnapshotIsACopy(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		syncResp(),
		notif("Ethernet0", operStatusLeaf, strVal("DOWN")),
	)

	got := w.snapshot()["Ethernet0"]
	got.Reasons["injected"] = 99

	require.NotContains(t, w.snapshot()["Ethernet0"].Reasons, "injected")
}

// Interfaces appear mid-stream: a Vlan comes up when a VPC is attached, a PortChannel
// when one is configured. Those must start counting like any other, not sit inert
// reporting zero until the subscription happens to reconnect.
func TestLinkEventsCountsInterfaceFirstSeenAfterSync(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		syncResp(),
		// Vlan1001 shows up only now.
		notif("Vlan1001", operStatusLeaf, strVal("UP")),
		notif("Vlan1001", operStatusLeaf, strVal("DOWN")),
		notif("Vlan1001", operStatusLeaf, strVal("UP")),
		notif("Vlan1001", operStatusLeaf, strVal("DOWN")),
	)

	got := w.snapshot()["Vlan1001"]
	require.Equal(t, uint64(2), sumCounts(got.Reasons), "first report starts it off, later downs are counted")
	require.Equal(t, uint64(2), sumCounts(got.Reasons))
}

// A flap that begins and ends inside a stream drop leaves oper-status where it started,
// so only last-change shows it happened. Counting it keeps the total a lower bound
// instead of a number that quietly omits known events.
func TestLinkEventsReconnectDetectsFlapWithUnchangedOperStatus(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		notif("Ethernet0", lastChangeLeaf, uintVal(1000)),
		syncResp(),
	)
	require.Zero(t, sumCounts(w.snapshot()["Ethernet0"].Reasons))

	// Reconnect: back UP as before, but the switch moved last-change while we were away.
	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		notif("Ethernet0", lastChangeLeaf, uintVal(2000)),
		syncResp(),
	)

	got := w.snapshot()["Ethernet0"]
	require.Equal(t, uint64(1), sumCounts(got.Reasons))
	require.Equal(t, map[string]uint64{unknownReason: 1}, got.Reasons)
}

// The counterpart: a reconnect where oper-status did change is counted once by the
// oper-status comparison, and last-change must not add a second one.
func TestLinkEventsReconnectDoesNotDoubleCount(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		notif("Ethernet0", lastChangeLeaf, uintVal(1000)),
		syncResp(),
	)

	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("DOWN")),
		notif("Ethernet0", lastChangeLeaf, uintVal(2000)),
		syncResp(),
	)

	require.Equal(t, uint64(1), sumCounts(w.snapshot()["Ethernet0"].Reasons))
}

// ygot renders an enum with no name as a whole "out-of-range ..." sentence, which would
// otherwise become a map key in the agent status and a Prometheus label value.
func TestLinkEventsUnsetReasonIsNotUsedAsALabel(t *testing.T) {
	t.Parallel()

	client := &reasonClient{}
	client.fire("Ethernet0", "phy-link-down", oc.SonicIfDownReason_DownReason_UNSET, time.Now())

	w := newLinkEventWatcher(client)
	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		syncResp(),
		notif("Ethernet0", operStatusLeaf, strVal("DOWN")),
	)

	got := w.snapshot()["Ethernet0"]
	require.Equal(t, map[string]uint64{unknownReason: 1}, got.Reasons)
	for key := range got.Reasons {
		require.NotContains(t, key, "out-of-range")
	}
}

// The watcher seeds its lifetime totals from the status of the previous run, so a poll
// that publishes a state without them would erase the history for good. One poll with
// the subscription down is enough to trigger it.
func TestUpdateLinkTransitionsKeepsTotalsWhenWatcherIsDown(t *testing.T) {
	t.Parallel()

	reg := switchstate.NewRegistry()
	reg.SaveSwitchState(&agentapi.SwitchState{
		Interfaces: map[string]agentapi.SwitchStateInterface{
			"E1/1": {Transitions: &agentapi.SwitchStateInterfaceTransitions{
				Reasons: map[string]uint64{"PHY_LINK_DOWN": 47},
			}},
		},
	})

	// No watcher: the subscription never came up, or has not synced yet.
	p := &BroadcomProcessor{}
	swState := &agentapi.SwitchState{
		Interfaces: map[string]agentapi.SwitchStateInterface{"E1/1": {}},
	}
	p.updateLinkTransitions(reg, swState, &agentapi.Agent{}, map[string]string{"Ethernet0": "E1/1"})

	got := swState.Interfaces["E1/1"].Transitions
	require.NotNil(t, got, "lifetime totals must survive a poll with no subscription")
	require.Equal(t, uint64(47), sumCounts(got.Reasons))
	require.Equal(t, map[string]uint64{"PHY_LINK_DOWN": 47}, got.Reasons)
}

// Crediting a missed down whenever last-change moved would over-count here: a link that
// was down and is now up may simply have come up, with no down in between. Counting one
// would make the total an over-estimate rather than a lower bound.
func TestLinkEventsGapEndingUpFromDownCreditsNothing(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	w.seed(map[string]linkEventSeed{
		"Ethernet0": {
			transitions: agentapi.SwitchStateInterfaceTransitions{
				Reasons: map[string]uint64{"PHY_LINK_DOWN": 40},
			},
			lastChange: 1000,
			oper:       "down",
		},
	})

	replay(t, w,
		notif("Ethernet0", lastChangeLeaf, uintVal(2000)),
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		syncResp(),
	)

	require.Equal(t, uint64(40), sumCounts(w.snapshot()["Ethernet0"].Reasons))
}

// Down before the gap and down after it means the link came up and went down again, so
// there is one down to credit even though the status looks unchanged.
func TestLinkEventsGapStayingDownCreditsOne(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	w.seed(map[string]linkEventSeed{
		"Ethernet0": {
			transitions: agentapi.SwitchStateInterfaceTransitions{
				Reasons: map[string]uint64{"PHY_LINK_DOWN": 40},
			},
			lastChange: 1000,
			oper:       "down",
		},
	})

	replay(t, w,
		notif("Ethernet0", lastChangeLeaf, uintVal(2000)),
		notif("Ethernet0", operStatusLeaf, strVal("DOWN")),
		syncResp(),
	)

	got := w.snapshot()["Ethernet0"]
	require.Equal(t, uint64(41), sumCounts(got.Reasons))
	require.Equal(t, uint64(1), got.Reasons[unknownReason])
}

// A link that went down while the agent was away is counted from the persisted status,
// not credited a second time by the gap rule.
func TestLinkEventsGapEndingDownFromUpCountsOnce(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	w.seed(map[string]linkEventSeed{
		"Ethernet0": {
			transitions: agentapi.SwitchStateInterfaceTransitions{
				Reasons: map[string]uint64{"PHY_LINK_DOWN": 40},
			},
			lastChange: 1000,
			oper:       "up",
		},
	})

	replay(t, w,
		notif("Ethernet0", lastChangeLeaf, uintVal(2000)),
		notif("Ethernet0", operStatusLeaf, strVal("DOWN")),
		syncResp(),
	)

	require.Equal(t, uint64(41), sumCounts(w.snapshot()["Ethernet0"].Reasons))
}

// A consistent seed is kept as it is.
func TestLinkEventsSeedKeepsReasons(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	w.seed(map[string]linkEventSeed{
		"Ethernet0": {
			transitions: agentapi.SwitchStateInterfaceTransitions{
				Reasons: map[string]uint64{"PHY_LINK_DOWN": 5, "ADMIN_DOWN": 2},
			},
			oper: "up",
		},
	})

	replay(t, w, notif("Ethernet0", operStatusLeaf, strVal("UP")), syncResp())

	got := w.snapshot()["Ethernet0"]
	require.Equal(t, uint64(7), sumCounts(got.Reasons))
	require.Equal(t, map[string]uint64{"PHY_LINK_DOWN": 5, "ADMIN_DOWN": 2}, got.Reasons)
}

// A link that went down while the agent was away shows up in the initial dump, and is
// counted from the status the previous run persisted.
func TestLinkEventsDownDuringDumpIsCounted(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	w.seed(map[string]linkEventSeed{
		"Ethernet0": {
			transitions: agentapi.SwitchStateInterfaceTransitions{
				Reasons: map[string]uint64{"PHY_LINK_DOWN": 5},
			},
			oper: "up",
		},
	})

	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("DOWN")), // went down while we were away
		syncResp(),
	)

	got := w.snapshot()["Ethernet0"]
	require.Equal(t, uint64(6), sumCounts(got.Reasons), "the down is real and counts")
	require.Equal(t, uint64(1), got.Reasons[unknownReason], "its cause is not in the event table any more")
}

// PortChannel and Vlan interfaces are subscribed and published under their own names,
// so seeding only the physical port mapping would restart their lifetime totals at zero
// on every agent restart, making the metric go backwards.
func TestStartLinkEventWatcherSeedsNonPhysicalInterfaces(t *testing.T) {
	t.Parallel()

	transitions := func(total uint64) *agentapi.SwitchStateInterfaceTransitions {
		return &agentapi.SwitchStateInterfaceTransitions{
			Reasons: map[string]uint64{"PHY_LINK_DOWN": total},
		}
	}

	w := newLinkEventWatcher(&reasonClient{})
	w.seed(buildLinkEventSeed(map[string]string{"Ethernet0": "E1/1"}, map[string]agentapi.SwitchStateInterface{
		"E1/1":         {OperStatus: agentapi.OperStatusUp, Transitions: transitions(3)},
		"PortChannel1": {OperStatus: agentapi.OperStatusUp, Transitions: transitions(40)},
		"Vlan1001":     {OperStatus: agentapi.OperStatusUp, Transitions: transitions(7)},
		"":             {OperStatus: agentapi.OperStatusUp, Transitions: transitions(1)},
	}))

	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		notif("PortChannel1", operStatusLeaf, strVal("UP")),
		notif("Vlan1001", operStatusLeaf, strVal("UP")),
		syncResp(),
	)

	got := w.snapshot()
	require.Equal(t, uint64(3), sumCounts(got["Ethernet0"].Reasons), "physical port seeded through the port map")
	require.Equal(t, uint64(40), sumCounts(got["PortChannel1"].Reasons), "PortChannel seeded by its own name")
	require.Equal(t, uint64(7), sumCounts(got["Vlan1001"].Reasons), "Vlan seeded by its own name")
	require.NotContains(t, got, "", "the unnamed interface is not tracked")
}

// The registry is empty on the first poll of a fresh process, so without falling back to
// the status the previous run left behind, that first write erases every lifetime total.
func TestUpdateLinkTransitionsCarriesForwardOnFirstPoll(t *testing.T) {
	t.Parallel()

	agent := &agentapi.Agent{}
	agent.Status.State.Interfaces = map[string]agentapi.SwitchStateInterface{
		"E1/1": {Transitions: &agentapi.SwitchStateInterfaceTransitions{
			Reasons: map[string]uint64{"PHY_LINK_DOWN": 47},
		}},
	}

	// Fresh registry: nothing saved yet, and no watcher synced.
	reg := switchstate.NewRegistry()
	p := &BroadcomProcessor{}
	swState := &agentapi.SwitchState{
		Interfaces: map[string]agentapi.SwitchStateInterface{"E1/1": {}},
	}
	p.updateLinkTransitions(reg, swState, agent, map[string]string{"Ethernet0": "E1/1"})

	got := swState.Interfaces["E1/1"].Transitions
	require.NotNil(t, got, "the first write of a restart must not erase the totals")
	require.Equal(t, uint64(47), sumCounts(got.Reasons))
}

// Most interfaces never go down, and an empty object per port is a real cost on a status
// this size. Absent and empty say the same thing once the breakdown is the only record.
func TestUpdateLinkTransitionsOmitsInterfacesWithNoDowns(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		notif("Ethernet1", operStatusLeaf, strVal("UP")),
		syncResp(),
		notif("Ethernet1", operStatusLeaf, strVal("DOWN")),
	)

	p := &BroadcomProcessor{linkEvents: w}
	swState := &agentapi.SwitchState{Interfaces: map[string]agentapi.SwitchStateInterface{
		"E1/1": {}, "E1/2": {},
	}}
	p.updateLinkTransitions(switchstate.NewRegistry(), swState, &agentapi.Agent{},
		map[string]string{"Ethernet0": "E1/1", "Ethernet1": "E1/2"})

	require.Nil(t, swState.Interfaces["E1/1"].Transitions, "a link that never went down publishes nothing")
	require.NotNil(t, swState.Interfaces["E1/2"].Transitions)
	require.Equal(t, uint64(1), sumCounts(swState.Interfaces["E1/2"].Transitions.Reasons))
}

// A fault that takes many ports down at once delivers their notifications in one burst.
// The event table cannot be read per interface, so without coalescing each port would
// pay for the whole table, serialised on the callback that is meant to be draining the
// stream.
func TestLinkEventsCoalescesReasonLookupsAcrossABurst(t *testing.T) {
	t.Parallel()

	base := time.Now()
	client := &reasonClient{}
	w := newLinkEventWatcher(client)

	ports := []string{"Ethernet0", "Ethernet1", "Ethernet2", "Ethernet3", "Ethernet4"}

	up := []*gnmiproto.SubscribeResponse{}
	for _, p := range ports {
		up = append(up, notif(p, operStatusLeaf, strVal("UP")))
	}
	replay(t, w, append(up, syncResp())...)

	before := client.gets

	// The whole uplink bundle drops together.
	down := []*gnmiproto.SubscribeResponse{}
	for _, p := range ports {
		client.fire(p, "phy-link-down", oc.SonicIfDownReason_DownReason_PHY_LINK_DOWN, base)
		down = append(down, notif(p, operStatusLeaf, strVal("DOWN")))
	}
	replay(t, w, down...)

	require.Equal(t, 1, client.gets-before, "one table read serves the whole burst")

	got := w.snapshot()
	for _, p := range ports {
		require.Equal(t, uint64(1), sumCounts(got[p].Reasons), p+" is still counted")
		require.Equal(t, map[string]uint64{"PHY_LINK_DOWN": 1}, got[p].Reasons, p+" is still attributed")
	}
}

// Coalescing must not turn into a stale cache: once the window passes, the next down
// reads the table again.
func TestLinkEventsRereadsReasonTableAfterTTL(t *testing.T) {
	t.Parallel()

	client := &reasonClient{}
	w := newLinkEventWatcher(client)
	w.reasonTTL = 20 * time.Millisecond

	replay(t, w, notif("Ethernet0", operStatusLeaf, strVal("UP")), syncResp())

	client.fire("Ethernet0", "phy-link-down", oc.SonicIfDownReason_DownReason_PHY_LINK_DOWN, time.Now())
	replay(t, w, notif("Ethernet0", operStatusLeaf, strVal("DOWN")))
	first := client.gets

	time.Sleep(40 * time.Millisecond)

	client.fire("Ethernet0", "phy-link-down", oc.SonicIfDownReason_DownReason_ADMIN_DOWN, time.Now())
	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		notif("Ethernet0", operStatusLeaf, strVal("DOWN")),
	)

	require.Greater(t, client.gets, first, "the table is re-read once the window has passed")
	require.Equal(t, map[string]uint64{"PHY_LINK_DOWN": 1, "ADMIN_DOWN": 1}, w.snapshot()["Ethernet0"].Reasons)
}

// The carry-forward preserves history, but it must not preserve emptiness: a status
// written before this field existed leaves an empty object per port, and resurrecting it
// on every poll would keep it there for the life of the switch.
func TestUpdateLinkTransitionsDoesNotCarryForwardEmpties(t *testing.T) {
	t.Parallel()

	reg := switchstate.NewRegistry()
	reg.SaveSwitchState(&agentapi.SwitchState{
		Interfaces: map[string]agentapi.SwitchStateInterface{
			// What an older build left behind: an empty object, and a last-event with no
			// counts backing it.
			"E1/1": {Transitions: &agentapi.SwitchStateInterfaceTransitions{}},
			"E1/2": {Transitions: &agentapi.SwitchStateInterfaceTransitions{
				LastEvent: "phy-link-down", LastReason: "ADMIN_DOWN",
			}},
			"E1/3": {Transitions: &agentapi.SwitchStateInterfaceTransitions{
				Reasons: map[string]uint64{"PHY_LINK_DOWN": 4},
			}},
		},
	})

	p := &BroadcomProcessor{}
	swState := &agentapi.SwitchState{Interfaces: map[string]agentapi.SwitchStateInterface{
		"E1/1": {}, "E1/2": {}, "E1/3": {},
	}}
	p.updateLinkTransitions(reg, swState, &agentapi.Agent{}, map[string]string{})

	require.Nil(t, swState.Interfaces["E1/1"].Transitions, "an empty object is not worth carrying")
	require.Nil(t, swState.Interfaces["E1/2"].Transitions, "nor a last-event with no downs behind it")
	require.NotNil(t, swState.Interfaces["E1/3"].Transitions, "real counts are still preserved")
}

// OPER_UP is a value of the down-reason enum, so without an explicit guard the row
// written when a link came back becomes the newest candidate and gets charged as the
// cause of the next down.
func TestLinkEventsNeverChargesADownToOperUp(t *testing.T) {
	t.Parallel()

	client := &reasonClient{}
	w := newLinkEventWatcher(client)
	w.reasonTTL = 0

	replay(t, w, notif("Ethernet0", operStatusLeaf, strVal("UP")), syncResp())

	// The link came back a moment ago, and the switch has not yet recorded why it just
	// went down again.
	client.fire("Ethernet0", "phy-link-up", oc.SonicIfDownReason_DownReason_OPER_UP, time.Now())
	replay(t, w, notif("Ethernet0", operStatusLeaf, strVal("DOWN")))

	got := w.snapshot()["Ethernet0"]
	require.Equal(t, map[string]uint64{unknownReason: 1}, got.Reasons)
	require.NotEqual(t, "OPER_UP", got.LastReason, "a link coming up is never why one went down")
}

// A value the decoder cannot read must not be mistaken for "not up", which would count a
// down that never happened and wipe the baseline for the next comparison.
func TestLinkEventsUndecodableValueIsNotADown(t *testing.T) {
	t.Parallel()

	w := newLinkEventWatcher(&reasonClient{})
	garbage := &gnmiproto.TypedValue{Value: &gnmiproto.TypedValue_JsonIetfVal{JsonIetfVal: []byte(`{"not":"a string"}`)}}

	replay(t, w,
		notif("Ethernet0", operStatusLeaf, strVal("UP")),
		syncResp(),
		notif("Ethernet0", operStatusLeaf, garbage),
	)
	require.Empty(t, w.snapshot()["Ethernet0"].Reasons, "an undecodable value is not a down")

	// The baseline survived, so a real down still counts.
	replay(t, w, notif("Ethernet0", operStatusLeaf, strVal("DOWN")))
	require.Equal(t, uint64(1), sumCounts(w.snapshot()["Ethernet0"].Reasons))
}

// The whole point of coalescing is that one burst costs one read. A failing read that is
// not cached would cost every port its own full timeout instead.
func TestLinkEventsCoalescesFailedReasonLookups(t *testing.T) {
	t.Parallel()

	client := &failingReasonClient{}
	w := newLinkEventWatcher(client)

	ports := []string{"Ethernet0", "Ethernet1", "Ethernet2", "Ethernet3"}
	up := []*gnmiproto.SubscribeResponse{}
	down := []*gnmiproto.SubscribeResponse{}
	for _, p := range ports {
		up = append(up, notif(p, operStatusLeaf, strVal("UP")))
		down = append(down, notif(p, operStatusLeaf, strVal("DOWN")))
	}
	replay(t, w, append(up, syncResp())...)

	before := client.gets
	replay(t, w, down...)

	require.Equal(t, 1, client.gets-before, "a failing read is cached like a successful one")
	for _, p := range ports {
		require.Equal(t, map[string]uint64{unknownReason: 1}, w.snapshot()[p].Reasons, p+" is still counted")
	}
}

type failingReasonClient struct{ gets int }

var _ GNMICClient = (*failingReasonClient)(nil)

func (c *failingReasonClient) Set(context.Context, *gnmiproto.SetRequest) error { return nil }

func (c *failingReasonClient) CallOperation(context.Context, string, []byte) ([]byte, error) {
	return nil, nil
}

func (c *failingReasonClient) Get(context.Context, string, ygot.ValidatedGoStruct, ...api.GNMIOption) error {
	c.gets++

	return errUnavailable
}

var errUnavailable = errors.New("gnmi unavailable")
