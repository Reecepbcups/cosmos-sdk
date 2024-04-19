package gordian

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/neilotoole/slogt"
	"github.com/rollchains/gordian/gcrypto"
	"github.com/rollchains/gordian/tm/tmapp"
	"github.com/rollchains/gordian/tm/tmconsensus"
	"github.com/rollchains/gordian/tm/tmconsensus/tmconsensustest"
	"github.com/rollchains/gordian/tm/tmengine"
	"github.com/rollchains/gordian/tm/tmgossip"
	"github.com/rollchains/gordian/tm/tmstore/tmmemstore"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/blake2b"
)

func signerFromInsecurePassphrase(insecurePassphrase string) (gcrypto.Ed25519Signer, error) {
	bh, err := blake2b.New(ed25519.SeedSize, nil)
	if err != nil {
		return gcrypto.Ed25519Signer{}, err
	}
	bh.Write([]byte("gordian-echo|"))
	bh.Write([]byte(insecurePassphrase))
	seed := bh.Sum(nil)

	privKey := ed25519.NewKeyFromSeed(seed)

	return gcrypto.NewEd25519Signer(privKey), nil
}

func TestGordianEngine(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log := slogt.New(t, slogt.Text())

	// fx := tmconsensustest.NewStandardFixture(1)
	// genesis := fx.DefaultGenesis()
	initAppState := strings.NewReader(`{"app_state": {"key", "value"}}`)

	var signer gcrypto.Signer
	var err error

	signer, err = signerFromInsecurePassphrase("password")
	require.NoError(t, err)

	var as *tmmemstore.ActionStore
	if signer != nil {
		as = tmmemstore.NewActionStore()
	}

	bs := tmmemstore.NewBlockStore()
	fs := tmmemstore.NewFinalizationStore()
	ms := tmmemstore.NewMirrorStore()
	rs := tmmemstore.NewRoundStore()
	vs := tmmemstore.NewValidatorStore(tmconsensustest.SimpleHashScheme{})

	blockFinCh := make(chan tmapp.FinalizeBlockRequest)
	initChainCh := make(chan tmapp.InitChainRequest)

	vals := make([]tmconsensus.Validator, 1)
	pubKeyBytes, err := hex.DecodeString("b5a700b9ffc2a63b9cceac8c726b9a7d59b9059e3bced6b1fa3832c4551f48a3")
	require.NoError(t, err)

	pubKey, err := gcrypto.NewEd25519PubKey(pubKeyBytes)
	require.NoError(t, err)

	vals[0] = tmconsensus.Validator{
		PubKey: pubKey,
		Power:  1,
	}

	cStrat := &echoConsensusStrategy{
		Log: log.With("sys", "consensus"),
	}
	if signer != nil {
		// No pubkey set in follower mode.
		cStrat.PubKey = signer.PubKey()
	}

	gs := tmgossip.NewChattyStrategy(ctx, log.With("sys", "chattygossip"), nil)

	e, err := tmengine.New(
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

		tmengine.WithConsensusStrategy(cStrat),
		tmengine.WithGossipStrategy(gs),

		tmengine.WithGenesis(&tmconsensus.ExternalGenesis{
			ChainID:           "gordiandemo-echo",
			InitialHeight:     1,
			InitialAppState:   initAppState,
			GenesisValidators: vals,
		}),

		tmengine.WithTimeoutStrategy(ctx, tmengine.LinearTimeoutStrategy{}),

		tmengine.WithBlockFinalizationChannel(blockFinCh),
		tmengine.WithInitChainChannel(initChainCh),

		tmengine.WithSigner(signer),
	)
	require.NoError(t, err)

	defer e.Wait()

	cancel()

}

// generate a mock strat
type echoConsensusStrategy struct {
	Log    *slog.Logger
	PubKey gcrypto.PubKey

	// Round-specific values.
	mu                sync.Mutex
	expProposerPubKey gcrypto.PubKey
	curH              uint64
	curR              uint32
}

func (s *echoConsensusStrategy) EnterRound(ctx context.Context, rv tmconsensus.RoundView, proposalOut chan<- tmconsensus.Proposal) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.curH = rv.Height
	s.curR = rv.Round

	// Pseudo-copy of the modulo round robin proposer selection strategy that the v0.2 code uses.

	expProposerIndex := (int(rv.Height) + int(rv.Round)) % len(rv.Validators)
	s.expProposerPubKey = rv.Validators[expProposerIndex].PubKey
	s.Log.Info("Entering round", "height", rv.Height, "round", rv.Round, "exp_proposer_index", expProposerIndex)

	if s.expProposerPubKey.Equal(s.PubKey) {
		appData := fmt.Sprintf("Height: %d; Round: %d", s.curH, s.curR)
		dataHash := sha256.Sum256([]byte(appData))
		proposalOut <- tmconsensus.Proposal{
			AppDataID: string(dataHash[:]),
		}
		s.Log.Info("Proposing block", "h", s.curH, "r", s.curR)
	}

	return nil
}

func (s *echoConsensusStrategy) ConsiderProposedBlocks(ctx context.Context, pbs []tmconsensus.ProposedBlock) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, pb := range pbs {
		if !s.expProposerPubKey.Equal(pb.ProposerPubKey) {
			continue
		}

		// Found a proposed block from the expected proposer.
		expBlockData := fmt.Sprintf("Height: %d; Round: %d", s.curH, s.curR)
		expDataHash := sha256.Sum256([]byte(expBlockData))

		if !bytes.Equal(pb.Block.DataID, expDataHash[:]) {
			// s.Log.Info("Rejecting proposed block from expected proposer",
			// 	"exp_id", glog.Hex(expDataHash[:]),
			// 	"got_id", glog.Hex(pb.Block.DataID),
			// )
			return "", nil
		}

		if s.PubKey != nil && s.PubKey.Equal(pb.ProposerPubKey) {
			s.Log.Info("Voting on a block that we proposed",
				"h", s.curH, "r", s.curR,
				// "block_hash", glog.Hex(pb.Block.Hash),
			)
		}
		return string(pb.Block.Hash), nil
	}

	// Didn't see a proposed block from the expected proposer.
	return "", tmconsensus.ErrProposedBlockChoiceNotReady
}

func (s *echoConsensusStrategy) ChooseProposedBlock(ctx context.Context, pbs []tmconsensus.ProposedBlock) (string, error) {
	// Follow the ConsiderProposedBlocks logic...
	hash, err := s.ConsiderProposedBlocks(ctx, pbs)
	if err == tmconsensus.ErrProposedBlockChoiceNotReady {
		// ... and if there is no choice ready, then vote nil.
		return "", nil
	}
	return hash, err
}

func (s *echoConsensusStrategy) DecidePrecommit(ctx context.Context, vs tmconsensus.VoteSummary) (string, error) {
	maj := tmconsensus.ByzantineMajority(vs.AvailablePower)
	if pow := vs.PrevoteBlockPower[vs.MostVotedPrevoteHash]; pow >= maj {
		s.Log.Info(
			"Submitting precommit",
			"h", s.curH, "r", s.curR,
			// "block_hash", glog.Hex(vs.MostVotedPrevoteHash),
		)
		return vs.MostVotedPrevoteHash, nil
	}

	// Didn't reach consensus on one block; automatically precommit nil.
	s.Log.Info(
		"Submitting nil precommit",
		"h", s.curH, "r", s.curR,
	)
	return "", nil
}
