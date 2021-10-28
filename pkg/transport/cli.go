package transport

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-xray-sdk-go/xray"

	"github.com/martinbaillie/ocistow/pkg/config"
	"github.com/martinbaillie/ocistow/pkg/service"
)

type StowCLI interface {
	Stow(src, dst string, annotations map[string]string) error
}

type stowCLI struct {
	config  *config.Config
	service service.Service
}

func NewCLI(cfg *config.Config, svc service.Service) StowCLI {
	return &stowCLI{service: svc, config: cfg}
}

func (c *stowCLI) Stow(src, dst string, annotations map[string]string) error {
	// Instantiate a few things like contexts and Xray segments that server
	// transports like Lambda would by default.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Minute*15))
	defer cancel()

	logger := c.config.Logger()

	if *c.config.AWSXray {
		var seg *xray.Segment
		ctx, seg = xray.BeginSegment(ctx, "Stow")

		cliLogger := logger.With().Str("trace_id", seg.TraceID).Logger()

		logger = &cliLogger

		defer seg.Close(nil)
	}

	ctx = logger.WithContext(ctx)

	if *c.config.Copy {
		if err := c.service.Copy(ctx, src, dst, annotations); err != nil {
			return fmt.Errorf("failed copy: %w", err)
		}
	}

	if *c.config.Sign {
		if err := c.service.Sign(ctx, dst, annotations); err != nil {
			return fmt.Errorf("failed sign: %w", err)
		}
	}

	return nil
}
