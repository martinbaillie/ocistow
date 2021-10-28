package config

import (
	"encoding/json"
	"fmt"

	"github.com/aws/aws-xray-sdk-go/xray"
	"github.com/aws/aws-xray-sdk-go/xraylog"
	"github.com/mattn/go-isatty"
	"github.com/sirupsen/logrus"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	stdlog "log"

	gocrlogs "github.com/google/go-containerregistry/pkg/logs"
)

const consoleTimeFormat = "15:04:05.000"

func (c *Config) Logger() *zerolog.Logger {
	if c.logger == nil {
		logger := zerolog.New(c.out)

		if isatty.IsTerminal(c.out.Fd()) {
			logger = log.Output(zerolog.ConsoleWriter{
				Out:          c.out,
				TimeFormat:   consoleTimeFormat,
				PartsExclude: []string{zerolog.LevelFieldName},
			})
		} else {
			logger = zerolog.New(c.out)
		}

		// Add timestamps.
		logger = logger.With().Timestamp().Logger()

		// Optionally set debug log level.
		level := zerolog.InfoLevel
		if *c.Debug {
			level = zerolog.DebugLevel

			// Adapt the Go container registry loggers.
			//
			// NOTE: Unfortunately these are package level output functions that
			// aren't context aware meaning it's not easy to enrich them with
			// request context. A solution to this is to override the transport
			// logging done by the container registry libraries.
			gocrLogger := logger.With().Str("level", zerolog.LevelDebugValue)
			gocrLogger = logger.With().Str("component", "registries")
			gocrlogs.Warn.SetOutput(gocrLogger.Logger())
			gocrlogs.Progress.SetOutput(gocrLogger.Logger())
			gocrlogs.Debug.SetOutput(gocrLogger.Logger())
		}
		logger = logger.Level(level)
		zerolog.SetGlobalLevel(level)

		// Adapt the ECR login helper package which uses a global logrus logger.
		logrus.SetOutput(logger.With().Str("component", "auth").Logger())

		// NOTE: This project uses the Xray libraries at the moment. Ideally
		// this would be the OTel libraries but they are not mature enough
		// for Go and Lambda quite yet. There's a couple of open issues and
		// PRs. For now just align the logs.
		if *c.AWSXray {
			xray.SetLogger(&xrayZeroLogger{logger.With().Str("component", "xray").Logger()})
		}

		// Tell the stdlib to use this logger.
		stdlog.SetFlags(0)
		stdlog.SetOutput(logger)

		c.logger = &logger
	}

	return c.logger
}

type xrayZeroLogger struct{ zerolog.Logger }

func (x *xrayZeroLogger) Log(ll xraylog.LogLevel, msg fmt.Stringer) {
	msgStr := msg.String()

	// Sometimes X-ray logs nested objects so try first to split the message
	// into fields.
	var fields map[string]interface{}
	if err := json.Unmarshal([]byte(msgStr), &fields); err == nil {
		var ok bool
		if msgStr, ok = fields["message"].(string); !ok {
			msgStr = "xray event"
		}
	}

	switch ll {
	case xraylog.LogLevelDebug:
		x.Debug().Fields(fields).Msg(msgStr)
	case xraylog.LogLevelWarn:
		x.Warn().Fields(fields).Msg(msgStr)
	case xraylog.LogLevelError:
		x.Error().Fields(fields).Msg(msgStr)
	case xraylog.LogLevelInfo:
		fallthrough
	default: // This is an enum but nothing is guaranteed.
		x.Info().Fields(fields).Msg(msgStr)
	}
}
