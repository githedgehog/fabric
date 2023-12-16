package bcm

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"k8s.io/apimachinery/pkg/api/equality"
)

type ActionType string

const (
	ActionTypeUpdate  ActionType = "update"
	ActionTypeReplace ActionType = "replace"
	ActionTypeDelete  ActionType = "delete"
)

type Action struct {
	Weight         ActionWeight           `json:"weight,omitempty"`
	ASummary       string                 `json:"summary,omitempty"`
	Type           ActionType             `json:"type,omitempty"`
	Path           string                 `json:"path,omitempty"`
	Value          ygot.ValidatedGoStruct `json:"value,omitempty"`
	CustomFunc     func() error
	WarningOnError bool
}

var _ dozer.Action = (*Action)(nil)

func (a *Action) Summary() string {
	return a.ASummary
}

type ActionWeight uint8

const (
	ActionWeightUnset ActionWeight = iota // keep it first

	// Creates/Updates:

	ActionWeightSystemZTP
	ActionWeightSystemHostname
	ActionWeightLLDP
	ActionWeightUser
	ActionWeightUserAuthorizedKeys
	ActionWeightPortGroup
	ActionWeightPortBreakout

	ActionWeightPrefixListUpdate
	ActionWeightPrefixListEntryDelete
	ActionWeightPrefixListEntryUpdate
	ActionWeightCommunityListUpdate
	ActionWeightRouteMapUpdate

	ActionWeightInterfaceBaseUpdate
	ActionWeightVRFBaseUpdate
	ActionWeightInterfaceVLANIPsUpdate
	ActionWeightInterfacePortChannelUpdate
	ActionWeightInterfacePortChannelMemberUpdate
	ActionWeightInterfaceVLANAnycastGatewayUpdate
	ActionWeightInterfaceNATZoneUpdate

	ActionWeightInterfaceSubinterfaceIPsDelete
	ActionWeightVRFInterfaceDelete
	ActionWeightInterfaceSubinterfaceDelete
	ActionWeightInterfaceSubinterfaceUpdate
	ActionWeightVRFInterfaceUpdate
	ActionWeightInterfaceSubinterfaceIPsUpdate

	ActionWeightLLDPInterfaceUpdate

	ActionWeightMCLAGDomainUpdate
	ActionWeightMCLAGInterfaceUpdate

	ActionWeightACLBaseUpdate
	ActionWeightACLInterfaceUpdate
	ActionWeightACLEntryDelete
	ActionWeightACLEntryUpdate

	ActionWeightVRFBGPBaseUpdate
	ActionWeightVRFSAGUpdate
	ActionWeightVRFBGPNeighborUpdate
	ActionWeightVRFBGPNetworkUpdate
	ActionWrightVRFTableConnectionUpdate

	ActionWeightNATBaseUpdate
	ActionWeightNATPoolUpdate
	ActionWeightNATBindingUpdate
	ActionWeightNATEntryUpdate

	ActionWeightSuppressVLANNeighUpdate

	ActionWeightVRFBGPImportVRFUpdate

	ActionWeightVXLANTunnelUpdate
	ActionWeightVXLANEVPNNVOUpdate
	ActionWeightVXLANTunnelMapUpdate

	ActionWeightVRFVNIUpdate

	ActionWeightVRFStaticRouteDelete // it seems like it's better to first remove routes and then add new ones
	ActionWeightVRFStaticRouteUpdate
	ActionWeightRouteMapStatementDelete
	ActionWeightRouteMapStatementUpdate

	ActionWeightDHCPRelayUpdate

	// Deletes:

	ActionWeightDHCPRelayDelete

	ActionWeightVRFVNIDelete

	ActionWeightVXLANTunnelMapDelete
	ActionWeightVXLANEVPNNVODelete
	ActionWeightVXLANTunnelDelete

	ActionWeightVRFBGPImportVRFDelete

	ActionWeightSuppressVLANNeighDelete

	ActionWeightNATEntryDelete
	ActionWeightNATBindingDelete
	ActionWeightNATPoolDelete
	ActionWeightNATBaseDelete

	ActionWeightLLDPInterfaceDelete

	ActionWeightMCLAGInterfaceDelete
	ActionWeightMCLAGDomainDelete

	ActionWeightInterfacePortChannelMemberDelete
	ActionWeightInterfacePortChannelDelete
	ActionWeightInterfaceNATZoneDelete
	ActionWeightInterfaceVLANIPsDelete
	ActionWeightInterfaceVLANAnycastGatewayDelete

	ActionWrightVRFTableConnectionDelete
	ActionWeightVRFBGPNetworkDelete
	ActionWeightVRFBGPNeighborDelete
	ActionWeightVRFSAGDelete
	ActionWeightVRFBGPBaseDelete
	ActionWeightVRFBaseDelete

	ActionWeightACLInterfaceDelete
	ActionWeightACLBaseDelete

	ActionWeightInterfaceBaseDelete

	ActionWeightRouteMapDelete
	ActionWeightPrefixListDelete
	ActionWeightCommunityListDelete

	ActionWeightMax // keep it last
)

