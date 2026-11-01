// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1beta1

import (
	"errors"
	"fmt"
	"math"
	"net/netip"
	"strconv"
	"strings"
	"unicode"

	kyaml "sigs.k8s.io/yaml"
)

const (
	BCMMutateKeep     = "keep"
	BCMMutateOverride = "override"
	BCMMutateAdd      = "add"
	BCMMutateRemove   = "remove"

	BCMMutateKindACL           = "acl"
	BCMMutateKindRouteMap      = "routeMap"
	BCMMutateKindPrefixList    = "prefixList"
	BCMMutateKindACLInterface  = "aclInterface"
	BCMMutateKindStaticRoute   = "staticRoute"
	BCMMutateKindBFDProfile    = "bfdProfile"
	BCMMutateKindCommunityList = "communityList"
	BCMMutateKindASPathList    = "asPathList"

	BCMMutateIngress = "ingress"
	BCMMutateEgress  = "egress"
)

var ErrInvalidBCMMutateRule = errors.New("invalid bcm mutate rule")

// BCMMutate is the value of the AnnotationSwitchBCMMutate annotation (alpha)
type BCMMutate struct {
	// Rules are <mode>@<kind>/<name>[/<element>] strings
	Rules []string `json:"rules,omitempty"`
}

// SwitchBCMMutateRules parses the AnnotationSwitchBCMMutate annotation, it accepts both YAML and JSON
func SwitchBCMMutateRules(annotations map[string]string) ([]BCMMutateRule, error) {
	if annotations == nil {
		return nil, nil
	}

	v, ok := annotations[AnnotationSwitchBCMMutate]
	if !ok {
		return nil, nil
	}

	mutate := BCMMutate{}
	if err := kyaml.UnmarshalStrict([]byte(v), &mutate); err != nil {
		return nil, fmt.Errorf("parsing annotation %s: %w", AnnotationSwitchBCMMutate, err)
	}

	rules, err := ParseBCMMutateRules(mutate.Rules)
	if err != nil {
		return nil, fmt.Errorf("annotation %s: %w", AnnotationSwitchBCMMutate, err)
	}

	return rules, nil
}

// BCMMutateRule is a parsed <mode>@<kind>/<name>[/<element>] rule
type BCMMutateRule struct {
	Mode    string
	Kind    string
	Name    string
	Element string
}

func (r BCMMutateRule) String() string {
	if r.Element == "" {
		return r.Mode + "@" + r.Kind + "/" + r.Name
	}

	return r.Mode + "@" + r.Kind + "/" + r.Name + "/" + r.Element
}

// ParseBCMMutateRules parses the rules and rejects the conflicting ones
func ParseBCMMutateRules(rules []string) ([]BCMMutateRule, error) {
	res := make([]BCMMutateRule, 0, len(rules))
	paths := map[string]string{}
	objectRules := map[string]string{}
	elementRules := map[string]string{}
	for _, str := range rules {
		rule, err := ParseBCMMutateRule(str)
		if err != nil {
			return nil, err
		}

		path := rule.Kind + "/" + rule.Name
		if rule.Element != "" {
			path += "/" + rule.Element
		}
		if other, ok := paths[path]; ok {
			return nil, fmt.Errorf("%w: %q conflicts with %q", ErrInvalidBCMMutateRule, str, other)
		}
		paths[path] = str

		// an object rule already takes all of its elements, so elements of the same object can't have rules of their own
		if rule.Kind == BCMMutateKindACL || rule.Kind == BCMMutateKindRouteMap || rule.Kind == BCMMutateKindPrefixList {
			object := rule.Kind + "/" + rule.Name
			other, conflict := elementRules[object]
			if rule.Element != "" {
				other, conflict = objectRules[object]
			}
			if conflict {
				return nil, fmt.Errorf("%w: %q conflicts with %q", ErrInvalidBCMMutateRule, str, other)
			}

			if rule.Element == "" {
				objectRules[object] = str
			} else if _, ok := elementRules[object]; !ok {
				elementRules[object] = str
			}
		}

		res = append(res, rule)
	}

	return res, nil
}

