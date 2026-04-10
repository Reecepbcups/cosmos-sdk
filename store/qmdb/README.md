# store/qmdb

Go bridge for [qmdb](https://github.com/LayerZero-Labs/qmdb) - a high-performance verifiable KV store written in Rust. Implements the cosmos-sdk `CommitKVStore` interface as a drop-in replacement for IAVL.

## How it works

- **Writes** buffer in a Go map, then flush to qmdb on `Commit()`
- **Reads** hit a Go btree index first (zero-alloc), fall back to Rust FFI for cold data
- **Iteration** uses the btree index directly (no FFI)
- **Commits** cross the CGo boundary to qmdb's SeqAds engine which handles Merkle hashing and disk persistence

## Setup

Requires Rust toolchain (stable). The Makefile handles cloning and building qmdb automatically.

```bash
# build the Rust library
make build

# run tests
make test

# run benchmarks
make bench
```

## Migrating from IAVL

This is a halt-and-migrate. Stop the chain at a known height, replay all KV data into QMDB, restart. No live cutover, no dual-write.

### The short version

```go
import "cosmossdk.io/store/qmdb"

// for each substore...
result, err := qmdb.MigrateKVStore(iavlStore, "/data/qmdb/bank", 0, func(n int) {
    fmt.Printf("  %d keys migrated\n", n)
})
result.Store.SetInitialVersion(haltHeight)
```

`MigrateKVStore` iterates every KV pair from the source store and replays it into a fresh QMDB instance. It batches commits (default 100k keys) to keep memory bounded. After migration, `SetInitialVersion` tells QMDB what chain height to resume from.

### Full migration flow

1. Halt the chain at height H via upgrade handler
2. For each substore (bank, staking, gov, etc):
   - Iterate all KV pairs from IAVL
   - Write them into a fresh QMDB store
   - Commit
   - Call `SetInitialVersion(H)` so the next commit produces H+1
3. Swap store type from `StoreTypeIAVL` to `StoreTypeQMDB` in rootmulti
4. Restart

Typical chain at ~1.2M blocks has low millions of KV pairs total. It's IO-bound. Expect minutes on decent hardware.

### What SetInitialVersion does

QMDB's Rust engine tracks an internal block height. After migration, that height is 1 (or however many batch commits happened). `SetInitialVersion(H)` overrides this via FFI so the next `Commit()` produces height H+1.

Caveat: only the in-memory height gets updated immediately. It persists to disk on the next `commit_block` call. So don't crash between `SetInitialVersion` and the first real `Commit`.

### Example upgrade handler

This is a sketch. Actual implementation depends on how rootmulti wiring lands.

```go
func migrateIAVLToQMDB(ctx sdk.Context, app *App) error {
    for _, storeKey := range app.GetStoreKeys() {
        src := app.CommitMultiStore().GetCommitKVStore(storeKey)

        dir := filepath.Join(app.HomeDir(), "data", "qmdb", storeKey.Name())
        result, err := qmdb.MigrateKVStore(src, dir, 0, func(n int) {
            fmt.Printf("[%s] migrated %d keys\n", storeKey.Name(), n)
        })
        if err != nil {
            return fmt.Errorf("migrate %s: %w", storeKey.Name(), err)
        }
        result.Store.SetInitialVersion(ctx.BlockHeight())
    }
    return nil
}
```

### Gotchas

- **create vs write**: QMDB distinguishes between creating a new key and updating an existing one. During migration everything is a `create` since the DB is fresh. `Commit()` handles this automatically by checking the btree index.

- **Btree after restart**: after migration the btree has all keys (they were just Set'd). But after a restart you'll need FFI iteration to rebuild it. That's work item #1 below.

- **Halt-height only**: every validator must migrate at the same height. No rolling upgrades, no gradual migration. Consensus requires identical state.

## What's still needed

Roughly in priority order:

1. **FFI iteration** - btree index rebuild on restart. Without this, every read after restart falls back to slow FFI (~2000ns)
2. **Btree rebuild on open** - use FFI iteration at startup to repopulate the Go btree
3. **Export/Import** - state sync so new validators can join post-migration
4. **Wire into rootmulti** - add `StoreTypeQMDB` case in `loadCommitStoreFromParams()`
5. **Migration handler** - the actual upgrade module code that does IAVL->QMDB
6. **Pruning** - `DeleteVersionsTo()` to prevent unbounded disk growth
7. **Proofs** - ICS23 proof translation if IBC/light clients need proofs from your chain

### Interfaces rootmulti expects

| Method | Where | Status |
|-|-|-|
| `CommitKVStore` (Get/Set/Delete/Commit/Iterator) | Everywhere | Done |
| `StoreWithInitialVersion` | Migration, new stores | Done |
| `Export(version)` / `Import(version)` | State sync (rootmulti ~858, ~947) | Not yet |
| `GetImmutable(version)` | Historical queries (rootmulti ~594) | Not yet |
| `DeleteVersionsTo(version)` | Pruning (rootmulti ~700) | Not yet |
| `LoadVersionForOverwriting(target)` | Rollback CLI (rootmulti ~1082) | Not yet |
| `GetVersioned(key, version)` | Historical ABCI queries | Not yet |
| `Query()` with Merkle proofs | ABCI proof generation | Not yet |

## Benchmarks (i9-13900K)

IAVL uses in-memory MemDB (best case). QMDB uses real disk.

### Get (ns/op) - scaling with dataset size

| Keys | IAVL | QMDB | Notes |
|-|-|-|-|
| 1K | 69 | 85 | IAVL 1.2x faster |
| 10K | 61 | 111 | IAVL 1.8x faster |
| 100K | 76 | 125 | IAVL 1.6x faster |
| 500K | 414 | 149 | **QMDB 2.8x faster** |

IAVL's AVL tree starts cache-thrashing around 500K entries. The btree stays flat.

### Has (ns/op) - QMDB dominates at all sizes

| Keys | IAVL | QMDB |
|-|-|-|
| 1K | 705 (10 allocs) | 80 (0 allocs) |
| 100K | 801 (10 allocs) | 121 (0 allocs) |
| 500K | 940 (11 allocs) | 149 (0 allocs) |

### Set (ns/op) - QMDB 4-5x faster at all sizes

| Keys | IAVL | QMDB |
|-|-|-|
| 1K | 1827 (26 allocs) | 358 (1 alloc) |
| 500K | 1818 (27 allocs) | 411 (1 alloc) |

### Iterator 1K entries

| | IAVL | QMDB |
|-|-|-|
| ns/op | 160,384 | 4,880 |
| allocs | 2,016 | 2 |

**33x faster**, zero-alloc btree iteration.

## Limitations

- **Cache misses**: reads for keys not in the btree index (after restart, or keys that don't exist) fall back to Rust FFI at ~2000ns
- **Recovery**: `SeqAds` reopen after close needs investigation (twig file recovery)
- **No ICS23 proofs yet**: proof translation from qmdb's ProofPath to ICS23 isn't implemented
- **Memory**: the btree index costs ~212 bytes/key. At 10M keys that's ~2GB

## Architecture

```
store.go          CommitKVStore implementation + CGo bindings
migrate.go        IAVL->QMDB migration logic (MigrateKVStore)
iterator.go       btree-backed range iteration (pure Go)
ffi/              Rust FFI crate source (copied into cloned qmdb on build)
  src/lib.rs      extern "C" shim wrapping qmdb's SeqAds
  qmdb_ffi.h      C header
  Cargo.toml      Rust crate manifest
Makefile          clone + build + test
```
