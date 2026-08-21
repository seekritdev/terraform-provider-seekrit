package crypto

import (
	"crypto/ecdh"
	"encoding/json"
	"fmt"
)

// ecPublicJWK is the JSON Web Key shape WebCrypto emits for an ECDH P-256
// public key (packages/crypto/src/keys.ts exportKey("jwk", ...)). Field order
// matches WebCrypto's output for cosmetic parity; only kty/crv/x/y are load
// bearing when the value is re-imported.
type ecPublicJWK struct {
	KeyOps []string `json:"key_ops"`
	Ext    bool     `json:"ext"`
	Kty    string   `json:"kty"`
	X      string   `json:"x"`
	Y      string   `json:"y"`
	Crv    string   `json:"crv"`
}

// publicKeyJWK serializes an ECDH P-256 public key to the JWK string the API
// stores (used to wrap DEK grants to this principal).
func publicKeyJWK(pub *ecdh.PublicKey) (string, error) {
	raw := pub.Bytes() // 0x04 || X(32) || Y(32), SEC1 uncompressed
	if len(raw) != 65 || raw[0] != 0x04 {
		return "", fmt.Errorf("unexpected P-256 public key encoding (%d bytes)", len(raw))
	}
	jwk := ecPublicJWK{
		KeyOps: []string{},
		Ext:    true,
		Kty:    "EC",
		X:      b64.EncodeToString(raw[1:33]),
		Y:      b64.EncodeToString(raw[33:65]),
		Crv:    "P-256",
	}
	out, err := json.Marshal(jwk)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// importPublicKeyJWK parses a stored JWK string back into an ECDH P-256 public
// key so a DEK can be wrapped to it.
func importPublicKeyJWK(jwkStr string) (*ecdh.PublicKey, error) {
	var jwk ecPublicJWK
	if err := json.Unmarshal([]byte(jwkStr), &jwk); err != nil {
		return nil, fmt.Errorf("invalid public key JWK: %w", err)
	}
	if jwk.Kty != "EC" || jwk.Crv != "P-256" {
		return nil, fmt.Errorf("unsupported public key JWK (kty=%q crv=%q)", jwk.Kty, jwk.Crv)
	}
	x, err := decodeB64(jwk.X, "JWK x coordinate")
	if err != nil {
		return nil, err
	}
	y, err := decodeB64(jwk.Y, "JWK y coordinate")
	if err != nil {
		return nil, err
	}
	// Left-pad each coordinate to the 32-byte field size, then rebuild the
	// SEC1 uncompressed point NewPublicKey expects.
	point := make([]byte, 65)
	point[0] = 0x04
	if len(x) > 32 || len(y) > 32 {
		return nil, fmt.Errorf("oversized P-256 coordinate")
	}
	copy(point[1+(32-len(x)):33], x)
	copy(point[33+(32-len(y)):65], y)
	pub, err := ecdh.P256().NewPublicKey(point)
	if err != nil {
		return nil, fmt.Errorf("invalid P-256 public key: %w", err)
	}
	return pub, nil
}
