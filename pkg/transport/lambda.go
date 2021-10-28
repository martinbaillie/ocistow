package transport

import (
	"context"
	"fmt"

	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-xray-sdk-go/header"
	"github.com/aws/aws-xray-sdk-go/xray"

	"github.com/martinbaillie/ocistow/pkg/config"
	"github.com/martinbaillie/ocistow/pkg/service"
)

type StowRequest struct {
	SrcImgRef   string            `json:"SrcImageRef"`
	DstImgRef   string            `json:"DstImageRef"`
	Annotations map[string]string `json:"Annotations"`
}

type StowLambdaHandler func(context.Context, StowRequest) error

func NewStowLambdaHandler(cfg *config.Config, svc service.Service) StowLambdaHandler {
	return func(ctx context.Context, req StowRequest) error {
		logger := cfg.Logger()

		if *cfg.AWSXray {
			// Extract key Lambda invoke information.
			var traceID string
			{
				if traceHeaderVal := ctx.Value(xray.LambdaTraceHeaderKey); traceHeaderVal != nil {
					traceHeader := traceHeaderVal.(string)
					traceID = header.FromString(traceHeader).TraceID
				}
			}
			lc, _ := lambdacontext.FromContext(ctx)

			// And create a contextual Lambda invoke-local logger.
			ll := logger.With().
				Str("aws_request_id", lc.AwsRequestID).
				Str("trace_id", traceID).
				Logger()

			logger = &ll
		}

		ctx = logger.WithContext(ctx)

		if *cfg.Copy {
			if err := svc.Copy(ctx, req.SrcImgRef, req.DstImgRef, req.Annotations); err != nil {
				return fmt.Errorf("failed copy: %w", err)
			}
		}

		if *cfg.Sign {
			if err := svc.Sign(ctx, req.DstImgRef, req.Annotations); err != nil {
				return fmt.Errorf("failed sign: %w", err)
			}
		}

		return nil
	}
}
