package baseapp_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
	"time"

	// abci "github.com/cometbft/cometbft/abci/types"
	"github.com/stretchr/testify/require"

	"github.com/strangelove-ventures/gordian/gordiantest"
	"github.com/strangelove-ventures/gordian/tm/tmapp"
	"github.com/strangelove-ventures/gordian/tm/tmconsensus"
	"github.com/strangelove-ventures/gordian/tm/tmconsensus/tmconsensustest"
	"github.com/strangelove-ventures/gordian/tm/tmengine/tmenginetest"
	"github.com/strangelove-ventures/gordian/tm/tmengine2"
	"github.com/strangelove-ventures/gordian/tm/tmstore"

	// "github.com/strangelove-ventures/gordian/tm/tmengine2/internal/tmstate2/tmstate2test"
	"github.com/strangelove-ventures/gordian/tm/tmgossip/tmgossiptest"
	"github.com/strangelove-ventures/gordian/tm/tmstore2"
)

func TestGordianABCI(t *testing.T) {
	// TODO: use Gordian hooks
	// suite := NewBaseAppSuite(t)

	logger := slog.Default()
	_, cancel, e := setupEngine(t, logger)
	defer e.Wait()
	defer cancel()

	// TODO: Impl InitChainRequests to baseapp somehow? how
	require.NotNil(t, e.InitChainRequests())

	req := gordiantest.ReceiveOrTimeout(t, e.InitChainRequests(), 100*time.Millisecond)
	require.Equal(t, "my-chain", req.Genesis.ChainID)

	// reqInfo := abci.RequestInfo{}
	// res, err := suite.baseApp.Info(&reqInfo)
	// require.NoError(t, err)

	// require.Equal(t, "", res.Version)
	// require.Equal(t, t.Name(), res.GetData())
	// require.Equal(t, int64(0), res.LastBlockHeight)
	// require.Equal(t, []uint8(nil), res.LastBlockAppHash)
	// require.Equal(t, suite.baseApp.AppVersion(), res.AppVersion)
}

func setupEngine(t *testing.T, logger *slog.Logger) (context.Context, context.CancelFunc, *tmengine2.Engine) {
	ctx, cancel := context.WithCancel(context.Background())

	fx := tmconsensustest.NewStandardFixture(2)
	bs := fx.NewMemBlockStore(ctx)
	cs := tmstore2.NewMemConsensusStore(
		ctx,
		fx.DefaultGenesis(),
		fx.HashScheme,
		fx.SignatureScheme,
		fx.CommonMessageSignatureProofScheme,
	)
	tm := tmenginetest.NewTimeoutManager()
	gStrat := tmgossiptest.NewMockGossipStrategy(fx.CommonMessageSignatureProofScheme)

	psStrat := tmconsensustest.ConstantProposerStrategy(fx.PrivVals[0].CVal)

	// consStrat := tmstate2test.NewNilConsStrategy()

	e, err := tmengine2.New(
		ctx,
		logger,
		tmengine2.WithConsensusStore(cs),
		tmengine2.WithBlockStore(bs),
		tmengine2.WithHashScheme(fx.HashScheme),
		tmengine2.WithSignatureScheme(fx.SignatureScheme),
		tmengine2.WithCommonMessageSignatureProofScheme(fx.CommonMessageSignatureProofScheme),
		tmengine2.WithTimeoutManager(tm),
		// tmengine2.WithConsensusStrategy(consStrat),
		tmengine2.WithGossipStrategy(gStrat),
		tmengine2.WithProposerSelectionStrategy(psStrat),
	)
	require.NoError(t, err)

	return ctx, cancel, e
}

