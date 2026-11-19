// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package tmpl

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
)

func Render(name, tmplText string, data any) ([]byte, error) {
	tmplText = strings.TrimSpace(tmplText)

	tmpl, err := template.New(name).Funcs(sprig.FuncMap()).Option("missingkey=error").Parse(tmplText)
	if err != nil {
		return nil, fmt.Errorf("parsing template: %w", err)
	}

	buf := bytes.NewBuffer(nil)
	err = tmpl.Execute(buf, data)
	if err != nil {
		return nil, fmt.Errorf("executing template: %w", err)
	}

	return buf.Bytes(), nil
}
