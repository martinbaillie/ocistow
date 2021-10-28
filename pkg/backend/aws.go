package backend

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-xray-sdk-go/xray"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sigstore/sigstore/pkg/signature"

	ecrlogin "github.com/awslabs/amazon-ecr-credential-helper/ecr-login"
	ecrloginapi "github.com/awslabs/amazon-ecr-credential-helper/ecr-login/api"
	cosig "github.com/sigstore/cosign/pkg/signature"
)

const sigStoreKMSPrefix = "awskms:///"

func NewAWS(kmsKeyARN, region string, xrayEnabled bool) Backend {
	a := &awsBackend{
		xrayEnabled: xrayEnabled,
		kmsKeyARN:   kmsKeyARN,
		region:      region,
		remoteOpts: []remote.Option{
			remote.WithAuthFromKeychain(&ecrAuthenticatedKeychain{}),
		},
	}

	if !strings.HasPrefix(a.kmsKeyARN, sigStoreKMSPrefix) {
		a.kmsKeyARN = sigStoreKMSPrefix + a.kmsKeyARN
	}

	// FIXME: Shouldn't use the default transport here.
	transport := http.DefaultTransport
	if xrayEnabled {
		transport = xray.RoundTripper(transport)
	}

	a.remoteOpts = append(a.remoteOpts, remote.WithTransport(transport))

	return a
}

type awsBackend struct {
	xrayEnabled bool
	kmsKeyARN   string
	region      string

	remoteOpts []remote.Option
}

func (ab *awsBackend) SignerVerifier(ctx context.Context) (k signature.SignerVerifier, err error) {
	// Can't easily take control of the transport or the AWS session used in
	// this library in order to configure a region or Xray inject. Instead use
	// the process environment to influence the AWS region and wrap the call
	// with Xray at this level.
	if ab.region != "" {
		os.Setenv("AWS_REGION", ab.region)
	}

	if ab.xrayEnabled {
		xray.Capture(ctx, "Getting KMS signing key", func(ctxKMS context.Context) error {
			k, err = cosig.SignerVerifierFromKeyRef(ctxKMS, ab.kmsKeyARN, nil)
			return nil // Discard as k/err are outside this closure.
		})
	} else {
		k, err = cosig.SignerVerifierFromKeyRef(ctx, ab.kmsKeyARN, nil)
	}

	return k, err
}

func (ab *awsBackend) RemoteOpts(ctx context.Context) []remote.Option {
	return append([]remote.Option{remote.WithContext(ctx)}, ab.remoteOpts...)
}

// ecrAuthenticatedKeychain implements authentication for just ECR and
// everything else is considered anonymous.
//
// NOTE: The Go container registry libraries have a "multikeychain" feature that
// can be utilised to consult multiple authentication sources.
type ecrAuthenticatedKeychain struct{}

// ecrAuthenticatedKeychain implements the Go container registry Keychain
// interface.
var _ (authn.Keychain) = (*ecrAuthenticatedKeychain)(nil)

func (k *ecrAuthenticatedKeychain) Resolve(r authn.Resource) (authn.Authenticator, error) {
	// FIXME: Hack. Extract to pkg config.
	if err := os.Setenv("AWS_ECR_DISABLE_CACHE", "true"); err != nil {
		return nil, fmt.Errorf("disabling credential caching: %w", err)
	}

	// Handle the legacy default Docker case i.e. where no registry is provided,
	// things default to index.docker.io.
	key := r.RegistryStr()
	if key == name.DefaultRegistry {
		key = authn.DefaultAuthKey
	}

	// This is also quite a lazy check.
	if !strings.HasSuffix(key, "amazonaws.com") {
		return authn.Anonymous, nil
	}

	user, pass, err := (ecrlogin.ECRHelper{
		ClientFactory: &xRayTracedClientFactory{&ecrloginapi.DefaultClientFactory{}},
	}).Get(r.RegistryStr())

	if err != nil {
		return nil, err
	}

	return authn.FromConfig(authn.AuthConfig{Username: user, Password: pass}), nil
}

// A full client factory is needed purely to take control of
// NewClientWithOptions so that the AWS session can be wrapped with X-ray
// instrumentation.
type xRayTracedClientFactory struct{ wrapped ecrloginapi.ClientFactory }

var _ (ecrloginapi.ClientFactory) = (*xRayTracedClientFactory)(nil)

func (x *xRayTracedClientFactory) NewClientWithOptions(o ecrloginapi.Options) ecrloginapi.Client {
	o.Session = xray.AWSSession(o.Session)

	return x.wrapped.NewClientWithOptions(o)
}

func (x *xRayTracedClientFactory) NewClient(s *session.Session, c *aws.Config) ecrloginapi.Client {
	return x.wrapped.NewClient(s, c)
}

func (x *xRayTracedClientFactory) NewClientFromRegion(r string) ecrloginapi.Client {
	return x.wrapped.NewClientFromRegion(r)
}

func (x *xRayTracedClientFactory) NewClientWithFipsEndpoint(r string) (ecrloginapi.Client, error) {
	return x.wrapped.NewClientWithFipsEndpoint(r)
}

func (x *xRayTracedClientFactory) NewClientWithDefaults() ecrloginapi.Client {
	return x.wrapped.NewClientWithDefaults()
}