type ActionQueue struct {
	actions []dozer.Action
}

func (q *ActionQueue) Add(action *Action) error {
	if action.Weight > ActionWeightMax {
		return errors.Errorf("action weight %d is greater than max %d", action.Weight, ActionWeightMax)
	}
	if action.Weight == ActionWeightUnset {
		return errors.Errorf("action weight is unset")
	}

	q.actions = append(q.actions, action)

	return nil
}

func (q *ActionQueue) Sort() {
	slices.SortStableFunc(q.actions, func(action, other dozer.Action) int {
		return int(action.(*Action).Weight) - int(other.(*Action).Weight)
	})
}

type ValueEnforcer[Key comparable, Value dozer.SpecPart] interface {
	Handle(basePath string, key Key, actual, desired Value, actions *ActionQueue) error
}

type DefaultMapEnforcer[Key comparable, Value dozer.SpecPart] struct {
	Summary       string
	CustomHandler func(basePath string, actual, desired map[Key]Value, actions *ActionQueue) error
	ValueHandler  ValueEnforcer[Key, Value] // used by default map handler
}

func (h *DefaultMapEnforcer[Key, Value]) Handle(basePath string, actualMap, desiredMap map[Key]Value, actions *ActionQueue) error {
	if h.ValueHandler == nil {
		return errors.Errorf("value handler is nil for map handler %s", h.Summary)
	}
	if h.CustomHandler != nil {
		return h.CustomHandler(basePath, actualMap, desiredMap, actions)
	}

	// for each actual value in the map we want to delete it if it's not present in desired
	for key, actual := range actualMap {
		if desired, ok := desiredMap[key]; !ok {
			err := h.ValueHandler.Handle(basePath, key, actual, desired, actions)
			if err != nil {
				return errors.Wrapf(err, "error calculating delete actions for map")
			}
		}
	}

	// for each desired value in the map we want to create or update state (actual=value or nil and desired=value)
	for key, desired := range desiredMap {
		actual := actualMap[key]
		err := h.ValueHandler.Handle(basePath, key, actual, desired, actions)
		if err != nil {
			return errors.Wrapf(err, "error calculating create/update actions for map")
		}
	}

	return nil
}

type DefaultValueEnforcer[Key comparable, Value dozer.SpecPart] struct {
	Summary       string
	Skip          func(key Key, actual, desired Value) bool // skip if true
	Getter        func(key Key, value Value) any            // nil to use Value for comparision or it should return values to compart
	NoReplace     bool                                      // replace instead of update
	MutateActual  func(key Key, actual Value) Value         // Mutates actual value before comparision
	MutateDesired func(key Key, desired Value) Value        // Mutates desired value before comparision

	CustomHandler func(basePath string, key Key, actual, desired Value, actions *ActionQueue) error // will be used instead of default one

	Path             string // used by default value handler
	CreatePath       string
	PathFunc         func(key Key, value Value) string
	Marshal          func(key Key, value Value) (ygot.ValidatedGoStruct, error) // used by default value handler
	Weight           ActionWeight
	UpdateWeight     ActionWeight
	DeleteWeight     ActionWeight
	WarningOnError   bool
	SkipDelete       bool
	RecreateOnUpdate bool
}

