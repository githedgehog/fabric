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
	"strings"

	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kvalidation "k8s.io/apimachinery/pkg/util/validation"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	ErrInvalidName      = fmt.Errorf("invalid resource name")
	ErrInvalidNamespace = fmt.Errorf("invalid resource namespace")
)

// MaxNameLength caps object names, and the names of other objects referenced by them, below the 63 characters
// Kubernetes allows for a label value or the name part of a label key, as the names are used in both
const MaxNameLength = 62

func DefaultObjectMetadata(obj kclient.Object) {
	if obj.GetNamespace() == "" {
		obj.SetNamespace(kmetav1.NamespaceDefault)
	}
}

// IsValidName reports whether the name can be used for an object, and so in the labels built from object names
func IsValidName(name string) bool {
	return len(name) <= MaxNameLength && len(kvalidation.IsDNS1123Subdomain(name)) == 0
}

// ValidateName checks the name of an object, or of another object it refers to, what describes it in the error.
// Object names are lowercase RFC 1123 subdomains, the same as Kubernetes requires, which are valid label values
// and label key names as long as they fit the length.
func ValidateName(what, name string) error {
	if errs := kvalidation.IsDNS1123Subdomain(name); len(errs) > 0 {
		return fmt.Errorf("%w: %s %q: %s", ErrInvalidName, what, name, strings.Join(errs, ", "))
	}
	if len(name) > MaxNameLength {
		return fmt.Errorf("%w: %s %s is too long, must be <= %d characters", ErrInvalidName, what, name, MaxNameLength)
	}

	return nil
}

func ValidateObjectMetadata(obj kclient.Object) error {
	if err := ValidateName("name", obj.GetName()); err != nil {
		return err
	}

	if obj.GetNamespace() != kmetav1.NamespaceDefault {
		return fmt.Errorf("%w: only default namespace is currently supported", ErrInvalidNamespace)
	}

	return nil
}
