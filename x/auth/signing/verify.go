package signing

import (
	"context"
	"fmt"

	signingv1beta1 "cosmossdk.io/api/cosmos/tx/signing/v1beta1"
	txsigning "cosmossdk.io/x/tx/signing"

	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/crypto/types/multisig"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
)

// APISignModesToInternal converts a protobuf SignMode array to a signing.SignMode array.
func APISignModesToInternal(modes []signingv1beta1.SignMode) ([]signing.SignMode, error) {
	internalModes := make([]signing.SignMode, len(modes))
	for i, mode := range modes {
		internalMode, err := APISignModeToInternal(mode)
		if err != nil {
			return nil, err
		}
		internalModes[i] = internalMode
	}
	return internalModes, nil
}

// APISignModeToInternal converts a protobuf SignMode to a signing.SignMode.
func APISignModeToInternal(mode signingv1beta1.SignMode) (signing.SignMode, error) {
	switch mode {
	case signingv1beta1.SignMode_SIGN_MODE_DIRECT:
		return signing.SignMode_SIGN_MODE_DIRECT, nil
	case signingv1beta1.SignMode_SIGN_MODE_LEGACY_AMINO_JSON:
		return signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON, nil
	case signingv1beta1.SignMode_SIGN_MODE_TEXTUAL:
		return signing.SignMode_SIGN_MODE_TEXTUAL, nil
	case signingv1beta1.SignMode_SIGN_MODE_DIRECT_AUX:
		return signing.SignMode_SIGN_MODE_DIRECT_AUX, nil
	default:
		return signing.SignMode_SIGN_MODE_UNSPECIFIED, fmt.Errorf("unsupported sign mode %s", mode)
	}
}

// internalSignModeToAPI converts a signing.SignMode to a protobuf SignMode.
func internalSignModeToAPI(mode signing.SignMode) (signingv1beta1.SignMode, error) {
	switch mode {
	case signing.SignMode_SIGN_MODE_DIRECT:
		return signingv1beta1.SignMode_SIGN_MODE_DIRECT, nil
	case signing.SignMode_SIGN_MODE_LEGACY_AMINO_JSON:
		return signingv1beta1.SignMode_SIGN_MODE_LEGACY_AMINO_JSON, nil
	case signing.SignMode_SIGN_MODE_TEXTUAL:
		return signingv1beta1.SignMode_SIGN_MODE_TEXTUAL, nil
	case signing.SignMode_SIGN_MODE_DIRECT_AUX:
		return signingv1beta1.SignMode_SIGN_MODE_DIRECT_AUX, nil
	default:
		return signingv1beta1.SignMode_SIGN_MODE_UNSPECIFIED, fmt.Errorf("unsupported sign mode %s", mode)
	}
}

// VerifySignature verifies a transaction signature contained in SignatureData abstracting over different signing
// modes. It differs from VerifySignature in that it uses the new txsigning.TxData interface in x/tx.
func VerifySignature(
	ctx context.Context,
	pubKey cryptotypes.PubKey,
	signerData txsigning.SignerData,
	signatureData signing.SignatureData,
	handler *txsigning.HandlerMap,
	txData txsigning.TxData,
) error {
	return VerifySignatureWithCache(ctx, pubKey, signerData, signatureData, handler, txData, nil)
}

// VerifySignatureWithCache behaves like VerifySignature but, when cache is non-nil,
// consults it to skip the EC verification for a single signature that has already
// been verified. A nil cache makes this identical to VerifySignature. The cache is
// only applied to single-signer signatures; multisig always runs the full path.
func VerifySignatureWithCache(
	ctx context.Context,
	pubKey cryptotypes.PubKey,
	signerData txsigning.SignerData,
	signatureData signing.SignatureData,
	handler *txsigning.HandlerMap,
	txData txsigning.TxData,
	cache *SignatureCache,
) error {
	switch data := signatureData.(type) {
	case *signing.SingleSignatureData:
		signMode, err := internalSignModeToAPI(data.SignMode)
		if err != nil {
			return err
		}
		signBytes, err := handler.GetSignBytes(ctx, signMode, signerData, txData)
		if err != nil {
			return err
		}
		if !verifyMaybeCached(cache, pubKey, signBytes, data.Signature) {
			return fmt.Errorf("unable to verify single signer signature")
		}
		return nil

	case *signing.MultiSignatureData:
		multiPK, ok := pubKey.(multisig.PubKey)
		if !ok {
			return fmt.Errorf("expected %T, got %T", (multisig.PubKey)(nil), pubKey)
		}
		err := multiPK.VerifyMultisignature(func(mode signing.SignMode) ([]byte, error) {
			signMode, err := internalSignModeToAPI(mode)
			if err != nil {
				return nil, err
			}
			return handler.GetSignBytes(ctx, signMode, signerData, txData)
		}, data)
		if err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unexpected SignatureData %T", signatureData)
	}
}

// verifyMaybeCached runs the signature verification through cache when provided,
// otherwise it performs the direct EC verification.
func verifyMaybeCached(cache *SignatureCache, pubKey cryptotypes.PubKey, signBytes, sig []byte) bool {
	if cache != nil {
		return cache.Verify(pubKey, signBytes, sig)
	}
	return pubKey.VerifySignature(signBytes, sig)
}
