package bcm

import (
	"github.com/openconfig/ygot/ygot"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
)

var specCommunityListsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecCommunityList]{
	Summary:      "Community List",
	ValueHandler: specCommunityListEnforcer,
}

var specCommunityListEnforcer = &DefaultValueEnforcer[string, *dozer.SpecCommunityList]{
	Summary:      "Community Lists %s",
	Path:         "",
	UpdateWeight: ActionWeightCommunityListUpdate,
	DeleteWeight: ActionWeightCommunityListDelete,
	Marshal: func(name string, value *dozer.SpecCommunityList) (ygot.ValidatedGoStruct, error) {
		return nil, nil // TODO
	},
}
