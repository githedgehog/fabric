package oc

//go:generate sh -c "go run github.com/openconfig/ygot/generator -output_file ocbind.go -ignore_unsupported -generate_simple_unions -generate_fakeroot -fakeroot_name=device -package_name oc -exclude_modules ietf-interfaces -path ../yang $(find ../yang -name '*.yang' -maxdepth 2 -not -name 'openconfig-platform-ext.yang')"
