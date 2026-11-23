# NVUE fixtures

Responses captured from real switches, one directory per model, served by the fake nvued in the tests. A fixture is
named after its request relative to `/nvue_v1`: `/` becomes `__`, the query follows `@`, and error responses have
their status before `.json` (`interface__swp999.404.json`). `nobody/` has the responses for a user without an NVUE
role. The tests fail on a request without a fixture.

MACs (also in IPv6 link-local addresses and system UUIDs), serial numbers and system UUIDs are replaced with fakes
when capturing, the capture fails if any original value is left or a response has a secret.

## Running the tests on a switch

Every client test also runs against the real nvued on a switch:

```sh
just build-nvue-test
# copy bin/nvue.test to the switch, then on it
sudo NVUE_LIVE=true ./nvue.test -test.v
# the 403 needs a user without an NVUE role
sudo -u nobody NVUE_LIVE=true ./nvue.test -test.v -test.run TestGetForbidden
```

## Capturing

`captureRequests` in `capture_test.go` lists the requests to capture, on the switch (`nobody` has to be able to write
to the directory):

```sh
sudo NVUE_CAPTURE=/tmp/nvue/sn5610 ./nvue.test -test.run TestCapture
sudo chmod 777 /tmp/nvue/sn5610
sudo -u nobody NVUE_CAPTURE=/tmp/nvue/sn5610 ./nvue.test -test.run TestCapture
```

then copy the directory over the one in `testdata`.
