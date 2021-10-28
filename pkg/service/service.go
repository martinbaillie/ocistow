package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/sigstore/cosign/pkg/oci"
	"github.com/sigstore/cosign/pkg/oci/static"
	"github.com/sigstore/cosign/pkg/oci/walk"
	"github.com/sigstore/sigstore/pkg/signature/payload"

	"github.com/martinbaillie/ocistow/pkg/backend"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	cosremote "github.com/sigstore/cosign/pkg/cosign/remote"
	ocimutate "github.com/sigstore/cosign/pkg/oci/mutate"
	ociremote "github.com/sigstore/cosign/pkg/oci/remote"
	sigopts "github.com/sigstore/sigstore/pkg/signature/options"
)

var (
	ErrInvalidOCIRefName = errors.New("invalid OCI reference name")
	ociReferenceRegex    = regexp.MustCompile(
		// NOTE: Sourced from https://git.io/JuM43.
		`^([A-Za-z0-9]+(([-._:@+]|--)[A-Za-z0-9]+)*)(/([A-Za-z0-9]+(([-._:@+]|--)[A-Za-z0-9]+)*))*$`,
	)
)

func parseOCIReference(ref string) (name.Reference, error) {
	if !ociReferenceRegex.MatchString(ref) {
		return nil, ErrInvalidOCIRefName
	}

	return name.ParseReference(ref)
}

type Service interface {
	Copy(ctx context.Context, src, dst string, annotations map[string]string) error
	Sign(ctx context.Context, dst string, annotations map[string]string) error
}

type service struct {
	backend backend.Backend
}

func NewService(b backend.Backend) Service {
	return &service{b}
}

func (s *service) Copy(ctx context.Context, src, dst string, annotations map[string]string) error {
	srcRef, err := parseOCIReference(src)
	if err != nil {
		return fmt.Errorf("parsing source reference %q: %w", src, err)
	}

	dstRef, err := parseOCIReference(dst)
	if err != nil {
		return fmt.Errorf("parsing destination reference %q: %w", dst, err)
	}

	var srcDesc *remote.Descriptor
	if srcDesc, err = remote.Get(srcRef, s.backend.RemoteOpts(ctx)...); err != nil {
		return fmt.Errorf("fetching %q: %w", src, err)
	}

	// TODO: Handle multi-arch.
	srcImg, err := srcDesc.Image()
	if err != nil {
		return fmt.Errorf("pulling linux/amd64 manifest: %w", err)
	}

	cfg, err := srcImg.ConfigFile()
	if err != nil {
		return fmt.Errorf("getting config: %w", err)
	}

	// Copy the existing config, merging the annotations with any existing
	// legacy Docker labels and overwriting where needed.
	cfg = cfg.DeepCopy()

	if cfg.Config.Labels == nil {
		cfg.Config.Labels = make(map[string]string, len(annotations))
	}

	for k, v := range annotations {
		cfg.Config.Labels[k] = v
	}

	srcImg, err = mutate.Config(srcImg, cfg.Config)
	if err != nil {
		return fmt.Errorf("mutating config: %w", err)
	}

	srcImg = mutate.Annotations(srcImg, annotations).(v1.Image)

	if err = remote.Write(dstRef, srcImg, s.backend.RemoteOpts(ctx)...); err != nil {
		return fmt.Errorf("writing destination image %q: %w", dstRef.Name(), err)
	}

	return nil
}

func (s *service) Sign(ctx context.Context, dst string, annotations map[string]string) error {
	dstRef, err := parseOCIReference(dst)
	if err != nil {
		return fmt.Errorf("parsing destination reference %q: %w", dst, err)
	}

	k, err := s.backend.SignerVerifier(ctx)
	if err != nil {
		return fmt.Errorf("discovering signing key: %w", err)
	}

	dd := cosremote.NewDupeDetector(k)

	// NOTE: Wrt. `ociremote` options, a signature suffix should be used to
	// allow for multiple tags in an AWS ECR to point at the same digest. This
	// currently breaks cosign verify though, as it has no suffix option.

	se, err := ociremote.SignedEntity(
		dstRef,
		ociremote.WithRemoteOptions(s.backend.RemoteOpts(ctx)...),
	)
	if err != nil {
		return fmt.Errorf("discovering existing signed entities: %w", err)
	}

	if err := walk.SignedEntity(ctx, se, func(ctx context.Context, se oci.SignedEntity) error {
		d, err := se.(interface{ Digest() (v1.Hash, error) }).Digest()
		if err != nil {
			return err
		}

		digest := dstRef.Context().Digest(d.String())

		downscaledAnnotations := make(map[string]interface{}, len(annotations))
		for k, v := range annotations {
			downscaledAnnotations[k] = v
		}

		payload, err := (&payload.Cosign{
			Image:       digest,
			Annotations: map[string]interface{}(downscaledAnnotations),
		}).MarshalJSON()
		if err != nil {
			return err
		}

		signature, err := k.SignMessage(bytes.NewReader(payload), sigopts.WithContext(ctx))
		if err != nil {
			return err
		}

		b64sig := base64.StdEncoding.EncodeToString(signature)

		sig, err := static.NewSignature(payload, b64sig)
		if err != nil {
			return err
		}

		newSE, err := ocimutate.AttachSignatureToEntity(se, sig, ocimutate.WithDupeDetector(dd))
		if err != nil {
			return err
		}

		return ociremote.WriteSignatures(
			digest.Repository,
			newSE,
			ociremote.WithRemoteOptions(s.backend.RemoteOpts(ctx)...),
		)
	}); err != nil {
		return fmt.Errorf("writing signatures: %w", err)
	}

	return nil
}
