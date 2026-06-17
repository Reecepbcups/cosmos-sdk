package signing_test

import (
	"testing"

	cmtcrypto "github.com/cometbft/cometbft/crypto"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
)

// BenchmarkSigCache models the real cost the SDK pays today: every tx signature
// is verified once on CheckTx and again during FinalizeBlock. The "no_cache"
// case does two full EC verifies per tx; the "with_cache" case does one verify
// plus one cache hit. Each iteration uses a fresh signature so the cached lookup
// is always a genuine first->second verify of the same tx (timer paused during
// signing setup).
func BenchmarkSigCache(b *testing.B) {
	require := require.New(b)
	sk := secp256k1.GenPrivKey()
	pk := sk.PubKey()

	b.Run("no_cache_double_verify", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			msg := cmtcrypto.CRandBytes(256)
			sig, err := sk.Sign(msg)
			require.NoError(err)
			b.StartTimer()

			require.True(pk.VerifySignature(msg, sig)) // CheckTx
			require.True(pk.VerifySignature(msg, sig)) // FinalizeBlock
		}
	})

	b.Run("with_cache_double_verify", func(b *testing.B) {
		b.ReportAllocs()
		sc, err := authsigning.NewSignatureCache(b.N + 1)
		require.NoError(err)
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			msg := cmtcrypto.CRandBytes(256)
			sig, err := sk.Sign(msg)
			require.NoError(err)
			b.StartTimer()

			require.True(sc.Verify(pk, msg, sig)) // CheckTx: miss -> verify
			require.True(sc.Verify(pk, msg, sig)) // FinalizeBlock: hit
		}
	})

	b.Run("cache_hit_only", func(b *testing.B) {
		b.ReportAllocs()
		sc, err := authsigning.NewSignatureCache(16)
		require.NoError(err)
		msg := cmtcrypto.CRandBytes(256)
		sig, err := sk.Sign(msg)
		require.NoError(err)
		require.True(sc.Verify(pk, msg, sig)) // prime
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			sc.Verify(pk, msg, sig)
		}
	})
}
