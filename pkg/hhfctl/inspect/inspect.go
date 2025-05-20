// Copyright 2023 Hedgehog
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package inspect

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
	"github.com/pkg/errors"
	agentapi "go.githedgehog.com/fabric/api/agent/v1beta1"
	dhcpapi "go.githedgehog.com/fabric/api/dhcp/v1beta1"
	vpcapi "go.githedgehog.com/fabric/api/vpc/v1beta1"
	wiringapi "go.githedgehog.com/fabric/api/wiring/v1beta1"
	"go.githedgehog.com/fabric/pkg/util/kubeutil"
	coreapi "k8s.io/api/core/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
	kyaml "sigs.k8s.io/yaml"
)

type Args struct {
	Verbose bool
	Output  OutputType
}

type OutputType string

const (
	OutputTypeUndefined OutputType = ""
	OutputTypeText      OutputType = "text"
	OutputTypeJSON      OutputType = "json"
	OutputTypeYAML      OutputType = "yaml"
)

var OutputTypes = []OutputType{OutputTypeText, OutputTypeJSON, OutputTypeYAML}

type In interface{}

type Out interface {
	MarshalText(now time.Time) (string, error)
}

type WithErrors interface {
	Errors() []error
}

type Func[TIn In, TOut Out] func(ctx context.Context, kube kclient.Reader, in TIn) (TOut, error)

func Run[TIn In, TOut Out](ctx context.Context, f Func[TIn, TOut], args Args, in TIn, w io.Writer) error {
	outType := OutputTypeText
	if args.Output != OutputTypeUndefined {
		outType = args.Output
	}

	if !slices.Contains(OutputTypes, outType) {
		return errors.Errorf("invalid output type: %s", outType)
	}

	kube, err := kubeutil.NewClient(ctx, "",
		wiringapi.SchemeBuilder,
		vpcapi.SchemeBuilder,
		agentapi.SchemeBuilder,
		dhcpapi.SchemeBuilder,
		&scheme.Builder{
			GroupVersion:  coreapi.SchemeGroupVersion,
			SchemeBuilder: coreapi.SchemeBuilder,
		})
	if err != nil {
		return errors.Wrapf(err, "cannot create kube client")
	}

	out, err := f(ctx, kube, in)
	if err != nil {
		return errors.Wrapf(err, "failed to run inspect function")
	}

	return Render(time.Now(), args.Output, w, out)
}

func Render[TOut Out](now time.Time, output OutputType, w io.Writer, out TOut) error {
	var data []byte
	var err error
	switch output {
	case OutputTypeText:
		dataS, err := out.MarshalText(now)
		if err != nil {
			return errors.Wrapf(err, "failed to get marshal output as text")
		}

		data = []byte(dataS)
	case OutputTypeYAML:
		data, err = kyaml.Marshal(out)
		if err != nil {
			return errors.Wrapf(err, "failed to marshal inspect output as yaml")
		}
	case OutputTypeJSON:
		data, err = json.MarshalIndent(out, "", "  ")
		if err != nil {
			return errors.Wrapf(err, "failed to marshal inspect output as json")
		}
	case OutputTypeUndefined:
		return errors.Errorf("output type %s is not defined", output)
	default:
		return errors.Errorf("output type %s is not implemented", output)
	}

	_, err = w.Write(data)
	if err != nil {
		return errors.Wrapf(err, "failed to write inspect output")
	}

	var o Out = out
	if we, ok := o.(WithErrors); ok {
		errs := we.Errors()

		if len(errs) > 0 {
			slog.Error("Inspect function reported errors", "count", len(errs))
		}

		for _, err := range errs {
			slog.Error("Reported ", "err", err)
		}

		if len(errs) > 0 {
			return errors.Errorf("inspect function reported %d errors", len(errs))
		}
	}

	return nil
}

func RenderTable(headers []string, data [][]string) string {
	str := &strings.Builder{}

	cfg := tablewriter.Config{
		Row: tw.CellConfig{
			Formatting: tw.CellFormatting{
				AutoWrap:  tw.WrapNormal,
				Alignment: tw.AlignLeft,
			},
			Padding: tw.CellPadding{Global: tw.Padding{Right: "    "}},
		},
		Header: tw.CellConfig{
			Formatting: tw.CellFormatting{
				AutoWrap:  tw.WrapNormal,
				Alignment: tw.AlignLeft,
			},
			Padding: tw.CellPadding{Global: tw.Padding{Right: "    "}},
		},
	}
	rendition := tw.Rendition{
		Borders: tw.BorderNone,
		Settings: tw.Settings{
			Lines:      tw.LinesNone,
			Separators: tw.SeparatorsNone,
		},
	}

	table := tablewriter.NewTable(str,
		tablewriter.WithRenderer(renderer.NewBlueprint(rendition)),
		tablewriter.WithConfig(cfg),
	)
	table.Header(headers)
	err := table.Bulk(data)
	if err != nil {
		slog.Error("Error in adding bulk data to table", "error", err)

		return "Error"
	}
	err = table.Render()
	if err != nil {
		slog.Error("Error in table rendering", "error", err)

		return "Error"
	}

	return str.String()
}

func HumanizeTime(now, then time.Time) string {
	return humanize.RelTime(then, now, "ago", "from now")
}
