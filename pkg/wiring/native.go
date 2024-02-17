package wiring

import (
	"github.com/pkg/errors"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1alpha2"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type NativeData struct {
	client.WithWatch
}

func NewNativeData() (*NativeData, error) {
	scheme := runtime.NewScheme()
	if err := wiringapi.AddToScheme(scheme); err != nil {
		return nil, errors.Wrap(err, "error adding fabricv1alpha1 to the scheme")
	}
	if err := vpcapi.AddToScheme(scheme); err != nil {
		return nil, errors.Wrap(err, "error adding vpcv1alpha1 to the scheme")
	}

	return &NativeData{
		fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects().
			Build(),
	}, nil
}
