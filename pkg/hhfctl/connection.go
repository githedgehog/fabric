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

package hhfctl

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/pkg/errors"
)

type ConnectionGetOptions struct {
	Type string
}

func ConnectionGet(ctx context.Context, options *ConnectionGetOptions) error {
	if options.Type == "" {
		return errors.Errorf("type is required")
	}

	if options.Type != "" {
		options.Type = strings.ToLower(options.Type)
		if options.Type == "mgmt" {
			options.Type = "management"
		}
		if options.Type == "loop" {
			options.Type = "vpc-loopback"
		}

		var columns []string

		if options.Type == "management" {
			columns = []string{
				"-o", "custom-columns=" +
					"NAME:.metadata.name,GEN:.metadata.generation," +
					"SERVERPORT:.spec.management.link.server.port," +
					"SERVERIP:.spec.management.link.server.ip," +
					"SWITCHPORT:.spec.management.link.switch.port," +
					"SWITCHIP:.spec.management.link.switch.ip," +
					"ONIEPORT:.spec.management.link.switch.oniePortName",
			}
		}
		if options.Type == "fabric" {
			columns = []string{
				"-o", "custom-columns=" +
					"NAME:.metadata.name," +
					"GEN:.metadata.generation," +
					"SPINE:.spec.fabric.links[*].spine.port," +
					"LEAF:.spec.fabric.links[*].leaf.port",
			}
		}
		if options.Type == "vpc-loopback" {
			columns = []string{
				"-o", "custom-columns=" +
					"NAME:.metadata.name," +
					"GEN:.metadata.generation," +
					"PORT1:.spec.vpcLoopback.links[*].switch1.port," +
					"PORT2:.spec.vpcLoopback.links[*].switch2.port",
			}
		}

		return kubectl(ctx, append([]string{
			"get", "connections",
			"-l", fmt.Sprintf("fabric.githedgehog.com/connection-type=%s", options.Type),
		}, columns...)...)
	}

	return kubectl(ctx, "get", "connections")
}

func kubectl(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
