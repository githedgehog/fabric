# Default SwitchProfiles

This package provides a catalog of the default `SwitchProfile` objects supported by the Fabric. It'll be enforced by the
Fabric to ensure the consistency of the switch usages and configurations.

`SwitchProfile` structure is explained in the API package as part of the `SwitchProfile` object definition.

## How to add a new SwitchProfile

To add a new `SwitchProfile`, you need to change the Fabric config in the `ConfigMap` object named `fabric-config` in
the default namespace by adding `allowExtraSwitchProfiles: true` to it so it looks like this:

```yaml
apiVersion: v1
data:
  config.yaml: |
    allowExtraSwitchProfiles: true
    ...
```

### PortGroups and ports belonging to them

It's the most simple setup - just list all the ports with refs to the correct groups.

```console
admin@s5248-01:~$ sonic-cli -c "show port-group | no-more"
-------------------------------------------------------------------------------------
Port-group  Interface range            Valid speeds      Default Speed Current Speed
-------------------------------------------------------------------------------------
1           Ethernet0 - Ethernet3      10G, 25G          25G           10G
2           Ethernet4 - Ethernet7      10G, 25G          25G           10G
3           Ethernet8 - Ethernet11     10G, 25G          25G           25G
4           Ethernet12 - Ethernet15    10G, 25G          25G           25G
5           Ethernet16 - Ethernet19    10G, 25G          25G           25G
6           Ethernet20 - Ethernet23    10G, 25G          25G           25G
7           Ethernet24 - Ethernet27    10G, 25G          25G           25G
8           Ethernet28 - Ethernet31    10G, 25G          25G           25G
9           Ethernet32 - Ethernet35    10G, 25G          25G           25G
10          Ethernet36 - Ethernet39    10G, 25G          25G           25G
11          Ethernet40 - Ethernet43    10G, 25G          25G           25G
12          Ethernet44 - Ethernet47    10G, 25G          25G           25G
```

### Breakout modes supported by the switch

For the breakouts. offsets need to be specified, it could be done by checking every breakout mode supported by the port
and checking the list of the resulting interface names (using `show interface breakout`).

In case of breakouts, `NOSName` is the actual name to enable breakout mode on the switch (e.g. `1/1` in SONiC), and the
`BaseNOSName` is the NOS name of the base port to be used together with the offset to calculate the actual port name.

```console
admin@s5248-01:~$ sonic-cli -c "show interface breakout modes | no-more"
------------------------------------------------------------------------------
Port  Pipe  Interface   Supported Modes                           Default Mode
------------------------------------------------------------------------------
1/49  N/A   Ethernet48  1x100G, 1x40G, 2x50G, 1x50G, 4x25G,       1x100G
                        4x10G, 1x25G, 1x10G
1/50  N/A   Ethernet52  1x100G, 1x40G, 2x50G, 1x50G, 4x25G,       1x100G
                        4x10G, 1x25G, 1x10G
1/51  N/A   Ethernet56  1x100G, 1x40G, 2x50G, 1x50G, 4x25G,       1x100G
                        4x10G, 1x25G, 1x10G
1/52  N/A   Ethernet60  1x100G, 1x40G, 2x50G, 1x50G, 4x25G,       1x100G
                        4x10G, 1x25G, 1x10G
1/53  N/A   Ethernet64  1x100G, 1x40G, 2x50G, 1x50G, 4x25G,       1x100G
                        4x10G, 1x25G, 1x10G
1/54  N/A   Ethernet68  1x100G, 1x40G, 2x50G, 1x50G, 4x25G,       1x100G
                        4x10G, 1x25G, 1x10G
1/55  N/A   Ethernet72  1x100G, 1x40G, 2x50G, 1x50G, 4x25G,       1x100G
                        4x10G, 1x25G, 1x10G
1/56  N/A   Ethernet76  1x100G, 1x40G, 2x50G, 1x50G, 4x25G,       1x100G
                        4x10G, 1x25G, 1x10G
```

```console
s5248-01# show interface breakout
-----------------------------------------------
Port  Breakout Mode  Status        Interfaces
-----------------------------------------------
1/55  4x25G          Completed     Ethernet72
                                   Ethernet73
                                   Ethernet74
                                   Ethernet75
```

### Standalone ports without breakout support

For some ports, there are no breakouts and port groups, e.g. Ethernet48-52 on Accton-AS4630-54NPE. In this case, it
seems like there is no easy way to determine the supported port speed and the default one. It could be done by trying to
set port speed on such port going through the full list of accepted values and checking which ones will fail.

```console
as4630-01(config-if-Ethernet48)# speed
  <10/100/1000/2500/5000/10000/20000/25000/40000/50000/100000/200000/400000>  Speed config of the interface
  auto                                                                        Enable auto-negotiation

as4630-01(config-if-Ethernet48)# speed 400000
%Error: Unsupported speed
```
