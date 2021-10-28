package config

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/rs/zerolog"

	ff "github.com/peterbourgon/ff/v3"
)

func New(name string, out *os.File) *Config {
	return &Config{
		FlagSet: flag.NewFlagSet(name, flag.ExitOnError),
		out:     out,
	}
}

type Config struct {
	*flag.FlagSet

	logger *zerolog.Logger
	out    *os.File

	Debug *bool

	Copy *bool
	Sign *bool

	AWSXray      *bool
	AWSKMSKeyARN *string
	AWSRegion    *string
}

func (c *Config) Parse(argv []string) error {
	c.Debug = c.Bool("debug", false, "debug logging")

	c.Copy = c.Bool("copy", true, "whether to copy the image")
	c.Sign = c.Bool("sign", true, "whether to sign the image")

	_, xrayDefault := os.LookupEnv("AWS_XRAY_DAEMON_ADDRESS")
	c.AWSXray = c.Bool("aws-xray", xrayDefault, "whether to enable AWS Xray tracing")
	c.AWSKMSKeyARN = c.String("aws-kms-key-arn", "", "AWS KMS key ARN to use for signing")
	c.AWSRegion = c.String("aws-region", "", "AWS region to use for operations")

	return ff.Parse(c.FlagSet, argv, ff.WithEnvVarNoPrefix())
}

func (c *Config) StringMap(name string, usage string) *StringMap {
	p := make(StringMap)

	c.Var(&p, name, usage)

	return (&p)
}

type StringMap map[string]string

func (sm *StringMap) Set(s string) error {
	kvs := strings.Split(s, "=")
	if len(kvs) != 2 {
		return fmt.Errorf("invalid key=value pair: %s", s)
	}

	(*sm)[kvs[0]] = kvs[1]

	return nil
}

func (sm *StringMap) String() (ret string) {
	for k, v := range *sm {
		ret += fmt.Sprintf("%s=%s", k, v)
	}

	return ret
}
