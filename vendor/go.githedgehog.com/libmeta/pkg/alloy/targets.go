// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package alloy

import (
	"fmt"
)

// +kubebuilder:object:generate=true
type Targets struct {
	Prometheus map[string]PrometheusTarget `json:"prometheus,omitempty"`
	Loki       map[string]LokiTarget       `json:"loki,omitempty"`
}

// +kubebuilder:object:generate=true
type Target struct {
	URL                string            `json:"url,omitempty"`
	BasicAuth          *TargetBasicAuth  `json:"basicAuth,omitempty"`
	BearerToken        string            `json:"bearerToken,omitempty"`
	Labels             map[string]string `json:"labels,omitempty"`
	InsecureSkipVerify bool              `json:"insecureSkipVerify,omitempty"`
	CAPEM              string            `json:"caPEM,omitempty"`
	CertPEM            string            `json:"certPEM,omitempty"`
}

// +kubebuilder:object:generate=true
type TargetBasicAuth struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// +kubebuilder:object:generate=true
type PrometheusTarget struct {
	Target              `json:",inline"`
	SendIntervalSeconds uint `json:"sendIntervalSeconds,omitempty"`
}

// +kubebuilder:object:generate=true
type LokiTarget struct {
	Target `json:",inline"`
}

func (ts *Targets) Validate() error {
	if ts == nil {
		return fmt.Errorf("targets is nil") //nolint:err113
	}

	for name, target := range ts.Prometheus {
		if err := validateIdentifier(name); err != nil {
			return fmt.Errorf("invalid prometheus target name %q: %w", name, err)
		}

		if err := target.Validate(); err != nil {
			return fmt.Errorf("invalid prometheus target %q: %w", name, err)
		}
	}

	for name, target := range ts.Loki {
		if err := validateIdentifier(name); err != nil {
			return fmt.Errorf("invalid loki target name %q: %w", name, err)
		}

		if err := target.Validate(); err != nil {
			return fmt.Errorf("invalid loki target %q: %w", name, err)
		}
	}

	return nil
}

func (t *Target) Validate() error {
	if t == nil {
		return fmt.Errorf("target is nil") //nolint:err113
	}

	if t.URL == "" {
		return fmt.Errorf("URL is required") //nolint:err113
	}

	if t.BasicAuth != nil && t.BearerToken != "" {
		return fmt.Errorf("only one of basicAuth or bearerToken can be set") //nolint:err113
	}

	for label := range t.Labels {
		if err := validateIdentifier(label); err != nil {
			return fmt.Errorf("invalid label name: %s", label) //nolint:err113
		}
	}

	return nil
}
