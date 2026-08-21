package crypto

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// vectors mirrors testdata/vectors.json, emitted by the real @seekrit/crypto
// (see testdata/gen-vectors.mts). This is the cross-implementation ground truth
// proving the Go port is bit-compatible with the browser/CLI/Workers crypto.
type vectors struct {
	Token                 string `json:"token"`
	TokenID               string `json:"tokenId"`
	TokenHash             string `json:"tokenHash"`
	PublicKeyJWK          string `json:"publicKeyJwk"`
	DEK                   string `json:"dek"`
	WrappedDEK            string `json:"wrappedDek"`
	RecipientToken        string `json:"recipientToken"`
	RecipientPublicKeyJWK string `json:"recipientPublicKeyJwk"`
	EnvironmentID         string `json:"environmentId"`
	SecretName            string `json:"secretName"`
	SecretPlaintext       string `json:"secretPlaintext"`
	SecretCiphertext      string `json:"secretCiphertext"`
}

func loadVectors(t *testing.T) vectors {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/vectors.json")
	if err != nil {
		t.Fatalf("read vectors: %v (regenerate with testdata/gen-vectors.mts)", err)
	}
	var v vectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("parse vectors: %v", err)
	}
	return v
}

// jwkEqual compares two JWK strings semantically (field order is irrelevant).
func jwkEqual(t *testing.T, a, b string) bool {
	t.Helper()
	var ja, jb ecPublicJWK
	if err := json.Unmarshal([]byte(a), &ja); err != nil {
		t.Fatalf("parse jwk a: %v", err)
	}
	if err := json.Unmarshal([]byte(b), &jb); err != nil {
		t.Fatalf("parse jwk b: %v", err)
	}
	return ja.Kty == jb.Kty && ja.Crv == jb.Crv && ja.X == jb.X && ja.Y == jb.Y
}

// TestTokenParseMatchesVectors proves parse + hash + public-key derivation are
// compatible with WebCrypto's token output.
func TestTokenParseMatchesVectors(t *testing.T) {
	v := loadVectors(t)

	tokenID, priv, err := ParseServiceToken(v.Token)
	if err != nil {
		t.Fatalf("ParseServiceToken: %v", err)
	}
	if tokenID != v.TokenID {
		t.Errorf("tokenID = %q, want %q", tokenID, v.TokenID)
	}
	if got := HashToken(v.Token); got != v.TokenHash {
		t.Errorf("HashToken = %q, want %q", got, v.TokenHash)
	}
	gotJWK, err := PublicKeyJWK(priv)
	if err != nil {
		t.Fatalf("PublicKeyJWK: %v", err)
	}
	if !jwkEqual(t, gotJWK, v.PublicKeyJWK) {
		t.Errorf("derived public JWK does not match vector:\n got  %s\n want %s", gotJWK, v.PublicKeyJWK)
	}
}

// TestUnwrapMatchesVectors proves the Go ECDH/HKDF/AES-GCM unwrap recovers the
// exact DEK that WebCrypto wrapped.
func TestUnwrapMatchesVectors(t *testing.T) {
	v := loadVectors(t)
	_, priv, err := ParseServiceToken(v.Token)
	if err != nil {
		t.Fatalf("ParseServiceToken: %v", err)
	}
	wantDEK, err := b64.DecodeString(v.DEK)
	if err != nil {
		t.Fatalf("decode dek: %v", err)
	}
	gotDEK, err := UnwrapDEK(v.WrappedDEK, priv)
	if err != nil {
		t.Fatalf("UnwrapDEK: %v", err)
	}
	if !bytes.Equal(gotDEK, wantDEK) {
		t.Errorf("unwrapped DEK mismatch:\n got  %x\n want %x", gotDEK, wantDEK)
	}
}

