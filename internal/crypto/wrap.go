package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
)

// ECIES-style key wrapping: an ephemeral P-256 keypair performs ECDH against the
// recipient's public key; the shared secret is run through HKDF-SHA256 to derive
// a one-time AES-256-GCM wrapping key. Only the holder of the recipient private
// key can unwrap. (packages/crypto/src/wrap.ts.)
//
// Blob format: wd1.<ephemeral pub (raw SEC1)>.<hkdf salt>.<iv>.<ciphertext||tag>
const (
	wrapPrefix = "wd1"
	hkdfInfo   = "seekrit/wrap-dek/v1"
	dekLength  = 32 // AES-256 environment DEK
	saltLength = 16
	ivLength   = 12
)

// GenerateDEK returns a fresh 256-bit environment data-encryption key
// (packages/crypto/src/aes.ts generateDek).
func GenerateDEK() ([]byte, error) {
	dek := make([]byte, dekLength)
	if _, err := rand.Read(dek); err != nil {
		return nil, err
	}
	return dek, nil
}

// deriveWrappingKey performs ECDH then HKDF-SHA256 to produce the one-time
// AES-256 key. The ECDH shared secret is the X-coordinate of the shared point
// (32 bytes for P-256), matching WebCrypto deriveBits(…, 256).
func deriveWrappingKey(own *ecdh.PrivateKey, peer *ecdh.PublicKey, salt []byte) ([]byte, error) {
	shared, err := own.ECDH(peer)
	if err != nil {
		return nil, err
	}
	return hkdf.Key(sha256.New, shared, salt, hkdfInfo, 32)
}

// WrapDEK wraps a DEK to a principal's public key (JWK string), producing a
// wd1. blob. Used when creating an environment (wrap to self) or granting env
// access to another principal (wrap to the recipient).
func WrapDEK(dek []byte, recipientPublicKeyJWK string) (string, error) {
	recipient, err := importPublicKeyJWK(recipientPublicKeyJWK)
	if err != nil {
		return "", err
	}
	ephemeral, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	wrappingKey, err := deriveWrappingKey(ephemeral, recipient, salt)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(wrappingKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	iv := make([]byte, ivLength)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	// Seal appends the 16-byte tag: ciphertext||tag, matching WebCrypto's layout.
	ciphertext := gcm.Seal(nil, iv, dek, nil)
	ephemeralRaw := ephemeral.PublicKey().Bytes() // raw SEC1 uncompressed point

	return fmt.Sprintf("%s.%s.%s.%s.%s",
		wrapPrefix,
		b64.EncodeToString(ephemeralRaw),
		b64.EncodeToString(salt),
		b64.EncodeToString(iv),
		b64.EncodeToString(ciphertext),
	), nil
}

// UnwrapDEK recovers a DEK from a wd1. blob using the principal's private key.
func UnwrapDEK(wrapped string, priv *ecdh.PrivateKey) ([]byte, error) {
	parts, err := splitBlob(wrapped, wrapPrefix, 4)
	if err != nil {
		return nil, err
	}
	ephRaw, err := decodeB64(parts[0], "ephemeral public key")
	if err != nil {
		return nil, err
	}
	salt, err := decodeB64(parts[1], "hkdf salt")
	if err != nil {
		return nil, err
	}
	iv, err := decodeB64(parts[2], "iv")
	if err != nil {
		return nil, err
	}
	ciphertext, err := decodeB64(parts[3], "ciphertext")
	if err != nil {
		return nil, err
	}
	ephemeral, err := ecdh.P256().NewPublicKey(ephRaw)
	if err != nil {
		return nil, fmt.Errorf("invalid ephemeral public key: %w", err)
	}
	wrappingKey, err := deriveWrappingKey(priv, ephemeral, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(wrappingKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(iv) != gcm.NonceSize() {
		return nil, errors.New("DEK unwrap failed: bad iv length")
	}
	dek, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return nil, errors.New("DEK unwrap failed: wrong private key or tampered grant")
	}
	return dek, nil
}