// TODO:
// - create a new engine
// - add the ability to echo between gordian & baseapp (impl the tmengine.Hooks)
func TestGordianBaseAppIntegration(t *testing.T) {
	logger := slog.Default()
	// ctx := sdk.Context{}.WithContext(context.Background())

	// ctx, cancel := context.WithCancel(context.Background())
	// defer cancel()

	t.Run("channel is nil when stores are already initialized", func(t *testing.T) {
		t.Parallel()

		_, cancel, e := setupEngine(t, logger)
		defer e.Wait()
		defer cancel()

		require.Nil(t, e.InitChainRequests())
	})

	t.Run("responding to InitChain request", func(t *testing.T) {
		t.Parallel()

		fx3 := tmconsensustest.NewStandardFixture(3)
		fx6 := tmconsensustest.NewStandardFixture(6)
		_ = fx6
		for _, tc := range []struct {
			name           string
			respValidators []tmconsensus.Validator
			wantValidators []tmconsensus.Validator
			proposerVal    tmconsensustest.PrivValEd25519
		}{
			{
				name:           "response validators empty",
				respValidators: nil,
				wantValidators: fx3.Vals(),
				proposerVal:    fx3.PrivVals[0],
			},
			{
				name:           "response validators different",
				respValidators: fx6.PrivVals[3:].Vals(),
				wantValidators: fx6.PrivVals[3:].Vals(),
				proposerVal:    fx6.PrivVals[5],
			},
		} {
			tc := tc
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				cs := tmstore2.NewUninitializedMemConsensusStore()
				bs := tmstore.NewUninitializedMemBlockStore()

				fx := tmconsensustest.NewStandardFixture(3)
				appStateReader := new(bytes.Buffer)
				g := tmconsensus.ExternalGenesis{
					ChainID:       "my-chain",
					InitialHeight: 1,

					InitialAppState: appStateReader,

					GenesisValidators: fx.Vals(),
				}

				gStrat := tmgossiptest.NewMockGossipStrategy(fx.CommonMessageSignatureProofScheme)

				psStrat := tmconsensustest.ConstantProposerStrategy(fx.PrivVals[0].CVal)

				// consStrat := tmstate2test.NewNilConsStrategy()

				e, err := tmengine2.New(
					ctx,
					gordiantest.NewLogger(t),
					tmengine2.WithConsensusStore(cs),
					tmengine2.WithBlockStore(bs),
					tmengine2.WithHashScheme(fx.HashScheme),
					tmengine2.WithSignatureScheme(fx.SignatureScheme),
					tmengine2.WithCommonMessageSignatureProofScheme(fx.CommonMessageSignatureProofScheme),
					tmengine2.WithTimeoutManager(tmenginetest.NewTimeoutManager()),
					// tmengine2.WithConsensusStrategy(consStrat),
					tmengine2.WithGossipStrategy(gStrat),
					tmengine2.WithProposerSelectionStrategy(psStrat),
					tmengine2.WithGenesis(&g),
				)
				require.NoError(t, err)
				defer e.Wait()
				defer cancel()

				require.NotNil(t, e.InitChainRequests())

				req := gordiantest.ReceiveOrTimeout(t, e.InitChainRequests(), 100*time.Millisecond)

				require.Equal(t, "my-chain", req.Genesis.ChainID)
				require.Equal(t, uint64(1), req.Genesis.InitialHeight)
				require.Equal(t, fx.Vals(), req.Genesis.GenesisValidators)
				require.Equal(t, appStateReader, req.Genesis.InitialAppState)

				initialAppStateHash := []byte("initial_app_state")
				gordiantest.SendOrTimeout(t, req.Resp, tmapp.InitChainResponse{
					AppStateHash: initialAppStateHash,
					Validators:   tc.respValidators,
				}, 100*time.Millisecond)

				_ = gordiantest.ReceiveOrTimeout(t, cs.Initialized(), 100*time.Millisecond)

				// Currently lack a way to read the chain ID back from the consensus store.
				// But can read the initial height, validators, and app state hash.
				rs, err := cs.HighestRoundState(ctx)
				require.NoError(t, err)
				require.Equal(t, uint64(1), rs.Height)
				require.Zero(t, rs.Round)
				require.Empty(t, rs.ProposedBlocks)

				f, err := cs.BlockFinalization(ctx, 0)
				require.NoError(t, err)
				require.Equal(t, tc.wantValidators, f.NextValidators)
				require.Equal(t, initialAppStateHash, f.AppStateHash)
			})
		}
	})
}
