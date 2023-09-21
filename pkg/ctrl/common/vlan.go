package common

import "github.com/pkg/errors"

type VLANRange struct {
	Min uint16 `json:"min,omitempty"`
	Max uint16 `json:"max,omitempty"`
}

func (r *VLANRange) Validate() error {
	if r.Min == 0 {
		return errors.Errorf("min is required")
	}
	if r.Max == 0 {
		return errors.Errorf("max is required")
	}
	if r.Min >= r.Max {
		return errors.Errorf("min must be less than max")
	}

	return nil
}
