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
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

type logEntry struct {
	Extra logEntryExtra `json:"extra"`
	Level string        `json:"level"`
	Msg   string        `json:"message"`
	Ts    time.Time     `json:"timestamp"`
}

type logEntryExtra struct {
	MAC    string `json:"ethaddr"`
	Serial string `json:"serial"`
}

func (svc *service) handleLog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	l := slog.With("rid", middleware.GetReqID(ctx))

	if r.Body != nil {
		defer r.Body.Close()

		data, err := io.ReadAll(r.Body)
		if err != nil {
			l.Error("Failed to read log entry", "error", err.Error())

			return
		}

		entry := &logEntry{}
		if err := json.Unmarshal(data, entry); err != nil {
			l.Error("Failed to unmarshal log entry", "error", err.Error())
			w.WriteHeader(http.StatusBadRequest)

			return
		}

		entry.Msg = strings.TrimSpace(entry.Msg)
		if entry.Msg == "" {
			return
		}

		slog.Info("Log: "+entry.Msg, "serial", entry.Extra.Serial, "mac", entry.Extra.MAC, "level", entry.Level)
	}

	w.WriteHeader(http.StatusOK)
}
