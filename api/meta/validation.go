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
	"fmt"
	"regexp"

	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrInvalidName      = fmt.Errorf("invalid resource name")
	ErrInvalidNamespace = fmt.Errorf("invalid resource namespace")
)

var nameChecker = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

func DefaultObjectMetadata(obj kclient.Object) {
	if obj.GetNamespace() == "" {
		obj.SetNamespace(kmetav1.NamespaceDefault)
	}
}

func ValidateObjectMetadata(obj kclient.Object) error {
	if !nameChecker.MatchString(obj.GetName()) {
		return fmt.Errorf("%w: name does not match a lowercase RFC 1123 subdomain", ErrInvalidName)
	}

	if obj.GetNamespace() != kmetav1.NamespaceDefault {
		return fmt.Errorf("%w: only default namespace is currently supported", ErrInvalidNamespace)
	}

	return nil
}
