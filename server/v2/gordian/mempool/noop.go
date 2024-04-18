package mempool

import (
	"context"

	"cosmossdk.io/core/transaction"
)

var _ Mempool[transaction.Tx] = (*NoOpMempool[transaction.Tx])(nil)

// NoOpMempool defines a no-op mempool. Transactions are completely discarded and
// ignored when BaseApp interacts with the mempool.
//
// Note: When this mempool is used, it assumed that an application will rely
// on CometBFT's transaction ordering defined in `RequestPrepareProposal`, which
// is FIFO-ordered by default.
type NoOpMempool[T transaction.Tx] struct{}

// Insert implements Mempool.
func (n NoOpMempool[T]) Insert(context.Context, T) error {
	return nil
}

// Remove implements Mempool.
func (n NoOpMempool[T]) Remove([]T) error {
	return nil
}

// Select implements Mempool.
func (n NoOpMempool[T]) Select(context.Context, []T) Iterator[T] {
	return nil
}
