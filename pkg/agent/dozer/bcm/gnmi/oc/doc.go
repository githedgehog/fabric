// Copyright 2023 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package oc

// Preparation to run the generator with new YANG files:

// Comment/remove following sections from file yang/extensions/openconfig-platform-ext.yang in section
// "augment /oc-pf:components/oc-pf:component/oc-transceiver:transceiver/oc-transceiver:state", line 148):
//  cable-length, max-port-power, max-module-power, display-name, vendor-oui, revision-compliance

// Comment/remove section "augment /oc-stp:stp/oc-stp:mstp/oc-stp:state" in yang/extensions/openconfig-spanning-tree-ext.yang:446:9

//go:generate sh -c "go run github.com/openconfig/ygot/generator -output_file ocbind.go -ignore_unsupported -generate_simple_unions -generate_fakeroot -fakeroot_name=device -package_name oc -exclude_modules ietf-interfaces -path ../yang $(find ../yang -name '*.yang' -maxdepth 2)"
