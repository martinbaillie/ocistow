package backend

import (
	"context"

	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sigstore/sigstore/pkg/signature"
)

// Backend models a subset of hooks to make it easier to specialise the more
// generic Go container registry and Sigstore code for a specific implementation
// (such as AWS).
type Backend interface {
	// Signing key.
	SignerVerifier(context.Context) (signature.SignerVerifier, error)

	// Authentication, request information.
	RemoteOpts(context.Context) []remote.Option
}
