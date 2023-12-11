//go:build linux
// +build linux

package dhcpd

import (
	"github.com/coredhcp/coredhcp/handler"
	"github.com/insomniacslk/dhcp/dhcpv4"
	// dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1alpha2"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func setup(svc *Service) func(args ...string) (handler.Handler4, error) {
	return func(args ...string) (handler.Handler4, error) {
		// TODO
		// you can use params from svc here, like
		// svc.kubeUpdates channel to listen for updates from k8s
		// svc.updateStatus to update status of a subnets in k8s

		// for {
		// 	switch <-svc.kubeUpdates {
		// 	//..
		// 	}
		// }

		// subnet := dhcpapi.DHCPSubnet{}
		// subnet.Status.Allocated["asdasd"] = dhcpapi.DHCPAllocated{
		// 	IP:       "",
		// 	Expiry:   metav1.Time{},
		// 	Hostname: "",
		// }
		// err := svc.updateStatus(subnet)

		return handlerDHCP4, nil
	}
}

func handlerDHCP4(req, resp *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, bool) {
	// TODO
	return resp, false
}
