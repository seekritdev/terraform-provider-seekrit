package crypto

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"regexp"
	"strings"
)

// Service tokens are self-contained principals: the token string itself carries
// private key alongside an independent random auth secret. Only ServiceTokenAuth
// is sent to the server, which stores its SHA-256 hash and the public key.
//
// Bundle: skt_<22-char base62 id>_v2.<auth secret>.<pkcs8 private key, base64url>
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
	// TokenHash is base64url(SHA-256(wire auth credential)) — what the server stores for auth.
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
	authBytes := make([]byte, 32)
	if _, err := rand.Read(authBytes); err != nil {
		return CreatedServiceToken{}, err
	}
	auth := tokenID + "_v2." + b64.EncodeToString(authBytes)
	token := auth + "." + b64.EncodeToString(pkcs8)
	pubJWK, err := publicKeyJWK(priv.PublicKey())
	if err != nil {
		return CreatedServiceToken{}, err
	}
	return CreatedServiceToken{
		Token:        token,
		TokenID:      tokenID,
		TokenHash:    HashToken(auth),
		PublicKeyJWK: pubJWK,
	}, nil
}

var serviceAuthRE = regexp.MustCompile(`^(skt_[0-9A-Za-z]+_v2\.[A-Za-z0-9_-]{43})(?:\.[A-Za-z0-9_-]+)?$`)

// ServiceTokenAuth returns only the independent authentication credential.
func ServiceTokenAuth(token string) (string, error) {
	m := serviceAuthRE.FindStringSubmatch(token)
	if m == nil {
		return "", fmt.Errorf("a v2 service token is required for API access; replace legacy tokens in the dashboard")
	}
	return m[1], nil
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
	if strings.HasPrefix(keyB64, "v2.") {
		if _, err := ServiceTokenAuth(token); err != nil {
			return "", nil, err
		}
		parts := strings.Split(keyB64, ".")
		if len(parts) != 3 {
			return "", nil, fmt.Errorf("auth credential has no private key")
		}
		keyB64 = parts[2]
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
