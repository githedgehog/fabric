package oc

// Preparation to run the generator with new YANG files (at least in 4.1.1 version):
// Comment/remove following sections from file yang/extensions/openconfig-platform-ext.yang in section
// "augment /oc-pf:components/oc-pf:component/oc-transceiver:transceiver/oc-transceiver:state", line 148):
//  cable-length, max-port-power, max-module-power, display-name, vendor-oui, revision-compliance

//go:generate sh -c "go run github.com/openconfig/ygot/generator -output_file ocbind.go -ignore_unsupported -generate_simple_unions -generate_fakeroot -fakeroot_name=device -package_name oc -exclude_modules ietf-interfaces -path ../yang $(find ../yang -name '*.yang' -maxdepth 2)"
