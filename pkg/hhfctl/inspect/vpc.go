package inspect

import (
	"context"
	"strings"

	"github.com/davecgh/go-spew/spew"
	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	"go.githedgehog.com/fabric/pkg/util/apiutil"
	"go.githedgehog.com/fabric/pkg/util/pointer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/strings/slices"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type VPCIn struct {
	Name   string
	Subnet string
}

type VPCOut struct {
	Spec             *vpcapi.VPCSpec                         `json:"spec,omitempty"`
	VPCAttachments   map[string]*vpcapi.VPCAttachmentSpec    `json:"vpcAttachments,omitempty"`
	VPCPeerings      map[string]*vpcapi.VPCPeeringSpec       `json:"vpcPeerings,omitempty"`
	ExternalPeerings map[string]*vpcapi.ExternalPeeringSpec  `json:"externalPeerings,omitempty"`
	Access           map[string]*apiutil.ReachableFromSubnet `json:"access,omitempty"`
}

func (out *VPCOut) MarshalText() (string, error) {
	// TODO print VRF name

	return spew.Sdump(out), nil // TODO implement marshal
}

var _ Func[VPCIn, *VPCOut] = VPC

func VPC(ctx context.Context, kube client.Reader, in VPCIn) (*VPCOut, error) {
	if in.Name == "" {
		return nil, errors.New("name is required")
	}

	name := in.Name
	subnet := in.Subnet

	out := &VPCOut{
		VPCAttachments:   map[string]*vpcapi.VPCAttachmentSpec{},
		VPCPeerings:      map[string]*vpcapi.VPCPeeringSpec{},
		ExternalPeerings: map[string]*vpcapi.ExternalPeeringSpec{},
		Access:           map[string]*apiutil.ReachableFromSubnet{},
	}

	vpc := &vpcapi.VPC{}
	if err := kube.Get(ctx, client.ObjectKey{Name: name, Namespace: metav1.NamespaceDefault}, vpc); err != nil {
		return nil, errors.Wrap(err, "failed to get VPC")
	}

	if subnet != "" {
		if _, exist := vpc.Spec.Subnets[subnet]; !exist {
			return nil, errors.Errorf("subnet %q not found in VPC %q", subnet, name)
		}
	}

	out.Spec = &vpc.Spec

	vpcAttaches := &vpcapi.VPCAttachmentList{}
	if err := kube.List(ctx, vpcAttaches, client.MatchingLabels{
		vpcapi.LabelVPC: name,
	}); err != nil {
		return nil, errors.Wrap(err, "failed to list VPC attachments")
	}

	for _, vpcAttach := range vpcAttaches.Items {
		attachSubnet := strings.SplitN(vpcAttach.Spec.Subnet, "/", 2)[1]

		if subnet != "" && attachSubnet != subnet {
			continue
		}

		out.VPCAttachments[vpcAttach.Name] = pointer.To(vpcAttach.Spec)
	}

	vpcPeerings := &vpcapi.VPCPeeringList{}
	if err := kube.List(ctx, vpcPeerings, client.MatchingLabels{
		vpcapi.ListLabelVPC(name): vpcapi.ListLabelValue,
	}); err != nil {
		return nil, errors.Wrap(err, "failed to list VPC peerings")
	}

	for _, vpcPeering := range vpcPeerings.Items {
		if subnet != "" {
			found := false
			for _, permit := range vpcPeering.Spec.Permit {
				if peer, exist := permit[name]; exist && slices.Contains(peer.Subnets, subnet) {
					found = true

					break
				}
			}

			if !found {
				continue
			}
		}

		out.VPCPeerings[vpcPeering.Name] = pointer.To(vpcPeering.Spec)
	}

	extPeerings := &vpcapi.ExternalPeeringList{}
	if err := kube.List(ctx, extPeerings, client.MatchingLabels{
		vpcapi.LabelVPC: name,
	}); err != nil {
		return nil, errors.Wrap(err, "failed to list external peerings")
	}

	for _, extPeering := range extPeerings.Items {
		if subnet != "" && !slices.Contains(extPeering.Spec.Permit.VPC.Subnets, subnet) {
			continue
		}

		out.ExternalPeerings[extPeering.Name] = pointer.To(extPeering.Spec)
	}

	access, err := apiutil.GetReachableFrom(ctx, kube, name)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get reachable from vpc")
	}

	for subnetName, subnetAccess := range access {
		if subnet != "" && subnetName != subnet {
			continue
		}

		out.Access[subnetName] = subnetAccess
	}

	return out, nil
}
