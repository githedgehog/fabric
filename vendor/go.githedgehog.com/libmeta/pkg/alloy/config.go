// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package alloy

import (
	_ "embed"
	"fmt"
	"strings"

	"go.githedgehog.com/libmeta/pkg/tmpl"
)

const (
	PyroscopePort = 4040
)

// +kubebuilder:object:generate=true
type Config struct {
	Hostname     string  `json:"hostname,omitempty"`
	AutoHostname bool    `json:"autoHostname,omitempty"`
	Targets      Targets `json:"targets,omitempty"`
	ProxyURL     string  `json:"proxyURL,omitempty"`

	Scrapes   map[string]Scrape  `json:"scrapes,omitempty"`
	LogFiles  map[string]LogFile `json:"logFiles,omitempty"`
	Kube      Kube               `json:"kube,omitempty"`
	Pyroscope Pyroscope          `json:"pyroscope,omitempty"`
}

// +kubebuilder:object:generate=true
type Scrape struct {
	IntervalSeconds uint `json:"intervalSeconds,omitempty"`

	Address string     `json:"address,omitempty"`
	Self    ScrapeSelf `json:"self,omitempty"`
	Unix    ScrapeUnix `json:"unix,omitempty"`

	Relabel []ScrapeRelabelRule `json:"relabel,omitempty"`
}

// +kubebuilder:object:generate=true
type ScrapeSelf struct {
	Enable bool `json:"enable,omitempty"`
}

// +kubebuilder:object:generate=true
type ScrapeUnix struct {
	Enable     bool     `json:"enable,omitempty"`
	Collectors []string `json:"collectors,omitempty"`
}

// +kubebuilder:object:generate=true
type ScrapeRelabelRule struct {
	SourceLabels []string `json:"sourceLabels,omitempty"`
	Separator    string   `json:"separator,omitempty"`
	TargetLabel  string   `json:"targetLabel,omitempty"`
	Replacement  string   `json:"replacement,omitempty"`
	Regex        string   `json:"regex,omitempty"`
	Action       string   `json:"action,omitempty"`
}

// +kubebuilder:object:generate=true
type LogFile struct {
	PathTargets []LogFilePathTarget `json:"pathTargets,omitempty"`
}

// +kubebuilder:object:generate=true
type LogFilePathTarget struct {
	Path        string `json:"path,omitempty"`
	PathExclude string `json:"pathExclude,omitempty"`
}

// +kubebuilder:object:generate=true
type Kube struct {
	PodLogs bool `json:"podLogs,omitempty"`
	Events  bool `json:"events,omitempty"`
}

// +kubebuilder:object:generate=true
type Pyroscope struct {
	Enable  bool   `json:"enable,omitempty"`
	Address string `json:"address,omitempty"`
	Port    uint16 `json:"port,omitempty"`
}

func (cfg *Config) Validate() error {
	if cfg == nil {
		return fmt.Errorf("config is nil") //nolint:err113
	}

	if err := cfg.Targets.Validate(); err != nil {
		return fmt.Errorf("invalid targets: %w", err)
	}

	for name, scrape := range cfg.Scrapes {
		if err := validateIdentifier(name); err != nil {
			return fmt.Errorf("invalid scrape name %q: %w", name, err)
		}

		if err := scrape.Validate(); err != nil {
			return fmt.Errorf("invalid scrape %q: %w", name, err)
		}
	}

	for name := range cfg.LogFiles {
		if err := validateIdentifier(name); err != nil {
			return fmt.Errorf("invalid log file name %q: %w", name, err)
		}
	}

	return nil
}

func (s *Scrape) Validate() error {
	if s == nil {
		return fmt.Errorf("scrape is nil") //nolint:err113
	}

	opts := 0
	if s.Address != "" {
		opts++
	}
	if s.Self.Enable {
		opts++
	}
	if s.Unix.Enable {
		opts++
	}
	if opts == 0 {
		return fmt.Errorf("no scrape options enabled") //nolint:err113
	}
	if opts > 1 {
		return fmt.Errorf("multiple scrape options enabled") //nolint:err113
	}

	return nil
}

//go:embed config.alloy.tmpl
var configTemplate string

func (cfg *Config) Render() ([]byte, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil") //nolint:err113
	}

	if cfg.Pyroscope.Enable {
		if cfg.Pyroscope.Address == "" {
			cfg.Pyroscope.Address = "0.0.0.0"
		}
		if cfg.Pyroscope.Port == 0 {
			cfg.Pyroscope.Port = PyroscopePort
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	data, err := tmpl.Render("config.alloy.tmpl", configTemplate, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to render config: %w", err)
	}

	var res strings.Builder
	for line := range strings.Lines(string(data)) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		res.WriteString(line)
	}

	return []byte(res.String()), nil
}
