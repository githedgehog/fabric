package framework

import (
	"os"

	"github.com/pkg/errors"

	gg "github.com/onsi/ginkgo/v2"
	g "github.com/onsi/gomega"
)

type CollapsedCoreConfig struct {
	DualHomedServer1 string
	DualHomedServer2 string
}

type Helper struct {
	Kube   *KubeClient
	Server *ServerClient
}

func New() (*Helper, error) {
	gg.GinkgoHelper()

	g.Expect(os.Getenv("KUBECONFIG")).NotTo(g.BeZero(), "Please make sure KUBECONFIG is set")
	g.Expect(os.Getenv("KUBECONFIG")).To(g.BeAnExistingFile(), "Please make sure KUBECONFIG is set to existing file using absolute path")

	kube, err := getKubeClient()
	if err != nil {
		return nil, errors.Wrapf(err, "error initializaing test framework")
	}

	return &Helper{
		Kube:   kube,
		Server: &ServerClient{},
	}, nil
}

func (h *Helper) Cleanup() error {
	gg.GinkgoHelper()

	if h != nil {
		// TODO cleanup
		// return errors.Errorf("error cleaning up test framework")
	}

	return nil
}

func (h *Helper) CollapsedCore() *CollapsedCoreConfig {
	gg.GinkgoHelper()

	// TODO validate inputs

	return &CollapsedCoreConfig{
		DualHomedServer1: "server-1",
		DualHomedServer2: "server-2",
	}
}
