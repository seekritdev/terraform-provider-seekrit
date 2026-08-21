package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

// Secret values are AES-256-GCM under the environment DEK, with the AAD binding
// the ciphertext to its location (`<environmentId>/<SECRET_NAME>`) so a blob
// cannot be moved between secrets or environments without the tag failing.
// (packages/crypto/src/aes.ts.)
//
// Blob format: sc1.<iv>.<ciphertext||tag>
const (
	secretPrefix  = "sc1"
	secretIVBytes = 12
)

// SecretAAD is the authenticated context for a secret's ciphertext. It must be
// byte-identical to the TS `secretAad` or every other client fails to decrypt.
func SecretAAD(environmentID, secretName string) string {
	return environmentID + "/" + secretName
}

// EncryptSecret encrypts a plaintext secret value under the environment DEK.
//
// This is the only place the provider handles secret plaintext, and it handles
// it the way every other seekrit client does: in the client, never on the wire.
// The caller must supply the value through a Terraform write-only argument, so
// the plaintext exists for the duration of one apply and is never persisted to
// state (see resource_secret.go).
func EncryptSecret(dek []byte, plaintext, aad string) (string, error) {
	gcm, err := secretCipher(dek)
	if err != nil {
		return "", err
	}
	iv := make([]byte, secretIVBytes)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	// Seal appends the 16-byte tag: ciphertext||tag, matching WebCrypto's layout.
	ciphertext := gcm.Seal(nil, iv, []byte(plaintext), []byte(aad))
	return fmt.Sprintf("%s.%s.%s", secretPrefix, b64.EncodeToString(iv), b64.EncodeToString(ciphertext)), nil
}

// DecryptSecret recovers a secret value from an sc1. blob. Used only by the
// ephemeral resource, whose result Terraform never writes to state or a plan
// file — a managed resource must not call this.
func DecryptSecret(dek []byte, blob, aad string) (string, error) {
	parts, err := splitBlob(blob, secretPrefix, 2)
	if err != nil {
		return "", err
	}
	iv, err := decodeB64(parts[0], "iv")
	if err != nil {
		return "", err
	}
	ciphertext, err := decodeB64(parts[1], "ciphertext")
	if err != nil {
		return "", err
	}
	gcm, err := secretCipher(dek)
	if err != nil {
		return "", err
	}
	if len(iv) != gcm.NonceSize() {
		return "", errors.New("secret decryption failed: bad iv length")
	}
	plaintext, err := gcm.Open(nil, iv, ciphertext, []byte(aad))
	if err != nil {
		return "", errors.New(
			"secret decryption failed: wrong key, tampered data, or mismatched context")
	}
	return string(plaintext), nil
}

func secretCipher(dek []byte) (cipher.AEAD, error) {
	if len(dek) != dekLength {
		return nil, fmt.Errorf("environment DEK must be %d bytes, got %d", dekLength, len(dek))
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