// TestGrantRewrapRoundTrip proves the grant path: unwrap our DEK, re-wrap it to
// a different principal, and confirm that principal can unwrap it back.
func TestGrantRewrapRoundTrip(t *testing.T) {
	v := loadVectors(t)
	_, priv, err := ParseServiceToken(v.Token)
	if err != nil {
		t.Fatalf("ParseServiceToken (principal): %v", err)
	}
	dek, err := UnwrapDEK(v.WrappedDEK, priv)
	if err != nil {
		t.Fatalf("UnwrapDEK: %v", err)
	}
	rewrapped, err := WrapDEK(dek, v.RecipientPublicKeyJWK)
	if err != nil {
		t.Fatalf("WrapDEK to recipient: %v", err)
	}
	_, recipPriv, err := ParseServiceToken(v.RecipientToken)
	if err != nil {
		t.Fatalf("ParseServiceToken (recipient): %v", err)
	}
	got, err := UnwrapDEK(rewrapped, recipPriv)
	if err != nil {
		t.Fatalf("recipient UnwrapDEK: %v", err)
	}
	if !bytes.Equal(got, dek) {
		t.Errorf("re-wrap round trip mismatch:\n got  %x\n want %x", got, dek)
	}
}

// TestWrongKeyAndTamperFail proves a wrong key or tampered blob never silently
// decrypts.
func TestWrongKeyAndTamperFail(t *testing.T) {
	v := loadVectors(t)
	_, priv, err := ParseServiceToken(v.Token)
	if err != nil {
		t.Fatalf("ParseServiceToken: %v", err)
	}
	_, wrongPriv, err := ParseServiceToken(v.RecipientToken)
	if err != nil {
		t.Fatalf("ParseServiceToken (wrong): %v", err)
	}
	if _, err := UnwrapDEK(v.WrappedDEK, wrongPriv); err == nil {
		t.Error("UnwrapDEK with wrong key unexpectedly succeeded")
	}

	// Flip one byte of the ciphertext segment.
	parts := []byte(v.WrappedDEK)
	parts[len(parts)-1] ^= 0x01
	if _, err := UnwrapDEK(string(parts), priv); err == nil {
		t.Error("UnwrapDEK of tampered blob unexpectedly succeeded")
	}
}

// TestSelfRoundTrip proves a Go-minted token is internally consistent: it
// parses, hashes back to its own hash, and can wrap/unwrap its own DEK.
func TestSelfRoundTrip(t *testing.T) {
	created, err := CreateServiceToken()
	if err != nil {
		t.Fatalf("CreateServiceToken: %v", err)
	}
	if HashToken(created.Token) != created.TokenHash {
		t.Error("self hash mismatch")
	}
	tokenID, priv, err := ParseServiceToken(created.Token)
	if err != nil {
		t.Fatalf("ParseServiceToken: %v", err)
	}
	if tokenID != created.TokenID {
		t.Errorf("parsed id %q != created id %q", tokenID, created.TokenID)
	}
	gotJWK, err := PublicKeyJWK(priv)
	if err != nil {
		t.Fatalf("PublicKeyJWK: %v", err)
	}
	if !jwkEqual(t, gotJWK, created.PublicKeyJWK) {
		t.Error("self public JWK mismatch")
	}

	dek, err := GenerateDEK()
	if err != nil {
		t.Fatalf("GenerateDEK: %v", err)
	}
	wrapped, err := WrapDEK(dek, created.PublicKeyJWK)
	if err != nil {
		t.Fatalf("WrapDEK: %v", err)
	}
	got, err := UnwrapDEK(wrapped, priv)
	if err != nil {
		t.Fatalf("UnwrapDEK: %v", err)
	}
	if !bytes.Equal(got, dek) {
		t.Error("self wrap round trip mismatch")
	}
}

