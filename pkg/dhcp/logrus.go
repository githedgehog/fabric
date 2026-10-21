// Copyright 2026 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dhcp

import (
	"context"
	"io"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/sirupsen/logrus"
)

// our plugin returns a nil response for every request it drops and logs why
const coreDHCPNilResponse = "dropping request because response is nil"

// slogHook forwards coredhcp's logrus entries to slog, so they follow our log level and format
type slogHook struct{}

var _ logrus.Hook = slogHook{}

func (slogHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (slogHook) Fire(entry *logrus.Entry) error {
	if strings.HasSuffix(entry.Message, coreDHCPNilResponse) {
		return nil
	}

	level := slog.LevelDebug
	switch entry.Level {
	case logrus.PanicLevel, logrus.FatalLevel, logrus.ErrorLevel:
		level = slog.LevelError
	case logrus.WarnLevel:
		level = slog.LevelWarn
	case logrus.InfoLevel:
		level = slog.LevelInfo
	case logrus.DebugLevel, logrus.TraceLevel:
	}

	attrs := make([]any, 0, 2*len(entry.Data))
	for _, key := range slices.Sorted(maps.Keys(entry.Data)) {
		attrs = append(attrs, key, entry.Data[key])
	}
	slog.Log(context.Background(), level, entry.Message, attrs...)

	return nil
}

// forwardToSlog makes the logger only forward its entries to slog
func forwardToSlog(ctx context.Context, logger *logrus.Logger) {
	logger.SetOutput(io.Discard)
	logger.AddHook(slogHook{})
	if slog.Default().Enabled(ctx, slog.LevelDebug) {
		logger.SetLevel(logrus.DebugLevel)
	} else {
		logger.SetLevel(logrus.InfoLevel)
	}
}
