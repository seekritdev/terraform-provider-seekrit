/**
 * Emit cross-implementation crypto test vectors using the REAL @seekrit/crypto
 * (WebCrypto) implementation. The Go provider's `internal/crypto` package must
 * reproduce these exactly — this file is the ground truth that proves the Go
 * ECDH / HKDF-SHA256 / AES-GCM wrap path and the `skt_` token scheme are
 * bit-compatible with the browser/CLI (the same discipline apps/run and
 * apps/provisioner use for their Rust ports).
 *
 * Run (from repo root):
 *   pnpm exec tsx apps/terraform-provider-seekrit/testdata/gen-vectors.mts \
 *     > apps/terraform-provider-seekrit/testdata/vectors.json
 */
// Import the crypto source directly by path so this runs without workspace
// module resolution (tsx strips types + resolves the extensionless graph).
import {
  createServiceToken,
  encryptSecret,
  generateDek,
  secretAad,
  toBase64Url,
  wrapDek,
} from "../../../packages/crypto/src/index.ts";

// The principal a Go-created service token authenticates as: the provider parses
// this string, derives its public key, and unwraps DEKs with its private key.
const principal = await createServiceToken();

// A fresh environment DEK, wrapped to the principal — exactly what the API
// returns from `GET /v1/orgs/{org}/envs/{env}/key`. The Go provider must unwrap
// it (to re-wrap for grants) and get back `dek` byte-for-byte.
const dek = generateDek();
const wrappedDek = await wrapDek(dek, principal.publicKeyJwk);

// A second principal — the recipient of a key grant. The Go provider re-wraps
// the DEK to `recipientPublicKeyJwk`; the vector test unwraps the result with
// `recipientToken`'s private key to prove the round-trip.
const recipient = await createServiceToken();

// One secret value encrypted under that DEK, with the real AAD. The Go provider
// writes secrets through the same `sc1.` format, so it must both decrypt this
// blob and produce blobs the TS/Rust clients can decrypt. `environmentId` and
// `secretName` are fixed strings so the AAD is reproducible.
const environmentId = "env_vectors";
const secretName = "DATABASE_URL";
const secretPlaintext = "postgres://user:pw@db.internal:5432/app?sslmode=require";
const secretCiphertext = await encryptSecret(
  dek,
  secretPlaintext,
  secretAad(environmentId, secretName),
);

const vectors = {
  // Token scheme: parse → (tokenId, privateKey); hashToken(token) === tokenHash;
  // publicKeyJwk is derivable from the private key.
  token: principal.token,
  tokenId: principal.tokenId,
  tokenHash: principal.tokenHash,
  publicKeyJwk: principal.publicKeyJwk,
  // DEK wrap/unwrap (the `wd1.` ECIES blob).
  dek: toBase64Url(dek),
  wrappedDek,
  // Grant re-wrap target.
  recipientToken: recipient.token,
  recipientPublicKeyJwk: recipient.publicKeyJwk,
  // Secret value encryption (the `sc1.` blob) and its AAD inputs.
  environmentId,
  secretName,
  secretPlaintext,
  secretCiphertext,
};

process.stdout.write(JSON.stringify(vectors, null, 2));
process.stdout.write("\n");
