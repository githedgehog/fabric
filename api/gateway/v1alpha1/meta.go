// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

const (
	LabelPrefix    = "gateway.githedgehog.com/"
	ListLabelValue = "true"
	LabelVPCs      = LabelPrefix + "vpcs"
	LabelNAT       = LabelPrefix + "nat"

	AnnotationHugePages1G = LabelPrefix + "hugepages-1g"
	AnnotationHugePages2M = LabelPrefix + "hugepages-2m"
)

func ListLabelPrefix(listType string) string {
	return listType + "." + LabelPrefix
}

func ListLabel(listType, val string) string {
	return ListLabelPrefix(listType) + val
}

func ListLabelVPC(vpcName string) string {
	return ListLabel("vpc", vpcName)
}
