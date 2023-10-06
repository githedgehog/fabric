package framework

import (
	"context"
	// wiringapi "go.githedgehog.com/fabric/api/wiring/v1alpha2"
)

type ServerClient struct{}

// TODO replace with server *wiringapi.Server?
func (c *ServerClient) NetworkSetup(ctx context.Context, name string) (string, error) {
	return "123", nil
}

// TODO replace with server *wiringapi.Server?
func (c *ServerClient) NetworkCheck(ctx context.Context, name string, target string) error {
	return nil
}
