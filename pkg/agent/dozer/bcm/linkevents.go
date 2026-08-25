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
	"encoding/json"
	"log/slog"
	"maps"
	"strconv"
	"strings"
	"sync"
	"time"

	gnmiproto "github.com/openconfig/gnmi/proto/gnmi"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric-bcm-ygot/pkg/oc"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	operStatusLeaf = "oper-status"
	lastChangeLeaf = "last-change"
	operStatusUp   = "UP"

	ifReasonEventPath = "/sonic-if-down-reason/IF_REASON_EVENT/IF_REASON_EVENT_LIST"

	// Cause lookup runs inline on the subscription callback, once per counted transition,
	// so this is how long a single lookup can stall the stream. A transition whose cause
	// cannot be read in that budget is counted as unknown rather than dropped.
	reasonLookupTimeout = 2 * time.Second
	// How far back an event row may be and still be accepted as the cause of the
	// transition being attributed. The agent runs on the switch, so these timestamps
	// share its clock.
	reasonMaxAge = time.Minute
	// How long one read of the event table is reused for. Long enough to cover a burst of
	// notifications for links that failed together, short enough that no interface can
	// plausibly go down twice inside it.
	reasonCacheTTL = 500 * time.Millisecond
	unknownReason  = "UNKNOWN"
)

// linkEventPaths are the only interface leaves this switch NOS answers an on-change
// subscription for. Counters and the enclosing state subtree are not on-change capable
// and are answered with silence, so they must not be added here.
var linkEventPaths = []string{
	"/openconfig-interfaces:interfaces/interface[name=*]/state/" + operStatusLeaf,
	"/openconfig-interfaces:interfaces/interface[name=*]/state/" + lastChangeLeaf,
}

// ifaceTransitions is the running count for one interface, keyed by NOS name. The
// per-reason counts are the whole record: their sum is the number of downs, so there is
// no separate total to keep in step with them. Admin shutdowns need no separate count
// either, since they arrive as their own ADMIN_DOWN reason.
type ifaceTransitions struct {
	reasons     map[string]uint64
	lastEvent   string
	lastReason  string
	lastEventAt time.Time

	// Values as of the last notification. An empty oper means this interface has not been
	// seen yet, so its first report establishes a starting point rather than counting.
	oper       string
	lastChange uint64

	// Values as of the previous sync-response, to detect downs that happened while
	// nothing was subscribed. See onSync.
	lastChangeAtSync uint64
	downsAtSync      uint64
	operAtSync       string
}

// linkEventSeed carries what the previous agent run persisted for one interface. The
// statuses matter as much as the counts: with them the first dump after a restart can
// tell a link that went down while the agent was away from one that never moved.
type linkEventSeed struct {
	transitions agentapi.SwitchStateInterfaceTransitions
	lastChange  uint64
	oper        string
}

// linkEventWatcher counts interface state transitions from a gNMI on-change
// subscription. Polling cannot do this job: the agent polls every 15s and the switch
// keeps no transition counter, so any number derived from polling would silently
// collapse a burst of flaps into one.
type linkEventWatcher struct {
	client GNMICClient

	mu     sync.Mutex
	synced bool
	ifaces map[string]*ifaceTransitions

	// Last IF_REASON_EVENT timestamp seen per interface and event
	reasonSeen map[string]map[string]time.Time

	// The event table as last read, shared by every down in the same burst. See reasonTable.
	reasonTableCache *oc.SonicIfDownReason_SonicIfDownReason_IF_REASON_EVENT
	reasonTableErr   error
	reasonTableAt    time.Time
	// How long that copy is reused for. A field so tests can turn coalescing off; in
	// production it is always reasonCacheTTL.
	reasonTTL time.Duration
}

func newLinkEventWatcher(client GNMICClient) *linkEventWatcher {
	return &linkEventWatcher{
		client:     client,
		ifaces:     map[string]*ifaceTransitions{},
		reasonSeen: map[string]map[string]time.Time{},
		reasonTTL:  reasonCacheTTL,
	}
}

