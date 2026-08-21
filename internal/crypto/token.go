package crypto

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"strings"
)

// Service tokens are self-contained principals: the token string itself carries
// the private key, so the server never holds it. The server stores only the
// SHA-256 hash of the full token (for authentication) and the public key (for
// wrapping DEK grants).
//
// Format: skt_<22-char base62 id>_<pkcs8 private key, base64url>
// (packages/crypto/src/token.ts).
const (
	tokenPrefix   = "skt"
	tokenIDLength = 22
	idAlphabet    = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

// CreatedServiceToken mirrors the TS CreatedServiceToken: the full secret token
// (shown once), plus the id/hash/public-key the server persists.
type CreatedServiceToken struct {
	// Token is the full secret string, sensitive — the server never sees it.
	Token string
	// TokenID is the public skt_… identifier, safe to store and display.
	TokenID string
	// TokenHash is base64url(SHA-256(token)) — what the server stores for auth.
	TokenHash string
	// PublicKeyJWK is stored server-side to wrap DEK grants for this token.
	PublicKeyJWK string
}

// randomTokenID reproduces token.ts randomTokenId: rejection-sampled base62 of
// length 22, prefixed with "skt_". (The exact RNG need not match the TS output;
// only the alphabet and shape matter for a fresh random id.)
func randomTokenID() (string, error) {
	out := make([]byte, 0, tokenIDLength)
	for len(out) < tokenIDLength {
		buf := make([]byte, tokenIDLength-len(out))
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, bb := range buf {
			if bb < 248 { // avoid modulo bias (248 = 62*4)
				out = append(out, idAlphabet[int(bb)%62])
			}
			if len(out) == tokenIDLength {
				break
			}
		}
	}
	return tokenPrefix + "_" + string(out), nil
}

// HashToken computes base64url(SHA-256(token)) over the UTF-8 token bytes —
// identical to packages/crypto/src/token.ts hashToken.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return b64.EncodeToString(sum[:])
}

// CreateServiceToken mints a fresh ECDH P-256 keypair and assembles the token
// string, hash, and public JWK — the client-side half of POST /v1/orgs/{org}/tokens.
func CreateServiceToken() (CreatedServiceToken, error) {
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return CreatedServiceToken{}, err
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return CreatedServiceToken{}, err
	}
	tokenID, err := randomTokenID()
	if err != nil {
		return CreatedServiceToken{}, err
	}
	token := tokenID + "_" + b64.EncodeToString(pkcs8)
	pubJWK, err := publicKeyJWK(priv.PublicKey())
	if err != nil {
		return CreatedServiceToken{}, err
	}
	return CreatedServiceToken{
		Token:        token,
		TokenID:      tokenID,
		TokenHash:    HashToken(token),
		PublicKeyJWK: pubJWK,
	}, nil
}

// IsServiceToken reports whether a credential is a seekrit service token.
func IsServiceToken(value string) bool {
	return strings.HasPrefix(value, tokenPrefix+"_")
}

// ParseServiceToken splits a token into its id and private key. Only the first
// two underscores are separators — the base64url key segment may itself contain
// underscores — so we split manually rather than on all underscores
// (packages/crypto/src/token.ts parseServiceToken).
func ParseServiceToken(token string) (tokenID string, priv *ecdh.PrivateKey, err error) {
	if !strings.HasPrefix(token, tokenPrefix+"_") {
		return "", nil, fmt.Errorf("not a valid seekrit service token")
	}
	rest := token[len(tokenPrefix)+1:] // after "skt_"
	i := strings.IndexByte(rest, '_')
	if i <= 0 || i == len(rest)-1 {
		return "", nil, fmt.Errorf("not a valid seekrit service token")
	}
	idSuffix, keyB64 := rest[:i], rest[i+1:]
	if !isBase62(idSuffix) {
		return "", nil, fmt.Errorf("not a valid seekrit service token")
	}
	pkcs8, err := b64.DecodeString(keyB64)
	if err != nil {
		return "", nil, fmt.Errorf("service token private key is corrupted: %w", err)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(pkcs8)
	if err != nil {
		return "", nil, fmt.Errorf("service token private key is corrupted: %w", err)
	}
	priv, err = toECDH(parsed)
	if err != nil {
		return "", nil, fmt.Errorf("service token private key is corrupted: %w", err)
	}
	return tokenPrefix + "_" + idSuffix, priv, nil
}

// PublicKeyJWK derives the stored public-key JWK from a token's private key.
func PublicKeyJWK(priv *ecdh.PrivateKey) (string, error) {
	return publicKeyJWK(priv.PublicKey())
}

func isBase62(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z') {
			return false
		}
	}
	return true
}
