
# slog: Webhook handler

[![tag](https://img.shields.io/github/tag/samber/slog-webhook.svg)](https://github.com/samber/slog-webhook/releases)
![Go Version](https://img.shields.io/badge/Go-%3E%3D%201.21-%23007d9c)
[![GoDoc](https://godoc.org/github.com/samber/slog-webhook?status.svg)](https://pkg.go.dev/github.com/samber/slog-webhook)
![Build Status](https://github.com/samber/slog-webhook/actions/workflows/test.yml/badge.svg)
[![Go report](https://goreportcard.com/badge/github.com/samber/slog-webhook)](https://goreportcard.com/report/github.com/samber/slog-webhook)
[![Coverage](https://img.shields.io/codecov/c/github/samber/slog-webhook)](https://codecov.io/gh/samber/slog-webhook)
[![Contributors](https://img.shields.io/github/contributors/samber/slog-webhook)](https://github.com/samber/slog-webhook/graphs/contributors)
[![License](https://img.shields.io/github/license/samber/slog-webhook)](./LICENSE)

A [webhook](https://webhook.com) Handler for [slog](https://pkg.go.dev/log/slog) Go library.

<div align="center">
  <hr>
  <sup><b>Sponsored by:</b></sup>
  <br>
  <a href="https://quickwit.io?utm_campaign=github_sponsorship&utm_medium=referral&utm_content=samber-slog-webhook&utm_source=github">
    <div>
      <img src="https://github.com/samber/oops/assets/2951285/49aaaa2b-b8c6-4f21-909f-c12577bb6a2e" width="240" alt="Quickwit">
    </div>
    <div>
      Cloud-native search engine for observability - An OSS alternative to Splunk, Elasticsearch, Loki, and Tempo.
    </div>
  </a>
  <hr>
</div>

**See also:**

- [slog-multi](https://github.com/samber/slog-multi): `slog.Handler` chaining, fanout, routing, failover, load balancing...
- [slog-formatter](https://github.com/samber/slog-formatter): `slog` attribute formatting
- [slog-sampling](https://github.com/samber/slog-sampling): `slog` sampling policy
- [slog-mock](https://github.com/samber/slog-mock): `slog.Handler` for test purposes

**HTTP middlewares:**

- [slog-gin](https://github.com/samber/slog-gin): Gin middleware for `slog` logger
- [slog-echo](https://github.com/samber/slog-echo): Echo middleware for `slog` logger
- [slog-fiber](https://github.com/samber/slog-fiber): Fiber middleware for `slog` logger
- [slog-chi](https://github.com/samber/slog-chi): Chi middleware for `slog` logger
- [slog-http](https://github.com/samber/slog-http): `net/http` middleware for `slog` logger

**Loggers:**

- [slog-zap](https://github.com/samber/slog-zap): A `slog` handler for `Zap`
- [slog-zerolog](https://github.com/samber/slog-zerolog): A `slog` handler for `Zerolog`
- [slog-logrus](https://github.com/samber/slog-logrus): A `slog` handler for `Logrus`

**Log sinks:**

- [slog-datadog](https://github.com/samber/slog-datadog): A `slog` handler for `Datadog`
- [slog-betterstack](https://github.com/samber/slog-betterstack): A `slog` handler for `Betterstack`
- [slog-rollbar](https://github.com/samber/slog-rollbar): A `slog` handler for `Rollbar`
- [slog-loki](https://github.com/samber/slog-loki): A `slog` handler for `Loki`
- [slog-sentry](https://github.com/samber/slog-sentry): A `slog` handler for `Sentry`
- [slog-syslog](https://github.com/samber/slog-syslog): A `slog` handler for `Syslog`
- [slog-logstash](https://github.com/samber/slog-logstash): A `slog` handler for `Logstash`
- [slog-fluentd](https://github.com/samber/slog-fluentd): A `slog` handler for `Fluentd`
- [slog-graylog](https://github.com/samber/slog-graylog): A `slog` handler for `Graylog`
- [slog-quickwit](https://github.com/samber/slog-quickwit): A `slog` handler for `Quickwit`
- [slog-slack](https://github.com/samber/slog-slack): A `slog` handler for `Slack`
- [slog-telegram](https://github.com/samber/slog-telegram): A `slog` handler for `Telegram`
- [slog-mattermost](https://github.com/samber/slog-mattermost): A `slog` handler for `Mattermost`
- [slog-microsoft-teams](https://github.com/samber/slog-microsoft-teams): A `slog` handler for `Microsoft Teams`
- [slog-webhook](https://github.com/samber/slog-webhook): A `slog` handler for `Webhook`
- [slog-kafka](https://github.com/samber/slog-kafka): A `slog` handler for `Kafka`
- [slog-nats](https://github.com/samber/slog-nats): A `slog` handler for `NATS`
- [slog-parquet](https://github.com/samber/slog-parquet): A `slog` handler for `Parquet` + `Object Storage`
- [slog-channel](https://github.com/samber/slog-channel): A `slog` handler for Go channels

## 🚀 Install

```sh
go get github.com/samber/slog-webhook/v2
```

**Compatibility**: go >= 1.21

No breaking changes will be made to exported APIs before v3.0.0.

## 💡 Usage

GoDoc: [https://pkg.go.dev/github.com/samber/slog-webhook/v2](https://pkg.go.dev/github.com/samber/slog-webhook/v2)

### Handler options

```go
type Option struct {
  // log level (default: debug)
  Level     slog.Leveler

  // URL
  Endpoint string
  Timeout  time.Duration // default: 10s

  // optional: customize webhook event builder
  Converter Converter
  // optional: custom marshaler
  Marshaler func(v any) ([]byte, error)
  // optional: fetch attributes from context
  AttrFromContext []func(ctx context.Context) []slog.Attr

  // optional: see slog.HandlerOptions
  AddSource   bool
  ReplaceAttr func(groups []string, a slog.Attr) slog.Attr
}
```

Other global parameters:

```go
slogwebhook.SourceKey = "source"
slogwebhook.ContextKey = "extra"
slogwebhook.ErrorKeys = []string{"error", "err"}
slogwebhook.RequestIgnoreHeaders = false
```

### Supported attributes

The following attributes are interpreted by `slogwebhook.DefaultConverter`:

| Atribute name    | `slog.Kind`       | Underlying type |
| ---------------- | ----------------- | --------------- |
| "user"           | group (see below) |                 |
| "error"          | any               | `error`         |
| "request"        | any               | `*http.Request` |
| other attributes | *                 |                 |

Other attributes will be injected in `extra` field.

Users must be of type `slog.Group`. Eg:

```go
slog.Group("user",
  slog.String("id", "user-123"),
  slog.String("username", "samber"),
  slog.Time("created_at", time.Now()),
)
```

### Example

```go
import (
	"fmt"
	"net/http"
	"time"

	slogwebhook "github.com/samber/slog-webhook/v2"

	"log/slog"
)

func main() {
  url := "https://webhook.site/xxxxxx"

  logger := slog.New(slogwebhook.Option{Level: slog.LevelDebug, Endpoint: url}.NewWebhookHandler())
  logger = logger.With("release", "v1.0.0")

  req, _ := http.NewRequest(http.MethodGet, "https://api.screeb.app", nil)
  req.Header.Set("Content-Type", "application/json")
  req.Header.Set("X-TOKEN", "1234567890")

  logger.
    With(
      slog.Group("user",
        slog.String("id", "user-123"),
        slog.Time("created_at", time.Now()),
      ),
    ).
    With("request", req).
    With("error", fmt.Errorf("an error")).
    Error("a message")
}
```

Output:

```json
{
  "error": {
    "error": "an error",
    "kind": "*errors.errorString",
    "stack": null
  },
  "extra": {
	"release": "v1.0.0"
  },
  "level": "ERROR",
  "logger": "samber/slog-webhook",
  "message": "a message",
  "request": {
    "headers": {
      "Content-Type": "application/json",
      "X-Token": "1234567890"
    },
    "host": "api.screeb.app",
    "method": "GET",
    "url": {
      "fragment": "",
      "host": "api.screeb.app",
      "path": "",
      "query": {},
      "raw_query": "",
      "scheme": "https",
      "url": "https://api.screeb.app"
    }
  },
  "timestamp": "2023-04-10T14:00:0.000000",
  "user": {
	"id": "user-123",
    "created_at": "2023-04-10T14:00:0.000000"
  }
}
```

### Tracing

Import the samber/slog-otel library.

```go
import (
	slogwebhook "github.com/samber/slog-webhook"
	slogotel "github.com/samber/slog-otel"
	"go.opentelemetry.io/otel/sdk/trace"
)

func main() {
	tp := trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
	)
	tracer := tp.Tracer("hello/world")

	ctx, span := tracer.Start(context.Background(), "foo")
	defer span.End()

	span.AddEvent("bar")

	logger := slog.New(
		slogwebhook.Option{
			// ...
			AttrFromContext: []func(ctx context.Context) []slog.Attr{
				slogotel.ExtractOtelAttrFromContext([]string{"tracing"}, "trace_id", "span_id"),
			},
		}.NewWebhookHandler(),
	)

	logger.ErrorContext(ctx, "a message")
}
```

## 🤝 Contributing

- Ping me on twitter [@samuelberthe](https://twitter.com/samuelberthe) (DMs, mentions, whatever :))
- Fork the [project](https://github.com/samber/slog-webhook)
- Fix [open issues](https://github.com/samber/slog-webhook/issues) or request new features

Don't hesitate ;)

```bash
# Install some dev dependencies
make tools

# Run tests
make test
# or
make watch-test
```

## 👤 Contributors

![Contributors](https://contrib.rocks/image?repo=samber/slog-webhook)

## 💫 Show your support

Give a ⭐️ if this project helped you!

[![GitHub Sponsors](https://img.shields.io/github/sponsors/samber?style=for-the-badge)](https://github.com/sponsors/samber)

## 📝 License

Copyright © 2023 [Samuel Berthe](https://github.com/samber).

This project is [MIT](./LICENSE) licensed.
