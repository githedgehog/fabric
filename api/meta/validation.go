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
