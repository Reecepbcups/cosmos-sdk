package gordian

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/neilotoole/slogt"
	"github.com/rollchains/gordian/tm/tmapp"
	"github.com/rollchains/gordian/tm/tmconsensus"
	"github.com/rollchains/gordian/tm/tmconsensus/tmconsensustest"
	"github.com/stretchr/testify/require"
)

func TestNewGordianEngine(t *testing.T) {
	log := slogt.New(t, slogt.Text())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// var signer gcrypto.Signer
	// var err error
	signer, err := SignerFromInsecurePassphrase("password")
	require.NoError(t, err)

	vals := make([]tmconsensus.Validator, 1)
	vals[0] = tmconsensus.Validator{
		PubKey: signer.PubKey(),
		Power:  1,
	}

	initAppState := strings.NewReader(`{"app_state": {"key", "value"}}`)

	// cStrat := &echoConsensusStrategy{
	// Log: log.With("sys", "consensus"),
	// }
	// cStrat.PubKey = signer.PubKey() // not follower mode
	cStrat := &tmconsensustest.NopConsensusStrategy{}

	initChainCh := make(chan tmapp.InitChainRequest)
	blockFinCh := make(chan tmapp.FinalizeBlockRequest)

	// TODO: handle in a server_v2 consensus background process
	go func() {
		// this is a  (a *echoApp) background routine for now. no done for now.

		// Assume we always need to initialize the chain at startup.
		select {
		case <-ctx.Done():
			fmt.Println("Stopping due to context cancellation", "cause", context.Cause(ctx))
			return

		case req := <-initChainCh:
			// a.vals = req.Genesis.GenesisValidators
			fmt.Println("InitChainRequest", req)
			fmt.Println("req.Genesis.GenesisValidators", req.Genesis.GenesisValidators)

			// Ignore genesis app state, start with empty state.

			stateHash := sha256.Sum256([]byte(""))
			select {
			case req.Resp <- tmapp.InitChainResponse{
				AppStateHash: stateHash[:],
				// Omitting validators since we want to match the input.
			}:
				// Okay.
			case <-ctx.Done():
				// a.log.Info(
				// 	"Stopping due to context cancellation while attempting to respond to InitChainRequest",
				// 	"cause", context.Cause(ctx),
				// )
				fmt.Println("Stopping due to context cancellation while attempting to respond to InitChainRequest", "cause", context.Cause(ctx))
				return
			}
		}

		cancel()
	}()

	gordianengine, err := NewGordianEngineInstance(
		ctx, log, vals,
		initAppState, cStrat,
		false, "gordiandemo-sdk", initChainCh, blockFinCh,
	)
	require.NoError(t, err)
	defer gordianengine.Wait()

	// print e
	// fmt.Println("engine", gordianengine)
}

/*
func TestGordianEngineOld(t *testing.T) {
	var signer gcrypto.Signer
	var err error

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log := slogt.New(t, slogt.Text())

	// fx := tmconsensustest.NewStandardFixture(1)
	// genesis := fx.DefaultGenesis()
	initAppState := strings.NewReader(`{"app_state": {"key", "value"}}`)

	signer, err = SignerFromInsecurePassphrase("password")
	require.NoError(t, err)

	var as *tmmemstore.ActionStore
	// if signer != nil {
	// 	as = tmmemstore.NewActionStore()
	// }

	bs := tmmemstore.NewBlockStore()
	fs := tmmemstore.NewFinalizationStore()
	ms := tmmemstore.NewMirrorStore()
	rs := tmmemstore.NewRoundStore()
	vs := tmmemstore.NewValidatorStore(tmconsensustest.SimpleHashScheme{})

	blockFinCh := make(chan tmapp.FinalizeBlockRequest)
	initChainCh := make(chan tmapp.InitChainRequest)

	vals := make([]tmconsensus.Validator, 1)

	vals[0] = tmconsensus.Validator{
		PubKey: signer.PubKey(),
		Power:  1,
	}

	cStrat := &echoConsensusStrategy{
		Log: log.With("sys", "consensus"),
	}
	cStrat.PubKey = signer.PubKey() // not follower mode

	gs := tmgossip.NewChattyStrategy(ctx, log.With("sys", "chattygossip"), nil)

	var gordianengine *tmengine.Engine

	// TODO: handle this through server_v2
	go func() {
		// this is a  (a *echoApp) background routine for now. no done for now.

		// Assume we always need to initialize the chain at startup.
		select {
		case <-ctx.Done():
			fmt.Println("Stopping due to context cancellation", "cause", context.Cause(ctx))
			return

		case req := <-initChainCh:
			// a.vals = req.Genesis.GenesisValidators
			fmt.Println("InitChainRequest", req)
			fmt.Println("req.Genesis.GenesisValidators", req.Genesis.GenesisValidators)

			// Ignore genesis app state, start with empty state.

			stateHash := sha256.Sum256([]byte(""))
			select {
			case req.Resp <- tmapp.InitChainResponse{
				AppStateHash: stateHash[:],

				// Omitting validators since we want to match the input.
			}:
				// Okay.
			case <-ctx.Done():
				// a.log.Info(
				// 	"Stopping due to context cancellation while attempting to respond to InitChainRequest",
				// 	"cause", context.Cause(ctx),
				// )
				fmt.Println("Stopping due to context cancellation while attempting to respond to InitChainRequest", "cause", context.Cause(ctx))
				return
			}
		}

		// init chain here from store v2 w/ consensus InitChain ?
		// but how does InitChain hook into the SDK through server_v2?
		// c.InitChain(ctx, &tmapp.InitChainRequest{})

		// call server_v2 to return to respch? (how)

		// icr := make(chan tmapp.InitChainResponse, 1)
		// icr <- tmapp.InitChainResponse{
		// 	AppStateHash: []byte("hash"),
		// 	Validators:   vals,
		// }
		// // print icr
		// result := <-icr
		// fmt.Println(result)

		// write to the initChainCh
		// initChainCh <- tmapp.InitChainRequest{
		// 	Resp: icr,
		// }
		// result := <-icr
		// fmt.Println(result)

		cancel()

	}()

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

		tmengine.WithConsensusStrategy(cStrat),
		tmengine.WithGossipStrategy(gs),

		tmengine.WithGenesis(&tmconsensus.ExternalGenesis{
			ChainID:           "gordiandemo-sdk",
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
	defer gordianengine.Wait()

}

var _ tmconsensus.ConsensusStrategy = (*echoConsensusStrategy)(nil)

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
*/
