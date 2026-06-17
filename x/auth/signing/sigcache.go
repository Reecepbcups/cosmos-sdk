package signing

import (
	"crypto/sha256"

	lru "github.com/hashicorp/golang-lru/v2"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
)

// DefaultSignatureCacheSize is the default number of verified signatures to retain.
// At ~32 bytes/key this is a few MB, enough to cover the mempool->block window
// for a busy chain.
const DefaultSignatureCacheSize = 500_000

// SignatureCache is a read-through cache of signatures that have already been
// verified. The same tx signature is verified once on CheckTx (mempool admission)
// and again during FinalizeBlock; caching lets the second verify become a map
// lookup instead of a full EC operation.
//
// Only known-good signatures are cached. The key folds in the pubkey, the exact
// sign bytes, and the signature, so a forged or mutated signature produces a
// different key and always runs the real verification. The cache can therefore
// never cause a bad signature to be accepted; it can only skip work it has
// already proven safe.
type SignatureCache struct {
	cache *lru.Cache[[32]byte, struct{}]
}

// NewSignatureCache returns a SignatureCache holding up to size entries.
func NewSignatureCache(size int) (*SignatureCache, error) {
	c, err := lru.New[[32]byte, struct{}](size)
	if err != nil {
		return nil, err
	}
	return &SignatureCache{cache: c}, nil
}

// sigCacheKey derives the cache key from the full (pubkey, signBytes, signature)
// tuple. signBytes already encodes account number, chain id, and sequence, so two
// verifications collide only when they are genuinely the same signature over the
// same message by the same key.
func sigCacheKey(pubKey cryptotypes.PubKey, signBytes, sig []byte) [32]byte {
	h := sha256.New()
	h.Write(pubKey.Bytes())
	h.Write(signBytes)
	h.Write(sig)
	var k [32]byte
	copy(k[:], h.Sum(nil))
	return k
}

// Len returns the number of cached signatures. Useful for metrics and tests.
func (sc *SignatureCache) Len() int {
	return sc.cache.Len()
}

// Verify reports whether sig is a valid signature of signBytes by pubKey, using
// the cache to skip the EC operation on a hit. It is safe for concurrent use.
func (sc *SignatureCache) Verify(pubKey cryptotypes.PubKey, signBytes, sig []byte) bool {
	key := sigCacheKey(pubKey, signBytes, sig)
	if _, ok := sc.cache.Get(key); ok {
		return true
	}
	if pubKey.VerifySignature(signBytes, sig) {
		sc.cache.Add(key, struct{}{})
		return true
	}
	return false
}