// TestGoPKCS8IsWebCryptoCompatible proves a Go-minted token's PKCS8 encoding is
// byte-identical to what WebCrypto parses/produces: parsing the vector token's
// key and re-marshaling it must reproduce the original PKCS8 bytes. If this
// holds, tokens the Go provider mints are importable by the TS/Rust clients.
func TestGoPKCS8IsWebCryptoCompatible(t *testing.T) {
	v := loadVectors(t)
	// Extract the base64url pkcs8 segment (after the second underscore).
	rest := v.Token[len("skt_"):]
	idx := indexByte(rest, '_')
	if idx < 0 {
		t.Fatal("malformed vector token")
	}
	origPKCS8, err := b64.DecodeString(rest[idx+1:])
	if err != nil {
		t.Fatalf("decode pkcs8: %v", err)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(origPKCS8)
	if err != nil {
		t.Fatalf("ParsePKCS8PrivateKey: %v", err)
	}
	priv, err := toECDH(parsed)
	if err != nil {
		t.Fatalf("toECDH: %v", err)
	}
	remarshaled, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	if !bytes.Equal(origPKCS8, remarshaled) {
		t.Errorf("Go PKCS8 re-marshal differs from WebCrypto's encoding\n orig len=%d\n new  len=%d",
			len(origPKCS8), len(remarshaled))
	}
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// TestSecretDecryptMatchesVectors proves the Go `sc1.` path reads what
// WebCrypto wrote — same AES-GCM layout, same AAD.
func TestSecretDecryptMatchesVectors(t *testing.T) {
	v := loadVectors(t)
	_, priv, err := ParseServiceToken(v.Token)
	if err != nil {
		t.Fatalf("ParseServiceToken: %v", err)
	}
	dek, err := UnwrapDEK(v.WrappedDEK, priv)
	if err != nil {
		t.Fatalf("UnwrapDEK: %v", err)
	}
	got, err := DecryptSecret(dek, v.SecretCiphertext, SecretAAD(v.EnvironmentID, v.SecretName))
	if err != nil {
		t.Fatalf("DecryptSecret: %v", err)
	}
	if got != v.SecretPlaintext {
		t.Errorf("plaintext = %q, want %q", got, v.SecretPlaintext)
	}
}

// TestSecretEncryptRoundTrip proves the reverse direction: a blob the Go
// provider writes decrypts under the same DEK and AAD. (Byte-equality with the
// vector ciphertext is impossible — the IV is random — so the round trip plus
// the decrypt-the-vector test above pin both directions.)
func TestSecretEncryptRoundTrip(t *testing.T) {
	v := loadVectors(t)
	_, priv, err := ParseServiceToken(v.Token)
	if err != nil {
		t.Fatalf("ParseServiceToken: %v", err)
	}
	dek, err := UnwrapDEK(v.WrappedDEK, priv)
	if err != nil {
		t.Fatalf("UnwrapDEK: %v", err)
	}
	aad := SecretAAD(v.EnvironmentID, v.SecretName)
	blob, err := EncryptSecret(dek, v.SecretPlaintext, aad)
	if err != nil {
		t.Fatalf("EncryptSecret: %v", err)
	}
	if !strings.HasPrefix(blob, "sc1.") {
		t.Errorf("blob %q does not carry the sc1. version prefix", blob)
	}
	got, err := DecryptSecret(dek, blob, aad)
	if err != nil {
		t.Fatalf("DecryptSecret: %v", err)
	}
	if got != v.SecretPlaintext {
		t.Errorf("round trip = %q, want %q", got, v.SecretPlaintext)
	}
}

// TestSecretAADIsLoadBearing proves the AAD actually binds a ciphertext to its
// environment + name: decrypting under a different name must fail, not silently
// return the value. This is what stops a blob being moved between secrets.
func TestSecretAADIsLoadBearing(t *testing.T) {
	v := loadVectors(t)
	_, priv, err := ParseServiceToken(v.Token)
	if err != nil {
		t.Fatalf("ParseServiceToken: %v", err)
	}
	dek, err := UnwrapDEK(v.WrappedDEK, priv)
	if err != nil {
		t.Fatalf("UnwrapDEK: %v", err)
	}
	for _, wrong := range []string{
		SecretAAD(v.EnvironmentID, "OTHER_NAME"),
		SecretAAD("env_somewhere_else", v.SecretName),
		"",
	} {
		if _, err := DecryptSecret(dek, v.SecretCiphertext, wrong); err == nil {
			t.Errorf("DecryptSecret with AAD %q unexpectedly succeeded", wrong)
		}
	}
}
