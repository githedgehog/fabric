# testdata

## Agent plan golden tests

When adding agent yaml, make sure to clean it up before committing:
- replace status with `{}`
- set registry password to `secret` (`.spec.version.password`)
- replace o11y targets with `{}`
- do not reformat files (keep output from `kubectl get -o yaml`)
- add newlines in the end of the `in` file

Run `just test-update` to generate new expected files.
