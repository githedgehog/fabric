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

package logutil

import (
	"bufio"
	"context"
	"io"
)

func NewSink(ctx context.Context, log func(msg string, args ...any), msgPrefix string, args ...any) io.Writer {
	r, w := io.Pipe()

	go func() {
		lines := make(chan string)
		defer close(lines)

		go func() {
			defer func() {
				recover() //nolint: errcheck
			}()

			s := bufio.NewScanner(r)
			for s.Scan() {
				lines <- s.Text()
			}
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case line := <-lines:
				log(msgPrefix+line, args...)
			}
		}
	}()

	return w
}
