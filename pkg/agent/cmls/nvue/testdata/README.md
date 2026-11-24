# NVUE fixtures

Responses captured from real switches, one directory per model, served by the fake nvued in the tests. A fixture is
named after its request relative to `/nvue_v1`: `/` becomes `__`, the query follows `@`, and error responses have
their status before `.json` (`interface__swp999.404.json`). `nobody/` has the responses for a user without an NVUE
role. The tests fail on a request without a fixture.

MACs (also in IPv6 link-local addresses and system UUIDs), serial numbers and system UUIDs are replaced with fakes
when capturing, the capture fails if any original value is left or a response has a secret.

## Running the tests on a switch

Every client test also runs against the real nvued on a switch, checking that the data is there on top of decoding it.
nvued answers the same 404 for a path it doesn't know as for data that doesn't apply, so running them on every new
Cumulus Linux release is what catches a renamed or removed endpoint:

```sh
just build-nvue-test
# copy bin/nvue.test to the switch, then on it
sudo NVUE_LIVE=true ./nvue.test -test.v
# the 403 needs a user without an NVUE role
sudo -u nobody NVUE_LIVE=true ./nvue.test -test.v -test.run TestGetForbidden
```

## Capturing

`captureRequests` in `capture_test.go` lists the requests to capture per model, named like the directory to capture
into. On the switch (`nobody` has to be able to write to the directory):

```sh
sudo NVUE_CAPTURE=/tmp/nvue/sn5610 ./nvue.test -test.run TestCapture
sudo chmod 777 /tmp/nvue/sn5610
sudo -u nobody NVUE_CAPTURE=/tmp/nvue/sn5610 ./nvue.test -test.run TestCapture
```

then copy the directory back and import it into `testdata`, which only adds the fixtures that are missing, such as the
ones of a new endpoint:

```sh
NVUE_IMPORT=/path/to/sn5610 just test ./pkg/agent/cmls/nvue/...
```

Refreshing the existing ones, such as for a new Cumulus Linux release, is an import with `NVUE_IMPORT_REFRESH=true`
followed by `just test-update ./pkg/agent/cmls/nvue/...`, which regenerates what the client is expected to decode from
them in `golden/`, and a review of its diff. The import fails if the status of a request changed, which usually means
that the switch isn't set up like the fixtures expect. The tests check the rest, such as the breakouts or the modules,
against the setup below.

The fakes of the scrubbed values are consistent within a capture only. The fixtures are from env-7 (Cumulus Linux
5.18.3) with the fabric configured by the agent and the VPCs of `hhfab vlab setup-vpcs`, the tests expect the switches
to have:

- sn5610: unused E1/1 and E1/3 with 800G DACs broken out 8x (disabling E1/2) and 4x, the 2x E1/63-64 spine links on
  100G DACs, a 400G optic on E1/29 and a 10G SFP optic on E1/65 to servers, nothing in E1/4, two of its four PSUs
  without power, and server-03 (E1/66) attached to the host BGP subnet of a VPC `hbgp-nvue` instead of `vpc-03`, with
  `ghcr.io/githedgehog/host-bgp` announcing an address on it
- sn3700c: a spine with the leaves over unnumbered links and the gateways over numbered ones, it only has the fixtures
  of the VRF, BGP and BFD tests
- sn4700: an ESLAG bond on a 4x 10G breakout (bond1 on E1/1/1), a 400G QSFP-DD optic (E1/9) and 100G QSFP28 DACs
- sn4600: a 4x 10G DAC breakout (E1/17) disabling E1/18, 100G QSFP28 DACs and a BGP external (`ext-bgp`) attached
- sn2700: a 4x 25G DAC breakout (E1/15) disabling E1/16, so without rates for both, and QSFP28 DACs that only report
  the cable length
- sn2201: 1G RJ45 ports up to servers (E1/1-2) and QSFP28 DACs

The `vx` fixtures are from leaf-01 of a Cumulus VX VLAB (Cumulus Linux 5.16.5), the one the CI runs, with the default
VLAB wiring, the VPCs of `hhfab vlab setup-vpcs` and the externals `ext-bgp-01` and `ext-snp-01` attached. They only
cover the switch state and what 5.16 and VX report differently: no traffic rates, BGP neighbors without their type and
peer group, and the multihop of the BFD peers as a JSON boolean. They're captured like the others, into a directory
named `vx`.