// seed restores what the previous agent run persisted, keyed by NOS interface name. The
// per-reason counts carry over, and the recorded last-change and oper-status let the
// first sync notice downs that happened while nothing was subscribed.
func (w *linkEventWatcher) seed(seed map[string]linkEventSeed) {
	w.mu.Lock()
	defer w.mu.Unlock()

	for name, prev := range seed {
		st := &ifaceTransitions{
			reasons:          maps.Clone(prev.transitions.Reasons),
			lastEvent:        prev.transitions.LastEvent,
			lastReason:       prev.transitions.LastReason,
			lastEventAt:      prev.transitions.LastEventAt.Time,
			lastChange:       prev.lastChange,
			lastChangeAtSync: prev.lastChange,
			oper:             prev.oper,
			operAtSync:       prev.oper,
		}
		if st.reasons == nil {
			st.reasons = map[string]uint64{}
		}
		st.downsAtSync = sumCounts(st.reasons)
		w.ifaces[name] = st
	}
}

func sumCounts(counts map[string]uint64) uint64 {
	var total uint64
	for _, count := range counts {
		total += count
	}

	return total
}

// run holds the subscription until ctx is done. It only returns on a fatal error;
// transient stream failures are retried underneath and surface as another sync-response.
func (w *linkEventWatcher) run(ctx context.Context, subscribe subscribeFunc) error {
	return subscribe(ctx, linkEventPaths, func(resp *gnmiproto.SubscribeResponse) error {
		if resp.GetSyncResponse() {
			w.onSync()

			return nil
		}

		notif := resp.GetUpdate()
		if notif == nil {
			return nil
		}

		for _, upd := range notif.GetUpdate() {
			name, leaf := ifaceAndLeaf(notif.GetPrefix(), upd.GetPath())
			if name == "" || leaf == "" {
				continue
			}
			w.onUpdate(ctx, name, leaf, upd.GetVal())
		}

		return nil
	})
}

// subscribeFunc matches gnmi.Client.SubscribeOnChange.
type subscribeFunc func(ctx context.Context, paths []string, onResponse func(*gnmiproto.SubscribeResponse) error) error

// onSync marks the end of the dump of current values the server sends when a
// subscription opens. It runs once at startup and again after every reconnect, and is
// where downs that happened while nothing was subscribed are accounted for, using
// last-change as the only available evidence.
func (w *linkEventWatcher) onSync() {
	w.mu.Lock()
	defer w.mu.Unlock()

	for name, st := range w.ifaces {
		// The dump that just ended carries the values as of now. If last-change moved
		// since the previous sync while oper-status came back to where it started, the
		// link went down and up again with nothing subscribed. The exact number of downs
		// is unrecoverable, so credit the one that state proves.
		//
		// Both guards are needed. Requiring the status to be unchanged is what keeps this
		// a lower bound: a link that was down and is now up may have gone up without ever
		// going down again, so there is nothing to credit. And requiring that no down was
		// counted in between stops a down the dump already saw being counted twice.
		downs := sumCounts(st.reasons)
		if st.lastChangeAtSync != 0 && st.lastChange != st.lastChangeAtSync &&
			downs == st.downsAtSync && isUp(st.oper) == isUp(st.operAtSync) {
			st.reasons[unknownReason]++
			downs++
			slog.Debug("Link down missed while not subscribed", "interface", name, "downs", downs)
		}
		st.lastChangeAtSync = st.lastChange
		st.downsAtSync = downs
		st.operAtSync = st.oper
	}

	w.synced = true
}

func (w *linkEventWatcher) onUpdate(ctx context.Context, name, leaf string, val *gnmiproto.TypedValue) {
	w.mu.Lock()

	st := w.ifaces[name]
	if st == nil {
		st = &ifaceTransitions{reasons: map[string]uint64{}}
		w.ifaces[name] = st
	}

	down := false

	switch leaf {
	case operStatusLeaf:
		v, ok := leafString(val)
		if !ok {
			break
		}
		down = wentDown(st.oper, v)
		st.oper = v
	case lastChangeLeaf:
		// Only recorded here; onSync is what reads it, to spot downs that happened while
		// nothing was subscribed.
		if v, ok := leafUint64(val); ok {
			st.lastChange = v
		}
	}

	w.mu.Unlock()

	if down {
		w.countDown(ctx, name)
	}
}

// countDown records a down together with the cause it is charged to, in one critical
// section, so that a reader can never observe the total ahead of the breakdown that is
// supposed to partition it. The cause lookup is deliberately left outside the lock: it
// is a gNMI call, and holding the mutex across it would stall the poller.
//
// The bucket is keyed by the reason and not by the event, because the event is the
// mechanism rather than the cause: an admin shutdown also drops the PHY, so it reports
// the same phy-link-down event as a pulled cable and is only told apart by its
// ADMIN_DOWN reason.
func (w *linkEventWatcher) countDown(ctx context.Context, name string) {
	event, reason, at := w.lookupCause(ctx, name)

	w.mu.Lock()
	defer w.mu.Unlock()

	st := w.ifaces[name]
	if st == nil {
		return
	}

	if reason == "" {
		st.reasons[unknownReason]++
		st.lastEvent, st.lastReason, st.lastEventAt = "", unknownReason, time.Now()

		return
	}

	st.reasons[reason]++
	st.lastEvent = event
	st.lastReason = reason
	st.lastEventAt = at
}

