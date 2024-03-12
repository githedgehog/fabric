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

package meta

import (
	"regexp"

	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var nameChecker = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

func DefaultObjectMetadata(obj client.Object) {
	if obj.GetNamespace() == "" {
		obj.SetNamespace("default")
	}
}

func ValidateObjectMetadata(obj client.Object) error {
	if !nameChecker.MatchString(obj.GetName()) {
		return errors.Errorf("name does not match a lowercase RFC 1123 subdomain")
	}

	if obj.GetNamespace() != "default" {
		return errors.Errorf("only default namespace is currently supported")
	}

	return nil
}
