package ante_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/testutil/testdata"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	authsign "github.com/cosmos/cosmos-sdk/x/auth/signing"
)

// buildCacheTestTx wires a single-signer valid tx and a cache-enabled antehandler.
func buildCacheTestTx(t *testing.T) (*AnteTestSuite, sdk.Context, sdk.Tx, sdk.AnteHandler, *authsign.SignatureCache) {
	t.Helper()
	suite := SetupTestSuite(t, true)
	suite.txBuilder = suite.clientCtx.TxConfig.NewTxBuilder()
	// non-zero height so the account number is part of the sign bytes.
	suite.ctx = suite.ctx.WithBlockHeight(1).WithIsSigverifyTx(true)

	priv, _, addr := testdata.KeyTestPubAddr()
	acc := suite.accountKeeper.NewAccountWithAddress(suite.ctx, addr)
	require.NoError(t, acc.SetAccountNumber(1000))
	suite.accountKeeper.SetAccount(suite.ctx, acc)

	require.NoError(t, suite.txBuilder.SetMsgs(testdata.NewTestMsg(addr)))
	suite.txBuilder.SetFeeAmount(testdata.NewTestFeeAmount())
	suite.txBuilder.SetGasLimit(testdata.NewTestGasLimit())

	signMode, err := authsign.APISignModeToInternal(suite.clientCtx.TxConfig.SignModeHandler().DefaultMode())
	require.NoError(t, err)
	tx, err := suite.CreateTestTx(
		suite.ctx,
		[]cryptotypes.PrivKey{priv},
		[]uint64{acc.GetAccountNumber()},
		[]uint64{0},
		suite.ctx.ChainID(),
		signMode,
	)
	require.NoError(t, err)

	cache, err := authsign.NewSignatureCache(authsign.DefaultSignatureCacheSize)
	require.NoError(t, err)

	spkd := ante.NewSetPubKeyDecorator(suite.accountKeeper)
	svd := ante.NewSigVerificationDecorator(
		suite.accountKeeper,
		suite.clientCtx.TxConfig.SignModeHandler(),
		ante.WithSignatureCache(cache),
	)
	antehandler := sdk.ChainAnteDecorators(spkd, svd)

	txBytes, err := suite.clientCtx.TxConfig.TxEncoder()(tx)
	require.NoError(t, err)
	ctx := suite.ctx.WithTxBytes(txBytes)
	return suite, ctx, tx, antehandler, cache
}

// TestSigVerificationDecoratorCacheHit runs the same valid tx through the
// decorator twice, modeling CheckTx then FinalizeBlock. The second pass must
// succeed without adding a new cache entry, proving it was served from cache.
func TestSigVerificationDecoratorCacheHit(t *testing.T) {
	_, ctx, tx, antehandler, cache := buildCacheTestTx(t)

	_, err := antehandler(ctx, tx, false) // CheckTx: verify + cache
	require.NoError(t, err)
	require.Equal(t, 1, cache.Len())

	_, err = antehandler(ctx, tx, false) // FinalizeBlock: must hit cache
	require.NoError(t, err)
	require.Equal(t, 1, cache.Len(), "second verify should be a cache hit, not a new entry")
}

// TestSigVerificationDecoratorCacheRejectsBadSig ensures the cache never masks an
// invalid signature: a tampered sig must still be rejected and never cached.
func TestSigVerificationDecoratorCacheRejectsBadSig(t *testing.T) {
	suite, ctx, tx, antehandler, cache := buildCacheTestTx(t)

	sigs, err := tx.(interface {
		GetSignaturesV2() ([]signing.SignatureV2, error)
	}).GetSignaturesV2()
	require.NoError(t, err)

	single, ok := sigs[0].Data.(*signing.SingleSignatureData)
	require.True(t, ok)
	single.Signature[0] ^= 0xFF // tamper

	require.NoError(t, suite.txBuilder.SetSignatures(sigs...))
	badTx := suite.txBuilder.GetTx()

	_, err = antehandler(ctx, badTx, false)
	require.Error(t, err)
	require.Equal(t, 0, cache.Len(), "invalid signature must never be cached")
}
