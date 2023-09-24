package validation

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Client interface {
	Get(ctx context.Context, key client.ObjectKey, obj client.Object) error
	List(ctx context.Context, list client.ObjectList, labels client.MatchingLabels) error
}

type inController struct {
	client.Client
}

var _ Client = &inController{}

func (c *inController) Get(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	return c.Client.Get(ctx, key, obj)
}

func (c *inController) List(ctx context.Context, list client.ObjectList, labels client.MatchingLabels) error {
	return c.Client.List(ctx, list, labels)
}

func WithCtrlRuntime(client client.Client) Client {
	return &inController{client}
}
