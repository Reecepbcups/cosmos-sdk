package ante_test

import (
	"testing"

	cmtcrypto "github.com/cometbft/cometbft/crypto"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
)

// TestSignatureCacheCorrectness asserts the cache never accepts a bad signature
// and stays consistent across repeated lookups. This path is consensus-critical.
func TestSignatureCacheCorrectness(t *testing.T) {
	sc, err := ante.NewSignatureCache(ante.DefaultSignatureCacheSize)
	require.NoError(t, err)

	sk := secp256k1.GenPrivKey()
	pk := sk.PubKey()
	msg := cmtcrypto.CRandBytes(256)
	sig, err := sk.Sign(msg)
	require.NoError(t, err)

	// valid signature: first call misses+verifies, second call hits cache.
	require.True(t, sc.Verify(pk, msg, sig))
	require.True(t, sc.Verify(pk, msg, sig))

	// tampered signature must be rejected and must not be cached.
	badSig := append([]byte(nil), sig...)
	badSig[0] ^= 0xFF
	require.False(t, sc.Verify(pk, msg, badSig))
	require.False(t, sc.Verify(pk, msg, badSig))

	// right signature, wrong message must be rejected.
	require.False(t, sc.Verify(pk, cmtcrypto.CRandBytes(256), sig))

	// right signature+message, wrong pubkey must be rejected.
	other := secp256k1.GenPrivKey().PubKey()
	require.False(t, sc.Verify(other, msg, sig))
}
