package qmdb

import (
	"fmt"

	"cosmossdk.io/store/types"
)

// DefaultBatchSize is the number of keys committed per batch during migration.
// Batching bounds peak memory in the pending map.
const DefaultBatchSize = 100_000

// MigrateResult holds the outcome of a store migration.
type MigrateResult struct {
	Store    *Store
	KeyCount int
	Batches  int
}

// MigrateKVStore copies all KV pairs from src into a fresh QMDB store at dir.
// Keys are flushed to disk in batches to limit memory. Use batchSize <= 0
// for DefaultBatchSize.
//
// The progress callback fires after each batch commit with the running total
// of keys migrated.
//
// After migration, call SetInitialVersion(haltHeight) on the returned store
// so subsequent commits resume at the correct chain height.
func MigrateKVStore(src types.KVStore, dir string, batchSize int, progress func(migrated int)) (*MigrateResult, error) {
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}

	store, err := NewStore(dir, true)
	if err != nil {
		return nil, fmt.Errorf("init qmdb at %s: %w", dir, err)
	}

	iter := src.Iterator(nil, nil)
	defer iter.Close()

	var total, inBatch int

	for ; iter.Valid(); iter.Next() {
		store.Set(iter.Key(), iter.Value())
		total++
		inBatch++

		if inBatch >= batchSize {
			store.Commit()
			inBatch = 0
			if progress != nil {
				progress(total)
			}
		}
	}

	// Flush the remaining partial batch.
	if inBatch > 0 {
		store.Commit()
		if progress != nil {
			progress(total)
		}
	} else if total == 0 {
		// Empty store still needs one commit for a valid height.
		store.Commit()
	}

	batches := total / batchSize
	if total%batchSize > 0 || total == 0 {
		batches++
	}

	return &MigrateResult{
		Store:    store,
		KeyCount: total,
		Batches:  batches,
	}, nil
}
