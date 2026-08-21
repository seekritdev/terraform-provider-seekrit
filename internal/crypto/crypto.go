// Package crypto is a Go port of the client-side primitives in
// packages/crypto (WebCrypto) that a Terraform provider needs: the `skt_`
// service-token scheme and the `wd1.` DEK wrapping (ECIES: P-256 ECDH +
// HKDF-SHA256 + AES-256-GCM).
//
// It is verified byte-for-byte against vectors emitted by the real
// @seekrit/crypto implementation (see testdata/gen-vectors.mts and
// crypto_test.go) — the same cross-implementation discipline apps/run and
// apps/provisioner use for their Rust ports. Nothing here touches secret
// plaintext; that path (sc1. AES-GCM + AAD) belongs to later provider phases.
package crypto

import (
	"crypto/ecdh"
	"crypto/ecdsa"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// b64 is base64url without padding — the encoding every seekrit blob uses
// (packages/crypto/src/encoding.ts toBase64Url/fromBase64Url).
var b64 = base64.RawURLEncoding

// toECDH normalizes whatever x509.ParsePKCS8PrivateKey yields for a P-256 key
// into an *ecdh.PrivateKey. WebCrypto exports ECDH P-256 keys with the
// id-ecPublicKey OID, which Go parses as *ecdsa.PrivateKey; convert it.
func toECDH(parsed any) (*ecdh.PrivateKey, error) {
	switch k := parsed.(type) {
	case *ecdh.PrivateKey:
		if k.Curve() != ecdh.P256() {
			return nil, errors.New("service token key is not P-256")
		}
		return k, nil
	case *ecdsa.PrivateKey:
		ek, err := k.ECDH()
		if err != nil {
			return nil, fmt.Errorf("service token key is not a valid ECDH key: %w", err)
		}
		if ek.Curve() != ecdh.P256() {
			return nil, errors.New("service token key is not P-256")
		}
		return ek, nil
	default:
		return nil, fmt.Errorf("unexpected private key type %T", parsed)
	}
}

// splitBlob mirrors packages/crypto/src/errors.ts: verify the version prefix
// and that there are exactly `segments` non-empty dot-separated parts after it.
func splitBlob(blob, prefix string, segments int) ([]string, error) {
	parts := strings.Split(blob, ".")
	if parts[0] != prefix {
		return nil, fmt.Errorf("expected a %q blob, got %q", prefix, parts[0])
	}
	if len(parts) != segments+1 {
		return nil, fmt.Errorf("malformed %q blob", prefix)
	}
	for _, p := range parts {
		if p == "" {
			return nil, fmt.Errorf("malformed %q blob", prefix)
		}
	}
	return parts[1:], nil
}

// decodeB64 decodes a base64url blob segment, labeling errors.
func decodeB64(part, label string) ([]byte, error) {
	out, err := b64.DecodeString(part)
	if err != nil {
		return nil, fmt.Errorf("malformed %s: %w", label, err)
	}
	return out, nil
}
