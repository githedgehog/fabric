// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"log/slog"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlogHook(t *testing.T) {
	logs := captureLogs(t)
	logger := logrus.New()
	forwardToSlog(t.Context(), logger)
	log := logger.WithField("prefix", "server")

	log.Print("MainHandler4: dropping request because response is nil")
	assert.Empty(t, logs.take(), "drops are logged by our plugin")

	for _, tt := range []struct {
		log   func(args ...any)
		level slog.Level
	}{
		{log.Error, slog.LevelError},
		{log.Warn, slog.LevelWarn},
		{log.Print, slog.LevelInfo},
		{log.Debug, slog.LevelDebug},
	} {
		tt.log("message")

		records := logs.take()
		require.Len(t, records, 1)
		assert.Equal(t, "message", records[0].Message)
		assert.Equal(t, tt.level, records[0].Level)
		assert.Equal(t, map[string]string{"prefix": "server"}, recordAttrs(records[0]))
	}
}
