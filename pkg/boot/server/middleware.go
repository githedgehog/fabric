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

package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func ResponseRequestID(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		rid := middleware.GetReqID(r.Context())
		if rid != "" {
			w.Header().Set("X-Request-ID", rid)
		}

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

func RequestLogger(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		start := time.Now()
		defer func() { //nolint:contextcheck
			rid := middleware.GetReqID(r.Context())
			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}

			slog.Debug("Request",
				"rid", rid,
				"method", r.Method,
				"url", fmt.Sprintf("%s://%s%s", scheme, r.Host, r.RequestURI),
				"from", r.RemoteAddr,
				"status", ww.Status(),
				"size", ww.BytesWritten(),
				"took", time.Since(start).Milliseconds(),
			)
		}()

		next.ServeHTTP(ww, r)
	}

	return http.HandlerFunc(fn)
}
