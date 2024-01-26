package bcm

import (
	"context"

	"github.com/openconfig/ygot/ygot"
	"github.com/pkg/errors"
	"go.githedgehog.com/fabric/pkg/agent/dozer"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi"
	"go.githedgehog.com/fabric/pkg/agent/dozer/bcm/gnmi/oc"
)

var specPortChannelConfigsEnforcer = &DefaultMapEnforcer[string, *dozer.SpecPortChannelConfig]{
	Summary:      "PortChannel Configs",
	ValueHandler: specPortChannelConfigEnforcer,
}

var specPortChannelConfigEnforcer = &DefaultValueEnforcer[string, *dozer.SpecPortChannelConfig]{
	Summary:      "PortChannel Config %s",
	Path:         "/sonic-portchannel/PORTCHANNEL/PORTCHANNEL_LIST[name=%s]/system_mac",
	UpdateWeight: ActionWeightPortChannelConfigUpdate,
	DeleteWeight: ActionWeightPortChannelConfigDelete,
	Marshal: func(key string, value *dozer.SpecPortChannelConfig) (ygot.ValidatedGoStruct, error) {
		return &oc.SonicPortchannel_SonicPortchannel_PORTCHANNEL_PORTCHANNEL_LIST{
			SystemMac: value.SystemMAC,
		}, nil
	},
}

func loadActualPortChannelConfigs(ctx context.Context, client *gnmi.Client, spec *dozer.Spec) error {
	ocPortChannel := &oc.SonicPortchannel_SonicPortchannel{}
	err := client.Get(ctx, "/sonic-portchannel/PORTCHANNEL", ocPortChannel)
	if err != nil {
		return errors.Wrapf(err, "failed to get portchannel")
	}

	spec.PortChannelConfigs, err = unmarshalActualPortChannelConfigs(ocPortChannel)
	if err != nil {
		return errors.Wrapf(err, "failed to unmarshal portchannel")
	}

	return nil
}

func unmarshalActualPortChannelConfigs(ocVal *oc.SonicPortchannel_SonicPortchannel) (map[string]*dozer.SpecPortChannelConfig, error) {
	portChannelConfigs := map[string]*dozer.SpecPortChannelConfig{}

	if ocVal == nil || ocVal.PORTCHANNEL == nil {
		return portChannelConfigs, nil
	}

	for name, portChannel := range ocVal.PORTCHANNEL.PORTCHANNEL_LIST {
		if portChannel.SystemMac == nil {
			continue
		}

		portChannelConfigs[name] = &dozer.SpecPortChannelConfig{
			SystemMAC: portChannel.SystemMac,
		}
	}

	return portChannelConfigs, nil
}
