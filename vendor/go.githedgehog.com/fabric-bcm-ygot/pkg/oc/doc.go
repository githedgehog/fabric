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

//go:generate sh -c "go tool generator -output_file ocbind.go -ignore_unsupported -generate_simple_unions -generate_fakeroot -fakeroot_name=device -package_name oc -exclude_modules ietf-interfaces -path ../yang $(find ../yang -name '*.yang' -maxdepth 2) && go fmt ./..."
