package service

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-xray-sdk-go/xray"

	log "github.com/rs/zerolog"
)

type ServiceMiddleware func(Service) Service

func NewContextLoggerMiddleware() ServiceMiddleware {
	return func(s Service) Service { return &contextLoggerMiddleware{s} }
}

type contextLoggerMiddleware struct{ next Service }

func (clsm *contextLoggerMiddleware) Copy(
	ctx context.Context, src, dst string, annotations map[string]string,
) (err error) {
	then := time.Now()

	defer func() {
		var e *log.Event
		{
			if err != nil {
				e = log.Ctx(ctx).Error()
				e.Fields(map[string]interface{}{"err": err})
			} else {
				e = log.Ctx(ctx).Info()
			}
		}

		e.Str("component", "service").
			Str("method", "Copy").
			Fields(map[string]interface{}{
				"took":        fmt.Sprint(time.Since(then)),
				"src":         src,
				"dst":         dst,
				"annotations": annotations,
			}).Msg("")
	}()

	return clsm.next.Copy(ctx, src, dst, annotations)
}

func (clsm *contextLoggerMiddleware) Sign(
	ctx context.Context, dst string, annotations map[string]string,
) (err error) {
	then := time.Now()

	defer func() {
		var e *log.Event
		{
			if err != nil {
				e = log.Ctx(ctx).Error()
				e.Fields(map[string]interface{}{"err": err})
			} else {
				e = log.Ctx(ctx).Info()
			}
		}

		e.Str("component", "service").
			Str("method", "Sign").
			Fields(map[string]interface{}{
				"took":        fmt.Sprint(time.Since(then)),
				"dst":         dst,
				"annotations": annotations,
			}).
			Msg("")
	}()

	return clsm.next.Sign(ctx, dst, annotations)
}
func NewAWSXrayMiddleware() ServiceMiddleware {
	return func(s Service) Service { return &awsXrayMiddleware{s} }
}

type awsXrayMiddleware struct{ next Service }

func (clsm *awsXrayMiddleware) Copy(
	ctx context.Context, src, dst string, annotations map[string]string,
) (err error) {
	return xray.Capture(ctx, "Copy", func(ctxCopy context.Context) error {
		err := clsm.next.Copy(ctxCopy, src, dst, annotations)

		xray.AddMetadata(ctxCopy, "src", src)
		xray.AddMetadata(ctxCopy, "dst", dst)
		xray.AddMetadata(ctxCopy, "annotations", annotations)

		if err != nil {
			xray.AddMetadata(ctxCopy, "err", err)
		}

		return err
	})
}

func (clsm *awsXrayMiddleware) Sign(
	ctx context.Context, dst string, annotations map[string]string,
) (err error) {
	return xray.Capture(ctx, "Sign", func(ctxSign context.Context) error {
		err := clsm.next.Sign(ctxSign, dst, annotations)

		xray.AddMetadata(ctxSign, "dst", dst)
		xray.AddMetadata(ctxSign, "annotations", annotations)

		if err != nil {
			xray.AddMetadata(ctxSign, "err", err)
		}

		return err
	})
}
