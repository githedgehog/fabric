//go:build e2e

package e2e

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lmittmann/tint"
	gg "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	g "github.com/onsi/gomega"
	"go.githedgehog.com/fabric/test/framework"
)

var h *framework.Helper

var _ = gg.BeforeSuite(func() {
	logLevel := slog.LevelDebug

	logger := slog.New(
		tint.NewHandler(gg.GinkgoWriter, &tint.Options{
			Level:      logLevel,
			TimeFormat: time.TimeOnly,
			NoColor:    true,
		}),
	)
	slog.SetDefault(logger)

	var err error
	h, err = framework.New()
	g.Expect(err).ToNot(g.HaveOccurred())
	g.Expect(h).ToNot(g.BeNil())
})

var _ = gg.AfterSuite(func() {
	g.Expect(h.Cleanup()).To(g.Succeed())
})

var _ = gg.ReportAfterEach(func(report gg.SpecReport) {
	// We can send report to TestOps after each test finishes
	// customFormat := fmt.Sprintf("%s | %s", report.State, report.FullText())
	// client.SendReport(customFormat)
})

var _ = gg.ReportAfterSuite("custom report", func(report gg.Report) {
	f, err := os.Create("report.custom")
	g.Expect(err).ToNot(g.HaveOccurred())
	defer f.Close()

	for _, specReport := range report.SpecReports {
		if specReport.LeafNodeType != types.NodeTypeIt {
			continue
		}
		fmt.Fprintf(f, "%s | %s | %s | %s | took %s\n", report.SuiteDescription, strings.Join(specReport.ContainerHierarchyTexts, " | "), specReport.LeafNodeText, specReport.State, specReport.RunTime)
	}
})

func TestFabricE2e(t *testing.T) {
	// GinkgoWriter.Write([]byte("hello world\n"))
	// slog.Info("hello world from slog")

	g.RegisterFailHandler(gg.Fail)
	gg.RunSpecs(t, "Fabric e2e test suite")
}