// reasonTable returns the switch's interface event table, re-reading it at most once per
// reasonCacheTTL and serving every caller in between from the same copy.
//
// The read costs tens of kilobytes and cannot be narrowed to a single interface: the list
// key is (ifname, event) and the server rejects a partial one. It also runs on the
// subscription callback. Without coalescing, a fault that takes many ports down at once
// would pay for the whole table once per port, serialised, and stall the stream for as
// long as that took.
//
// The cost is that a second down on the same interface inside the window sees a table
// that predates it and is charged to UNKNOWN. A link cannot physically go down, come up
// and go down again that fast, so this only gives up accuracy the switch could not have
// given us anyway.
func (w *linkEventWatcher) reasonTable(ctx context.Context) (*oc.SonicIfDownReason_SonicIfDownReason_IF_REASON_EVENT, error) {
	w.mu.Lock()
	if !w.reasonTableAt.IsZero() && time.Since(w.reasonTableAt) < w.reasonTTL {
		cached, cachedErr := w.reasonTableCache, w.reasonTableErr
		w.mu.Unlock()

		return cached, cachedErr
	}
	w.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, reasonLookupTimeout)
	defer cancel()

	events := &oc.SonicIfDownReason_SonicIfDownReason_IF_REASON_EVENT{}
	err := w.client.Get(ctx, ifReasonEventPath, events)
	if err != nil {
		events, err = nil, errors.Wrapf(err, "reading %s", ifReasonEventPath)
	}

	w.mu.Lock()
	w.reasonTableCache, w.reasonTableErr, w.reasonTableAt = events, err, time.Now()
	w.mu.Unlock()

	return events, err
}

