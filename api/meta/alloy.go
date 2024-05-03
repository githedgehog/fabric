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
)

var alloyLabel = regexp.MustCompile(`^[a-z]([_a-z0-9]*[a-z0-9])?$`)

// +kubebuilder:object:generate=true
type AlloyConfig struct {
	AgentScrapeIntervalSeconds uint                             `json:"agentScrapeIntervalSeconds,omitempty"`
	UnixExporterEnabled        bool                             `json:"unixExporterEnabled,omitempty"`
	UnixExporterCollectors     []string                         `json:"unixExporterCollectors,omitempty"`
	UnixScrapeIntervalSeconds  uint                             `json:"unixScrapeIntervalSeconds,omitempty"`
	PrometheusTargets          map[string]AlloyPrometheusTarget `json:"prometheusTargets,omitempty"`
	LokiTargets                map[string]AlloyLokiTarget       `json:"lokiTargets,omitempty"`
}

type AlloyBasicAuth struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// +kubebuilder:object:generate=true
type AlloyTarget struct {
	URL                string            `json:"url,omitempty"`
	BasicAuth          AlloyBasicAuth    `json:"basicAuth,omitempty"`
	BearerToken        string            `json:"bearerToken,omitempty"`
	Labels             map[string]string `json:"labels,omitempty"`
	UseControlProxy    bool              `json:"useControlProxy,omitempty"`
	InsecureSkipVerify bool              `json:"insecureSkipVerify,omitempty"`
	CAPEM              string            `json:"caPEM,omitempty"`
	CertPEM            string            `json:"certPEM,omitempty"`
}

// +kubebuilder:object:generate=true
type AlloyPrometheusTarget struct {
	AlloyTarget         `json:",inline"`
	SendIntervalSeconds uint `json:"sendIntervalSeconds,omitempty"`
}

// +kubebuilder:object:generate=true
type AlloyLokiTarget struct {
	AlloyTarget `json:",inline"`
}

func (a *AlloyConfig) Default() {
	a.AgentScrapeIntervalSeconds = max(a.AgentScrapeIntervalSeconds, 15)
	a.UnixScrapeIntervalSeconds = max(a.UnixScrapeIntervalSeconds, 15)

	for _, t := range a.PrometheusTargets {
		t.SendIntervalSeconds = max(t.SendIntervalSeconds, 15)
	}
}

func (a *AlloyConfig) Validate() error {
	for name := range a.PrometheusTargets {
		if !alloyLabel.MatchString(name) {
			return errors.Errorf("prometheus target name %q isn't valid", name)
		}
	}

	for name := range a.LokiTargets {
		if !alloyLabel.MatchString(name) {
			return errors.Errorf("loki target name %q isn't valid", name)
		}
	}

	return nil
}
