package main

import (
	"fmt"
	"log"
	"os"
	"path"

	"github.com/martinbaillie/ocistow/pkg/backend"
	"github.com/martinbaillie/ocistow/pkg/config"
	"github.com/martinbaillie/ocistow/pkg/service"
	"github.com/martinbaillie/ocistow/pkg/transport"
)

func run(argv []string, out *os.File) error {
	cfg := config.New(path.Base(argv[0]), out)

	src := cfg.String("source", "", "source image")
	dst := cfg.String("destination", "", "destination image")
	annotations := cfg.StringMap("annotations", "destination image annotations (key=value)")

	if err := cfg.Parse(argv[1:]); err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	svc := service.NewService(backend.NewAWS(*cfg.AWSKMSKeyARN, *cfg.AWSRegion, *cfg.AWSXray))

	if *cfg.AWSXray {
		svc = service.NewAWSXrayMiddleware()(svc)
	}

	svc = service.NewContextLoggerMiddleware()(svc)

	return transport.NewCLI(cfg, svc).Stow(*src, *dst, *annotations)
}

func main() {
	if err := run(os.Args, os.Stderr); err != nil {
		log.Fatal(err)
	}
}
