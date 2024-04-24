package gordian

import (
	"context"
	"crypto/ed25519"
	"io"
	"log/slog"

	"github.com/rollchains/gordian/gcrypto"
	"github.com/rollchains/gordian/tm/tmapp"
	"github.com/rollchains/gordian/tm/tmconsensus"
	"github.com/rollchains/gordian/tm/tmconsensus/tmconsensustest"
	"github.com/rollchains/gordian/tm/tmengine"
	"github.com/rollchains/gordian/tm/tmgossip"
	"github.com/rollchains/gordian/tm/tmstore/tmmemstore"
	"golang.org/x/crypto/blake2b"
)

const ConsensusPrefix = "gordian|"

// This may make sense upstream to Gordian
func NewGordianEngineInstance(
	ctx context.Context,
	log *slog.Logger,
	vals []tmconsensus.Validator,
	initAppState io.Reader,
	cs tmconsensus.ConsensusStrategy,
	followerMode bool, // i.e. no signer / signer == nil
	chainId string,
	initChainCh chan tmapp.InitChainRequest,
	blockFinCh chan tmapp.FinalizeBlockRequest,
) (*tmengine.Engine, error) {
	var gordianengine *tmengine.Engine
	var signer gcrypto.Signer
	var err error

	// fx := tmconsensustest.NewStandardFixture(1)
	// genesis := fx.DefaultGenesis()
	// initAppState := strings.NewReader(`{"app_state": {"key", "value"}}`)

	signer, err = SignerFromInsecurePassphrase("password")
	if err != nil {
		return gordianengine, err
	}

	var as *tmmemstore.ActionStore
	if !followerMode {
		as = tmmemstore.NewActionStore()
	}

	bs := tmmemstore.NewBlockStore()
	fs := tmmemstore.NewFinalizationStore()
	ms := tmmemstore.NewMirrorStore()
	rs := tmmemstore.NewRoundStore()
	vs := tmmemstore.NewValidatorStore(tmconsensustest.SimpleHashScheme{})

	gs := tmgossip.NewChattyStrategy(ctx, log.With("sys", "chattygossip"), nil)

	gordianengine, err = tmengine.New(
		ctx,
		log.With("sys", "engine"),
		tmengine.WithActionStore(as),
		tmengine.WithBlockStore(bs),
		tmengine.WithFinalizationStore(fs),
		tmengine.WithMirrorStore(ms),
		tmengine.WithRoundStore(rs),
		tmengine.WithValidatorStore(vs),

		tmengine.WithHashScheme(tmconsensustest.SimpleHashScheme{}),
		tmengine.WithSignatureScheme(tmconsensustest.SimpleSignatureScheme{}),
		tmengine.WithCommonMessageSignatureProofScheme(gcrypto.SimpleCommonMessageSignatureProofScheme),

		tmengine.WithConsensusStrategy(cs),
		tmengine.WithGossipStrategy(gs),

		tmengine.WithGenesis(&tmconsensus.ExternalGenesis{
			ChainID:           chainId,
			InitialHeight:     1,
			InitialAppState:   initAppState,
			GenesisValidators: vals,
		}),

		tmengine.WithTimeoutStrategy(ctx, tmengine.LinearTimeoutStrategy{}),

		tmengine.WithBlockFinalizationChannel(blockFinCh),
		tmengine.WithInitChainChannel(initChainCh),

		tmengine.WithSigner(signer),
	)

	// Make sure to `defer gordianengine.Wait()`
	return gordianengine, err
}

func SignerFromInsecurePassphrase(insecurePassphrase string) (gcrypto.Ed25519Signer, error) {
	bh, err := blake2b.New(ed25519.SeedSize, nil)
	if err != nil {
		return gcrypto.Ed25519Signer{}, err
	}
	bh.Write([]byte(ConsensusPrefix))
	bh.Write([]byte(insecurePassphrase))
	seed := bh.Sum(nil)

	privKey := ed25519.NewKeyFromSeed(seed)

	return gcrypto.NewEd25519Signer(privKey), nil
}