func (h *DefaultValueEnforcer[Key, Value]) Handle(basePath string, key Key, actual, desired Value, actions *ActionQueue) error {
	if h.MutateActual != nil {
		actual = h.MutateActual(key, actual)
	}
	if h.MutateDesired != nil {
		desired = h.MutateDesired(key, desired)
	}

	var actualVal any = actual
	var desiredVal any = desired

	if h.Getter != nil {
		if !actual.IsNil() {
			actualVal = h.Getter(key, actual)
		}
		if !desired.IsNil() {
			desiredVal = h.Getter(key, desired)
		}
	}

	if equality.Semantic.DeepEqual(actualVal, desiredVal) {
		return nil
	}

	if h.Skip != nil && h.Skip(key, actual, desired) {
		return nil
	}

	if h.CustomHandler != nil {
		return h.CustomHandler(basePath, key, actual, desired, actions)
	}

	summary := SafeSprintf(h.Summary, key)

	if h.UpdateWeight == ActionWeightUnset {
		h.UpdateWeight = h.Weight
	}
	if h.DeleteWeight == ActionWeightUnset {
		h.DeleteWeight = h.Weight
	}
	if h.UpdateWeight == ActionWeightUnset {
		return errors.Errorf("update weight is unset for %s", summary)
	}
	if h.DeleteWeight == ActionWeightUnset {
		return errors.Errorf("delete weight is unset for %s", summary)
	}
	if h.UpdateWeight >= ActionWeightMax {
		return errors.Errorf("update weight %d is greater than max %d", h.UpdateWeight, ActionWeightMax)
	}
	if h.DeleteWeight >= ActionWeightMax {
		return errors.Errorf("delete weight %d is greater than max %d", h.DeleteWeight, ActionWeightMax)
	}
	if h.RecreateOnUpdate && h.UpdateWeight < h.DeleteWeight {
		// if we want to recreate on update we need to delete first
		return errors.Errorf("update weight %d is less than delete weight %d for %s but recreate on update requests", h.UpdateWeight, h.DeleteWeight, summary)
	}

	// delete actual value if desired isn't present or recreate on update requested
	if desired.IsNil() || !actual.IsNil() && h.RecreateOnUpdate {
		if h.SkipDelete {
			slog.Debug("Skipping delete", "summary", summary, "key", key)
			return nil
		}

		path := SafeSprintf(h.Path, key)
		if h.PathFunc != nil {
			path = h.PathFunc(key, actual)
		}
		path = basePath + path

		if err := actions.Add(&Action{
			Weight:         h.DeleteWeight,
			ASummary:       fmt.Sprintf("Delete %s", summary),
			Type:           ActionTypeDelete,
			Path:           path,
			WarningOnError: h.WarningOnError,
		}); err != nil {
			return errors.Wrapf(err, "failed to add delete action for %s (key %v)", summary, key)
		}
	}

	if !desired.IsNil() {
		path := SafeSprintf(h.Path, key)
		if h.PathFunc != nil {
			path = h.PathFunc(key, desired)
		}

		if actual.IsNil() || h.RecreateOnUpdate {
			summary = fmt.Sprintf("Create %s", summary)
			if h.CreatePath != "" {
				path = SafeSprintf(h.CreatePath, key)
			}
		} else {
			summary = fmt.Sprintf("Update %s", summary)
		}

		path = basePath + path

		actionType := ActionTypeUpdate

		// use replace if not creating and replacing is not disabled
		if !actual.IsNil() && !h.NoReplace && !h.RecreateOnUpdate {
			actionType = ActionTypeReplace
		}

		val, err := h.Marshal(key, desired)
		if err != nil {
			return errors.Wrapf(err, "failed to marshal %s (key %v)", summary, key)
		}

		if err := actions.Add(&Action{
			Weight:         h.UpdateWeight,
			ASummary:       summary,
			Type:           actionType,
			Path:           path,
			Value:          val,
			WarningOnError: h.WarningOnError,
		}); err != nil {
			return errors.Wrapf(err, "failed to add update action for %s (key %v)", summary, key)
		}
	}

	return nil
}

func SafeSprintf(format string, key any) string {
	if !strings.Contains(format, "%") { // TODO replace with better check and check type
		return format
	}
	return fmt.Sprintf(format, key)
}

func ValueOrNil[Value dozer.SpecPart, Result any](actual, desired Value, getter func(Value) Result) (Result, Result) {
	var res1, res2 Result

	if !actual.IsNil() {
		res1 = getter(actual)
	}
	if !desired.IsNil() {
		res2 = getter(desired)
	}

	return res1, res2
}
