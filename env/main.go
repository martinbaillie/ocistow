package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"strconv"

	"github.com/aws/aws-cdk-go/awscdk"
	"github.com/aws/aws-cdk-go/awscdk/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/awslambda"
	"github.com/aws/aws-cdk-go/awscdk/awss3assets"
	"github.com/aws/jsii-runtime-go"

	"github.com/martinbaillie/ocistow/pkg/config"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	if err := run(os.Args, os.Stderr, cwd); err != nil {
		log.Fatal(err)
	}
}

func run(argv []string, out *os.File, cwd string) error {
	cfg := config.New(path.Base(argv[0]), out)

	if err := cfg.Parse(argv[1:]); err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	app := awscdk.NewApp(nil)

	tracing := awslambda.Tracing_DISABLED
	if *cfg.AWSXray {
		tracing = awslambda.Tracing_ACTIVE
	}

	stack := awscdk.NewStack(app, jsii.String("ocistow"), &awscdk.StackProps{})
	functionPkg := "github.com/martinbaillie/ocistow/cmd/ocistow-lambda"
	function := awslambda.NewFunction(
		stack,
		jsii.String("ocistow-function"),
		&awslambda.FunctionProps{
			FunctionName:  jsii.String("ocistow-function"),
			Tracing:       tracing,
			Timeout:       awscdk.Duration_Minutes(jsii.Number(15)), // Maximum Lambda duration.
			Runtime:       awslambda.Runtime_PROVIDED_AL2(),         // Graviton!
			Architectures: &[]awslambda.Architecture{awslambda.Architecture_ARM_64()},
			MemorySize:    jsii.Number(10240),
			Environment: &map[string]*string{
				"DEBUG":           jsii.String(strconv.FormatBool(*cfg.Debug)),
				"AWS_XRAY":        jsii.String(strconv.FormatBool(*cfg.AWSXray)),
				"AWS_KMS_KEY_ARN": cfg.AWSKMSKeyARN,
			},
			Code: awslambda.AssetCode_FromAsset(
				jsii.String(cwd),
				&awss3assets.AssetOptions{
					Bundling: &awscdk.BundlingOptions{
						Local: &localGoBundler{
							goOS:   "linux",
							goArch: "arm64",
							asset:  "bootstrap",
							pkg:    functionPkg,
						},
						User: jsii.String("root"),
						Command: jsii.Strings(
							"sh",
							"-c",
							"go build -ldflags=\"-s -w\" -o /asset-output/bootstrap "+functionPkg,
						),
						Image: awscdk.NewDockerImage( // Fallback bunndling image.
							jsii.String("docker.io/library/golang:1.16-alpine"),
							jsii.String(""),
						),
					},
				},
			),
			Handler: jsii.String("main"),
		},
	)

	function.Role().AddManagedPolicy(
		awsiam.ManagedPolicy_FromAwsManagedPolicyName(
			jsii.String("AmazonEC2ContainerRegistryFullAccess"),
		),
	)

	function.AddToRolePolicy(
		awsiam.NewPolicyStatement(
			&awsiam.PolicyStatementProps{
				Effect: awsiam.Effect_ALLOW,
				Actions: jsii.Strings(
					"kms:Sign",
					"kms:DescribeKey",
					"kms:GetPublicKey",
				),
				Resources: jsii.Strings(*cfg.AWSKMSKeyARN),
			},
		),
	)

	awscdk.NewCfnOutput(
		stack,
		jsii.String("Function ARN:"),
		&awscdk.CfnOutputProps{
			Value: function.FunctionArn(),
		},
	)

	_ = app.Synth(nil)

	return nil
}

type localGoBundler struct {
	goArch string
	goOS   string
	pkg    string
	asset  string
}

func (lgb *localGoBundler) TryBundle(outputDir *string, options *awscdk.BundlingOptions) *bool {
	// TODO: Check for local Golang first, return false if missing so that
	// Docker bundle gets used.

	cmd := exec.Command(
		"/usr/bin/env",
		"sh",
		"-c",
		fmt.Sprintf("go build -v -trimpath -ldflags=%q -o %s/%s %s",
			"-s -w", *outputDir, lgb.asset, lgb.pkg),
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "GOOS="+lgb.goOS, "GOARCH="+lgb.goArch)

	// TODO: Better handling.
	if err := cmd.Run(); err != nil {
		return jsii.Bool(false)
	}

	return jsii.Bool(true)
}
