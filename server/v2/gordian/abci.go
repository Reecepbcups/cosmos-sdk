package gordian

import (
	// "context"
	// "errors"
	// "fmt"
	// "sync/atomic"

	// "cosmossdk.io/core/header"
	// consensustypes "github.com/cosmos/cosmos-sdk/x/consensus/types"

	// consensusv1 "cosmossdk.io/api/cosmos/consensus/v1"
	// coreappmgr "cosmossdk.io/core/app"
	// "cosmossdk.io/core/event"
	// "cosmossdk.io/core/store"
	// "cosmossdk.io/core/transaction"
	// errorsmod "cosmossdk.io/errors"
	// "cosmossdk.io/log"
	// "cosmossdk.io/server/v2/appmanager"
	// "cosmossdk.io/server/v2/cometbft/handlers"
	// "cosmossdk.io/server/v2/cometbft/mempool"
	// "cosmossdk.io/server/v2/cometbft/types"
	// cometerrors "cosmossdk.io/server/v2/cometbft/types/errors"
	// "cosmossdk.io/server/v2/streaming"
	// "cosmossdk.io/store/v2/snapshots"
	// abci "github.com/cometbft/cometbft/abci/types"
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"cosmossdk.io/core/header"
	"cosmossdk.io/core/transaction"
	"cosmossdk.io/log"
	"cosmossdk.io/server/v2/appmanager"
	"cosmossdk.io/server/v2/gordian/mempool"
	"cosmossdk.io/server/v2/streaming"
	"cosmossdk.io/store/types"
	"cosmossdk.io/store/v2/snapshots"
	"github.com/rollchains/gordian/tm/tmapp"
	"github.com/rollchains/gordian/tm/tmconsensus"
)

const (
	QueryPathApp   = "app"
	QueryPathP2P   = "p2p"
	QueryPathStore = "store"
)

// var _ abci.Application = (*Consensus[transaction.Tx])(nil)

type Consensus[T transaction.Tx] struct {
	app             *appmanager.AppManager[T]
	cfg             Config
	store           types.Store
	logger          log.Logger
	txCodec         transaction.Codec[T]
	streaming       streaming.Manager
	snapshotManager *snapshots.Manager
	mempool         mempool.Mempool[T]

	// this is only available after this node has committed a block (in FinalizeBlock),
	// otherwise it will be empty and we will need to query the app for the last
	// committed block. TODO(tip): check if concurrency is really needed
	lastCommittedBlock atomic.Pointer[tmconsensus.Block] // TODO: state changes kv pair?

	// prepareProposalHandler handlers.PrepareHandler[T]
	// processProposalHandler handlers.ProcessHandler[T]
	// verifyVoteExt          handlers.VerifyVoteExtensionhandler
	// extendVote             handlers.ExtendVoteHandler

	chainID string
}

func NewConsensus[T transaction.Tx](
	app *appmanager.AppManager[T],
	mp mempool.Mempool[T],
	store types.Store, // TODO: change to types/store.go within Gordian
	cfg Config,
	txCodec transaction.Codec[T],
	logger log.Logger,
) *Consensus[T] {
	return &Consensus[T]{
		mempool: mp,
		store:   store,
		app:     app,
		cfg:     cfg,
		txCodec: txCodec,
		logger:  logger,
	}
}

// var _ abci.Application = (*Consensus[transaction.Tx])(nil) // abci types needed within gordian

// TODO: do I return cometbft abci resp or tmapp InitChainResp?
func (c *Consensus[T]) InitChain(ctx context.Context, req *tmapp.InitChainRequest) (*tmapp.InitChainResponse, error) {
	c.chainID = req.Genesis.ChainID
	initHeight := req.Genesis.InitialHeight
	c.logger.Info("InitChain", "initialHeight", initHeight, "chainID", c.chainID)

	var consMessages []transaction.Type
	// TODO: consensus types is too defined to CometBFT still.
	// if req.ConsensusParams != nil {
	// consMessages = append(consMessages, &consensustypes.MsgUpdateParams{
	// 	Authority: c.cfg.ConsensusAuthority,
	// 	Block:     req.ConsensusParams.Block,
	// 	Evidence:  req.ConsensusParams.Evidence,
	// 	Validator: req.ConsensusParams.Validator,
	// 	Abci:      req.ConsensusParams.Abci,
	// })
	// }

	genesisHeaderInfo := header.Info{
		Height:  int64(initHeight),
		Hash:    nil,
		Time:    time.Now(), // TODO: gordian genesis. Do we really need for block 1?
		ChainID: c.chainID,
		AppHash: nil,
	}

	var appStateBytes []byte
	if _, err := req.Genesis.InitialAppState.Read(appStateBytes); err != nil {
		return nil, fmt.Errorf("error reading initial app state: %w", err)
	}

	fmt.Println(appStateBytes)

	genesisState, err := c.app.InitGenesis(ctx, genesisHeaderInfo, consMessages, appStateBytes)
	if err != nil {
		return nil, fmt.Errorf("genesis state init failure: %w", err)
	}

	println(genesisState) // TODO: this needs to be committed to store as height 0.

	return &tmapp.InitChainResponse{
		AppStateHash: []byte{},
		Validators:   req.Genesis.GenesisValidators,
	}, nil
}

func FinalizeBlock(ctx context.Context, req *tmapp.FinalizeBlockRequest) (*tmapp.FinalizeBlockResponse, error) {
	return nil, nil
}
