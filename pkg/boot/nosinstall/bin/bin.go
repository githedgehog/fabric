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

package bin

import (
	_ "embed"
	"fmt"
	"io"
)

//go:embed fabric-nos-install
var nosInstall []byte

func WriteNOSInstall(w io.Writer) error {
	_, err := w.Write(nosInstall)
	if err != nil {
		return fmt.Errorf("writing: %w", err)
	}

	return nil
}