func ParseBCMMutateRule(str string) (BCMMutateRule, error) {
	mode, path, ok := strings.Cut(str, "@")
	if !ok {
		return BCMMutateRule{}, fmt.Errorf("%w: %q has no mode", ErrInvalidBCMMutateRule, str)
	}

	parts := strings.SplitN(path, "/", 3)
	rule := BCMMutateRule{Mode: mode, Kind: parts[0]}
	if len(parts) > 1 {
		rule.Name = parts[1]
	}
	if len(parts) > 2 {
		rule.Element = parts[2]
		if rule.Element == "" {
			return BCMMutateRule{}, fmt.Errorf("%w: %q has an empty element", ErrInvalidBCMMutateRule, str)
		}
	}
	if rule.Name == "" {
		return BCMMutateRule{}, fmt.Errorf("%w: %q has no name", ErrInvalidBCMMutateRule, str)
	}

	switch mode {
	case BCMMutateKeep, BCMMutateOverride, BCMMutateAdd, BCMMutateRemove:
	default:
		return BCMMutateRule{}, fmt.Errorf("%w: %q has unknown mode %q", ErrInvalidBCMMutateRule, str, mode)
	}

	members := rule.Kind == BCMMutateKindCommunityList || rule.Kind == BCMMutateKindASPathList
	if (mode == BCMMutateAdd || mode == BCMMutateRemove) != (members && rule.Element != "") {
		return BCMMutateRule{}, fmt.Errorf("%w: %q: add and remove are only for community and as-path list members", ErrInvalidBCMMutateRule, str)
	}

	if err := validateBCMMutateElement(rule); err != nil {
		return BCMMutateRule{}, fmt.Errorf("%w: %q: %w", ErrInvalidBCMMutateRule, str, err)
	}

	return rule, nil
}

func validateBCMMutateElement(rule BCMMutateRule) error {
	switch rule.Kind {
	case BCMMutateKindACL, BCMMutateKindRouteMap:
		if rule.Element != "" {
			return validateBCMMutateSeq(rule.Element, math.MaxUint16)
		}
	case BCMMutateKindPrefixList:
		if rule.Element != "" {
			return validateBCMMutateSeq(rule.Element, math.MaxUint32)
		}
	case BCMMutateKindACLInterface:
		if rule.Element != BCMMutateIngress && rule.Element != BCMMutateEgress {
			return fmt.Errorf("direction must be %s or %s", BCMMutateIngress, BCMMutateEgress) //nolint:err113
		}
	case BCMMutateKindStaticRoute:
		prefix, err := netip.ParsePrefix(rule.Element)
		if err != nil {
			return fmt.Errorf("parsing prefix: %w", err)
		}
		if prefix.Masked().String() != rule.Element {
			return fmt.Errorf("prefix must be %s", prefix.Masked()) //nolint:err113
		}
	case BCMMutateKindBFDProfile:
		if rule.Element != "" {
			return fmt.Errorf("bfd profile has no elements") //nolint:err113
		}
	case BCMMutateKindCommunityList:
		if rule.Element != "" {
			asn, value, ok := strings.Cut(rule.Element, ":")
			if !ok {
				return fmt.Errorf("community must be ASN:value") //nolint:err113
			}
			if _, err := strconv.ParseUint(asn, 10, 16); err != nil {
				return fmt.Errorf("parsing community ASN: %w", err)
			}
			if _, err := strconv.ParseUint(value, 10, 16); err != nil {
				return fmt.Errorf("parsing community value: %w", err)
			}
		}
	case BCMMutateKindASPathList:
		if strings.ContainsFunc(rule.Element, unicode.IsSpace) {
			return fmt.Errorf("as-path member can't have spaces") //nolint:err113
		}
	default:
		return fmt.Errorf("unknown kind %q", rule.Kind) //nolint:err113
	}

	return nil
}

func validateBCMMutateSeq(seq string, maxSeq uint64) error {
	val, err := strconv.ParseUint(seq, 10, 64)
	if err != nil {
		return fmt.Errorf("parsing seq: %w", err)
	}
	if val == 0 || val > maxSeq || strconv.FormatUint(val, 10) != seq {
		return fmt.Errorf("seq must be a number from 1 to %d", maxSeq) //nolint:err113
	}

	return nil
}
