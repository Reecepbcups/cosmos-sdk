package gordian

import (
	"context"
	"testing"

	appmodulev2 "cosmossdk.io/core/appmodule/v2"
	"cosmossdk.io/core/transaction"
	appmanager "cosmossdk.io/server/v2/appmanager"
	ammstore "cosmossdk.io/server/v2/appmanager/store"
	"cosmossdk.io/server/v2/gordian/mempool"
	"cosmossdk.io/server/v2/stf"
	"cosmossdk.io/server/v2/stf/branch"
	"cosmossdk.io/server/v2/stf/mock"
	"cosmossdk.io/store/v2/storage/pebbledb"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestGordian(t *testing.T) {
	mockTx := mock.Tx{
		Sender:   []byte("sender"),
		Msg:      wrapperspb.Bool(true), // msg does not matter at all because our handler does nothing.
		GasLimit: 100_000,
	}
	_ = mockTx

	s := stf.NewSTF(
		func(ctx context.Context, msg transaction.Type) (msgResp transaction.Type, err error) {
			// err = kvSet(t, ctx, "exec")
			return nil, err
		},
		func(ctx context.Context, msg transaction.Type) (msgResp transaction.Type, err error) {
			// err = kvSet(t, ctx, "query")
			return nil, err
		},
		func(ctx context.Context, txs []mock.Tx) error { return nil },
		func(ctx context.Context) error {
			// return kvSet(t, ctx, "begin-block")
			return nil
		},
		func(ctx context.Context) error {
			// return kvSet(t, ctx, "end-block")
			return nil
		},
		func(ctx context.Context, tx mock.Tx) error {
			// return kvSet(t, ctx, "validate")
			return nil
		},
		func(ctx context.Context) ([]appmodulev2.ValidatorUpdate, error) { return nil, nil },
		func(ctx context.Context, tx mock.Tx, success bool) error {
			// return kvSet(t, ctx, "post-tx-exec")
			return nil
		},
		branch.DefaultNewWriterMap,
	)

	storageDB, err := pebbledb.New(t.TempDir())
	require.NoError(t, err)
	ss, _ := ammstore.New(storageDB)

	b := appmanager.Builder[mock.Tx]{
		STF:                s,
		DB:                 ss,
		ValidateTxGasLimit: 100_000,
		QueryGasLimit:      100_000,
		SimulationGasLimit: 100_000,
	}

	am, err := b.Build()
	require.NoError(t, err)

	mockStore := NewMockStore()

	c := NewConsensus[mock.Tx](am, mempool.NoOpMempool[mock.Tx]{}, mockStore, Config{}, mock.TxCodec{}, nil)
	_ = c
	// c.app.InitGenesis()
}