// lookupCause reads the switch's per-interface event table and returns the most recent
// entry that moved since the previous lookup. The table is not on-change capable, so it
// has to be pulled; it holds one row per (interface, event) carrying the last time that
// event fired, which is enough to tell which one just did.
func (w *linkEventWatcher) lookupCause(ctx context.Context, name string) (string, string, time.Time) {
	events, err := w.reasonTable(ctx)
	if err != nil {
		slog.Debug("Failed to read interface down reasons", "interface", name, "err", err)

		return "", "", time.Time{}
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	seen := w.reasonSeen[name]
	if seen == nil {
		seen = map[string]time.Time{}
		w.reasonSeen[name] = seen
	}

	// The table also holds rows from boot and from earlier faults. Without a floor the
	// first lookup after startup, when nothing has been seen yet, would charge the
	// transition to whichever old row happens to be newest.
	floor := time.Now().Add(-reasonMaxAge)

	var (
		bestEvent  string
		bestReason string
		bestAt     time.Time
	)

	for key, entry := range events.IF_REASON_EVENT_LIST {
		if key.Ifname != name || entry == nil || entry.Timestamp == nil {
			continue
		}

		at, err := time.Parse(time.RFC3339Nano, *entry.Timestamp)
		if err != nil {
			slog.Debug("Unparseable interface event timestamp", "interface", name, "event", key.Event, "ts", *entry.Timestamp)

			continue
		}

		if !at.After(seen[key.Event]) || at.Before(floor) {
			continue
		}
		seen[key.Event] = at

		// OPER_UP is a value of the down-reason enum but says the link is up, so it can
		// never be why one went down; the row for a link coming back would otherwise be
		// charged to the next down. UNSET has no name in the ygot map and renders as a
		// whole "out-of-range ..." sentence.
		if entry.Reason == oc.SonicIfDownReason_DownReason_UNSET ||
			entry.Reason == oc.SonicIfDownReason_DownReason_OPER_UP {
			continue
		}

		if at.After(bestAt) {
			bestAt, bestEvent, bestReason = at, key.Event, entry.Reason.String()
		}
	}

	return bestEvent, bestReason, bestAt
}

// snapshot returns the current counts keyed by NOS interface name. It reports nothing
// until the subscription has synced, so that an unestablished watcher reads as unknown
// rather than as zero transitions.
func (w *linkEventWatcher) snapshot() map[string]agentapi.SwitchStateInterfaceTransitions {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.synced {
		return nil
	}

	out := make(map[string]agentapi.SwitchStateInterfaceTransitions, len(w.ifaces))
	for name, st := range w.ifaces {
		t := agentapi.SwitchStateInterfaceTransitions{
			LastEvent:  st.lastEvent,
			LastReason: st.lastReason,
		}
		if len(st.reasons) > 0 {
			t.Reasons = maps.Clone(st.reasons)
		}
		if !st.lastEventAt.IsZero() {
			t.LastEventAt = kmetav1.Time{Time: st.lastEventAt}
		}
		out[name] = t
	}

	return out
}

// wentDown reports whether a status moved from up to anything else. An empty prev means
// the interface has not been seen yet, so its first report only establishes a starting
// point. Anything that is not "UP" counts as down, since oper-status has several
// not-up values (TESTING, LOWER_LAYER_DOWN, NOT_PRESENT) and a move between two of them
// is not a new down.
func wentDown(prev, next string) bool {
	return prev != "" && isUp(prev) && !isUp(next)
}

func isUp(status string) bool {
	return strings.EqualFold(status, operStatusUp)
}

// ifaceAndLeaf pulls the interface name and the updated leaf out of a notification, whose
// path is split between the notification prefix and the update itself.
func ifaceAndLeaf(prefix, path *gnmiproto.Path) (string, string) {
	elems := make([]*gnmiproto.PathElem, 0, len(prefix.GetElem())+len(path.GetElem()))
	elems = append(elems, prefix.GetElem()...)
	elems = append(elems, path.GetElem()...)

	if len(elems) == 0 {
		return "", ""
	}

	name := ""
	for _, elem := range elems {
		if elem.GetName() == "interface" {
			name = elem.GetKey()["name"]
		}
	}

	return name, elems[len(elems)-1].GetName()
}

func leafString(val *gnmiproto.TypedValue) (string, bool) {
	switch v := val.GetValue().(type) {
	case *gnmiproto.TypedValue_StringVal:
		return v.StringVal, true
	case *gnmiproto.TypedValue_JsonIetfVal:
		var s string
		if err := json.Unmarshal(v.JsonIetfVal, &s); err != nil {
			return "", false
		}

		return s, true
	default:
		return "", false
	}
}

func leafUint64(val *gnmiproto.TypedValue) (uint64, bool) {
	switch v := val.GetValue().(type) {
	case *gnmiproto.TypedValue_UintVal:
		return v.UintVal, true
	case *gnmiproto.TypedValue_IntVal:
		if v.IntVal < 0 {
			return 0, false
		}

		return uint64(v.IntVal), true
	case *gnmiproto.TypedValue_JsonIetfVal:
		var n json.Number
		if err := json.Unmarshal(v.JsonIetfVal, &n); err != nil {
			return 0, false
		}
		parsed, err := strconv.ParseUint(n.String(), 10, 64)
		if err != nil {
			return 0, false
		}

		return parsed, true
	default:
		return 0, false
	}
}

// buildLinkEventSeed turns the interface state the previous agent run persisted into
// seeds keyed by NOS interface name, which is how the subscription identifies them.
func buildLinkEventSeed(portMap map[string]string, ifaces map[string]agentapi.SwitchStateInterface) map[string]linkEventSeed {
	apiToNOS := make(map[string]string, len(portMap))
	for nosName, apiName := range portMap {
		apiToNOS[apiName] = nosName
	}

	seed := make(map[string]linkEventSeed, len(ifaces))
	for ifaceName, prev := range ifaces {
		if ifaceName == "" {
			continue
		}

		// PortChannel interfaces are reported under their own names, and so are absent
		// from the physical port mapping. Seeding them by name is what stops their counts
		// restarting at zero on every agent restart.
		nosName, exists := apiToNOS[ifaceName]
		if !exists {
			nosName = ifaceName
		}

		s := linkEventSeed{oper: string(prev.OperStatus)}
		if prev.Transitions != nil {
			s.transitions = *prev.Transitions
		}
		if !prev.LastChange.IsZero() {
			s.lastChange = uint64(prev.LastChange.Unix()) //nolint:gosec
		}
		if len(s.transitions.Reasons) == 0 && s.lastChange == 0 && s.oper == "" {
			continue
		}

		seed[nosName] = s
	}

	return seed
}
