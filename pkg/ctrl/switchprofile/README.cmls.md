# Cumulus Switch Profiles

Notes on the Cumulus switch profiles and the NVIDIA switches they describe. The notes were collected from the NVIDIA
hardware manuals and the Cumulus Linux 5.18 docs (see [Sources](#sources)) and aren't verified on actual hardware unless
stated otherwise. Per-switch notes are in the comments on the corresponding profile variables, and values in the
profiles that still need verification are marked with `TODO verify`.

## Ports and breakouts

Cumulus ports use the full NOS name in `NOSName`: `eth0` for the management port and `swpN` for the data ports, with no
`BaseNOSName`. Breakout ports are named by index, e.g. `swp5` broken out 4 ways becomes `swp5s0` to `swp5s3`, so the
offsets of a breakout mode must be exactly `"0"` to `"n-1"`. A mode with a single offset (`["0"]`) keeps the bare
`swp5` name. The API tests in `api/wiring/v1beta1/switchprofile_types_test.go` cover this naming.

When breaking out a port limits the breakouts of the port next to it, e.g. an odd port at the max breakout disables the
even one, use separate port profiles with the `-odd` and `-even` suffixes (`wiringapi.OddPortProfileSuffix` and
`wiringapi.EvenPortProfileSuffix`) and keep only the supported breakout modes in each of them. The limitation itself
isn't enforced, so describe it in the profile notes.

The port breakouts and limitations are best copied from the NVIDIA documentation, and then verified at the CLI. The
[NVIDIA switch documentation](https://docs.nvidia.com/networking/switches/index.html#nvidiatab-ethernet-switches) does
get modified by NVIDIA so it won't exist forever, sadly.

If a picture of the available breakouts isn't available, the manual process of configuring breakouts via the CLI is the
best option.

1. Start with adjacent ports, for example swp5 and swp6. Attempt the largest breakouts first. If there is a limitation
   on adjacent ports that will cut down on the options that you need to test.

   **Note:** in the past breakout limitations have been on physically adjacent ports, if there are 4 ports in a column
   on the front panel of a switch include all of those ports in the breakout testing from the previous step.

2. Once the breakout is settled, set the speed of the ports via the CLI.

## Get Platform info

This command shows the CPU, port type and count, system type, ASIC, etc.

```
cumulus@nv-sn3700c:mgmt:~$ nv show platform
               operational
-------------  -------------------------------------------------
system-mac     9c:05:91:28:4b:fd
manufacturer   Mellanox
cpu            x86_64 Intel(R) Pentium(R) CPU D1508 @ 2.20GHz x4
memory         7.28 GB
disk-size      28GB
port-layout    32 x 100G-QSFP28
part-number    MSN3700-CS2FC
serial-number  MT2249J39547
asic-model     Spectrum-2
system-uuid    6fb8ea6e-7335-11ed-8000-9c0591284b00
system-type    MSN3700C
```

The profile `Platform` is the string ONIE reports, which is shown by `onie-sysinfo -p` in ONIE. NVIDIA uses two formats
for it: `x86_64-mlnx_msnXXXX-r0` for the older switches and `x86_64-nvidia_snXXXX-r0` for the newer ones.

The Spectrum (gen 1) silicon revision is shown by `sudo decode-syseeprom version`: a `Device Version` of 16 or higher
means Spectrum A1.

## Common notes

### Auto-negotiation and FEC

- NVUE enables auto-negotiation by default on all ports.
- Since Cumulus 5.14 setting the port speed keeps auto-negotiation on and only advertises that speed, in 5.13 and
  earlier it switched the port to the force mode.
- PAM4 DAC cables and Spectrum-4 to Spectrum-4 PAM4 links require auto-negotiation. 10G DAC and optical links don't
  have auto-negotiation at all.
- With auto-negotiation off FEC has to match the peer, Spectrum-2 only supports RS FEC at 25G.

### Breakouts

- Some switches require disabling the adjacent (subsequent even) port when breaking out a port to 4x or 8x. NVUE
  disables it automatically, but the disabled port can't be used.
- Cumulus configures the number of breakout ports and their speed separately, so the profile breakout modes map to a
  breakout plus a speed.
- Since Cumulus 5.9 the 4x breakout on 8-lane ports (QSFP-DD, OSFP) uses 2 lanes per port by default. Links to peers
  on Spectrum-1/2/3 stay down unless `lanes-per-port` is set to 1.

### ECMP, RoCE and adaptive routing

- RoCE is supported on all Spectrum generations, but the RoCE config isn't portable between them.
- The ECMP hash fields don't include the RoCE queue pair, so `ECMPRoCEQPN` is false for all Cumulus switches.
- Adaptive routing is only available on Spectrum-4 (Spectrum-X) at 200G and 400G, it isn't supported on 800G links,
  subinterfaces, SVIs and bonds.

### EVPN multihoming

EVPN multihoming (`ESLAG`) is supported on Spectrum A1, Spectrum-2 and later, Spectrum A0 isn't supported. Spectrum A1
is limited to 2 switches per Ethernet segment.

### Table sizes and ACLs

Default forwarding table profile:

| Silicon     | MAC | IPv4 / IPv6 neighbors | IPv4 / IPv6 LPM |
| ----------- | --- | --------------------- | --------------- |
| Spectrum    | 40k | 32k / 16k             | 64k / 22k       |
| Spectrum-2+ | 50k | 41k / 20k             | 82k / 74k       |

ACLs on Spectrum use a single TCAM shared by ingress, egress and multicast and are limited to 1000 IPv4 and 500 IPv6
rules (non-atomic update). Spectrum-2 and later support 25000 IPv4 and 12500 IPv6 rules.

## Sources

- [Cumulus Linux 5.18 Switch Port Attributes](https://docs.nvidia.com/networking-ethernet-software/cumulus-linux-518/Layer-1-and-Switch-Ports/Interface-Configuration-and-Management/Switch-Port-Attributes/)
- [Minimum Cumulus Linux Release for Each Switch Model](https://docs.nvidia.com/networking-ethernet-software/knowledge-base/Support/Support-Offerings/Minimum-Cumulus-Linux-Release-for-Each-Switch-Model/)
- [Cumulus Linux 5.18 Forwarding Table Size and Profiles](https://docs.nvidia.com/networking-ethernet-software/cumulus-linux-518/Layer-3/Routing/Forwarding-Table-Size-and-Profiles/)
- [Cumulus Linux 5.18 EVPN Multihoming](https://docs.nvidia.com/networking-ethernet-software/cumulus-linux-518/Network-Virtualization/Ethernet-Virtual-Private-Network-EVPN/EVPN-Multihoming/)
- [Cumulus Linux 5.18 ECMP](https://docs.nvidia.com/networking-ethernet-software/cumulus-linux-518/Layer-3/Routing/Equal-Cost-Multipath-Load-Sharing/)
- [Cumulus Linux 5.18 Release Notes](https://docs.nvidia.com/networking-ethernet-software/cumulus-linux-518/Whats-New/rn/)
- [NVIDIA ONIE CLI reference](https://networking-docs.nvidia.com/onieswum/5.3.0021/cli-reference)
- [SN2201 hardware manual](https://networking-docs.nvidia.com/sn2201hw/)
- [SN2000 hardware manual](https://networking-docs.nvidia.com/sn2000hw/latest/)
- [SN3000 hardware manual](https://networking-docs.nvidia.com/sn3000hw/)
- [SN4000 hardware manual](https://networking-docs.nvidia.com/sn4000hw/)
- [SN5000 hardware manual](https://networking-docs.nvidia.com/sn5000hw/)
