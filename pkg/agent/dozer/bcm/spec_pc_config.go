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
	Summary: "PortChannel Config %s",
	CustomHandler: func(basePath string, key string, actual, desired *dozer.SpecPortChannelConfig, actions *ActionQueue) error {
		if err := specPortChannelConfigSystemMACEnforcer.Handle(basePath, key, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle system mac")
		}

		if err := specPortChannelConfigFallbackEnforcer.Handle(basePath, key, actual, desired, actions); err != nil {
			return errors.Wrap(err, "failed to handle fallback")
		}

		return nil
	},
}

var specPortChannelConfigSystemMACEnforcer = &DefaultValueEnforcer[string, *dozer.SpecPortChannelConfig]{
	Summary: "PortChannel System MAC %s",
	Path:    "/sonic-portchannel/PORTCHANNEL/PORTCHANNEL_LIST[name=%s]/system_mac",
	Getter: func(key string, value *dozer.SpecPortChannelConfig) any {
		return value.SystemMAC
	},
	UpdateWeight: ActionWeightPortChannelConfigMACUpdate,
	DeleteWeight: ActionWeightPortChannelConfigMACDelete,
	Marshal: func(key string, value *dozer.SpecPortChannelConfig) (ygot.ValidatedGoStruct, error) {
		return &oc.SonicPortchannel_SonicPortchannel_PORTCHANNEL_PORTCHANNEL_LIST{
			SystemMac: value.SystemMAC,
		}, nil
	},
}

var specPortChannelConfigFallbackEnforcer = &DefaultValueEnforcer[string, *dozer.SpecPortChannelConfig]{
	Summary: "PortChannel Fallback %s",
	Path:    "/sonic-portchannel/PORTCHANNEL/PORTCHANNEL_LIST[name=%s]/fallback",
	Getter: func(key string, value *dozer.SpecPortChannelConfig) any {
		return value.Fallback
	},
	UpdateWeight: ActionWeightPortChannelConfigFallbackUpdate,
	DeleteWeight: ActionWeightPortChannelConfigFallbackDelete,
	Marshal: func(key string, value *dozer.SpecPortChannelConfig) (ygot.ValidatedGoStruct, error) {
		return &oc.SonicPortchannel_SonicPortchannel_PORTCHANNEL_PORTCHANNEL_LIST{
			Fallback: value.Fallback,
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
		if portChannel == nil || portChannel.SystemMac == nil && portChannel.Fallback == nil {
			continue
		}

		portChannelConfigs[name] = &dozer.SpecPortChannelConfig{
			SystemMAC: portChannel.SystemMac,
			Fallback:  portChannel.Fallback,
		}
	}

	return portChannelConfigs, nil
}
