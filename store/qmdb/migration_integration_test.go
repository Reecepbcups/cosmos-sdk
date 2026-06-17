package qmdb

import (
	"fmt"
	"path/filepath"
	"testing"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log"
	iavlstore "cosmossdk.io/store/iavl"
	"cosmossdk.io/store/metrics"
	"cosmossdk.io/store/types"
)

// TestEndToEndChainMigration simulates a full halt-and-migrate flow:
//
//  1. Create multiple IAVL substores (bank, staking, gov)
//  2. Run several blocks of chain activity
//  3. "Halt" at a known height
//  4. Migrate each substore to QMDB
//  5. Verify all data matches between IAVL and QMDB
//  6. Verify continued commits at the correct height
func TestEndToEndChainMigration(t *testing.T) {
	type substore struct {
		name string
		iavl types.CommitKVStore
	}

	mkStore := func(name string) substore {
		db := dbm.NewMemDB()
		key := types.NewKVStoreKey(name)
		s, err := iavlstore.LoadStore(db, log.NewNopLogger(), key, types.CommitID{}, 128, false, metrics.NewNoOpMetrics())
		require.NoError(t, err)
		return substore{name: name, iavl: s}
	}

	stores := []substore{
		mkStore("bank"),
		mkStore("staking"),
		mkStore("gov"),
	}

	commitAll := func() {
		for _, s := range stores {
			s.iavl.Commit()
		}
	}
	byName := func(name string) types.CommitKVStore {
		for _, s := range stores {
			if s.name == name {
				return s.iavl
			}
		}
		t.Fatalf("store %s not found", name)
		return nil
	}

	// ---- Block 1: genesis state ----
	byName("bank").Set([]byte("balances/cosmos1a"), []byte("1000uatom"))
	byName("bank").Set([]byte("balances/cosmos1b"), []byte("500uatom"))
	byName("bank").Set([]byte("supply/uatom"), []byte("1500"))
	byName("staking").Set([]byte("validators/val1"), []byte(`{"power":100,"moniker":"alice"}`))
	byName("staking").Set([]byte("validators/val2"), []byte(`{"power":50,"moniker":"bob"}`))
	byName("gov").Set([]byte("params/voting_period"), []byte("172800s"))
	byName("gov").Set([]byte("params/quorum"), []byte("0.334"))
	commitAll()

	// ---- Block 2: transfers + delegation ----
	byName("bank").Set([]byte("balances/cosmos1a"), []byte("900uatom"))
	byName("bank").Set([]byte("balances/cosmos1c"), []byte("100uatom"))
	byName("bank").Set([]byte("supply/uatom"), []byte("1500")) // unchanged total
	byName("staking").Set([]byte("delegations/cosmos1a/val1"), []byte("100uatom"))
	commitAll()

	// ---- Block 3: governance proposal ----
	byName("gov").Set([]byte("proposals/1/status"), []byte("voting"))
	byName("gov").Set([]byte("proposals/1/title"), []byte("Upgrade to v2"))
	byName("gov").Set([]byte("votes/1/cosmos1a"), []byte("yes"))
	commitAll()

	haltHeight := int64(3)

	// ---- Migrate each substore ----
	migrated := make(map[string]*Store)
	migratedCounts := make(map[string]int)
	baseDir := t.TempDir()

	for _, s := range stores {
		dir := filepath.Join(baseDir, s.name)
		result, err := MigrateKVStore(s.iavl, dir, 0, nil)
		require.NoError(t, err, "migrate %s", s.name)

		result.Store.SetInitialVersion(haltHeight)
		migrated[s.name] = result.Store
		migratedCounts[s.name] = result.KeyCount
		t.Cleanup(func() { result.Store.Close() })
	}

	// ---- Verify key counts ----
	require.Equal(t, 4, migratedCounts["bank"])    // balances/a, /b, /c + supply
	require.Equal(t, 3, migratedCounts["staking"]) // val1, val2, delegation
	require.Equal(t, 5, migratedCounts["gov"])     // 2 params, proposal status+title, 1 vote

	// ---- Verify data integrity for each substore ----
	for _, s := range stores {
		dst := migrated[s.name]

		// Keys match
		srcKeys := collectKeys(s.iavl.Iterator(nil, nil))
		dstKeys := collectKeys(dst.Iterator(nil, nil))
		require.Equal(t, srcKeys, dstKeys, "store %s: key set mismatch", s.name)

		// Values match
		for _, k := range srcKeys {
			srcVal := s.iavl.Get([]byte(k))
			dstVal := dst.Get([]byte(k))
			require.Equal(t, srcVal, dstVal, "store %s key %s: value mismatch", s.name, k)
		}
	}

	// ---- Verify version tracking ----
	for name, dst := range migrated {
		require.Equal(t, haltHeight, dst.LastCommitID().Version, "store %s version", name)
	}

	// ---- Verify specific values survived migration ----
	require.Equal(t, []byte("900uatom"), migrated["bank"].Get([]byte("balances/cosmos1a")))
	require.Equal(t, []byte("500uatom"), migrated["bank"].Get([]byte("balances/cosmos1b")))
	require.Equal(t, []byte("100uatom"), migrated["bank"].Get([]byte("balances/cosmos1c")))
	require.Equal(t, []byte(`{"power":100,"moniker":"alice"}`), migrated["staking"].Get([]byte("validators/val1")))
	require.Equal(t, []byte("100uatom"), migrated["staking"].Get([]byte("delegations/cosmos1a/val1")))
	require.Equal(t, []byte("voting"), migrated["gov"].Get([]byte("proposals/1/status")))

	// ---- Verify QMDB can continue committing at correct height ----
	// Block 4: post-migration activity
	migrated["bank"].Set([]byte("balances/cosmos1d"), []byte("50uatom"))
	migrated["bank"].Set([]byte("balances/cosmos1a"), []byte("850uatom"))
	cid := migrated["bank"].Commit()
	require.Equal(t, haltHeight+1, cid.Version)

	// New data readable
	require.Equal(t, []byte("50uatom"), migrated["bank"].Get([]byte("balances/cosmos1d")))
	require.Equal(t, []byte("850uatom"), migrated["bank"].Get([]byte("balances/cosmos1a")))

	// Block 5: another commit
	migrated["staking"].Set([]byte("validators/val3"), []byte(`{"power":25,"moniker":"carol"}`))
	cid = migrated["staking"].Commit()
	require.Equal(t, haltHeight+1, cid.Version) // staking's first post-migration commit

	require.Equal(t, []byte(`{"power":25,"moniker":"carol"}`), migrated["staking"].Get([]byte("validators/val3")))
}

// TestMigrateMultipleBlocksPreservesLatestState verifies that migration
// captures the latest state, not intermediate versions. IAVL keeps history
// but migration should only copy the current working set.
func TestMigrateMultipleBlocksPreservesLatestState(t *testing.T) {
	src := newIAVLStore(t)

	// Block 1: create key
	src.Set([]byte("x"), []byte("v1"))
	src.Commit()

	// Block 2: update
	src.Set([]byte("x"), []byte("v2"))
	src.Commit()

	// Block 3: update again
	src.Set([]byte("x"), []byte("v3"))
	src.Set([]byte("y"), []byte("created-at-block-3"))
	src.Commit()

	dir := t.TempDir()
	result, err := MigrateKVStore(src, dir, 0, nil)
	require.NoError(t, err)
	defer result.Store.Close()

	// Only latest values should exist
	require.Equal(t, []byte("v3"), result.Store.Get([]byte("x")))
	require.Equal(t, []byte("created-at-block-3"), result.Store.Get([]byte("y")))
	require.Equal(t, 2, result.KeyCount)
}

// TestMigrateDeletedKeysNotCopied verifies that keys deleted in IAVL
// don't end up in QMDB. The iterator only yields live keys.
func TestMigrateDeletedKeysNotCopied(t *testing.T) {
	src := newIAVLStore(t)

	src.Set([]byte("keep"), []byte("yes"))
	src.Set([]byte("remove"), []byte("bye"))
	src.Commit()

	src.Delete([]byte("remove"))
	src.Commit()

	dir := t.TempDir()
	result, err := MigrateKVStore(src, dir, 0, nil)
	require.NoError(t, err)
	defer result.Store.Close()

	require.Equal(t, 1, result.KeyCount)
	require.Equal(t, []byte("yes"), result.Store.Get([]byte("keep")))
	require.Nil(t, result.Store.Get([]byte("remove")))
	require.False(t, result.Store.Has([]byte("remove")))
}

// TestMigrateScaleMultipleStores runs a larger migration across multiple
// stores to stress test the batch path and verify counts.
func TestMigrateScaleMultipleStores(t *testing.T) {
	type result struct {
		name  string
		count int
		store *Store
	}

	storeNames := []string{"auth", "bank", "staking", "slashing", "distribution", "gov", "mint"}
	keysPerStore := 500

	var results []result

	for _, name := range storeNames {
		src := newIAVLStore(t)
		for i := range keysPerStore {
			k := fmt.Sprintf("%s/key_%04d", name, i)
			v := fmt.Sprintf("%s/val_%04d", name, i)
			src.Set([]byte(k), []byte(v))
		}
		src.Commit()

		dir := filepath.Join(t.TempDir(), name)
		res, err := MigrateKVStore(src, dir, 200, nil)
		require.NoError(t, err)

		results = append(results, result{name: name, count: res.KeyCount, store: res.Store})
		t.Cleanup(func() { res.Store.Close() })
	}

	totalKeys := 0
	for _, r := range results {
		require.Equal(t, keysPerStore, r.count, "store %s", r.name)
		totalKeys += r.count

		// Spot check first and last key
		first := fmt.Sprintf("%s/key_0000", r.name)
		last := fmt.Sprintf("%s/key_0499", r.name)
		require.NotNil(t, r.store.Get([]byte(first)), "store %s missing first key", r.name)
		require.NotNil(t, r.store.Get([]byte(last)), "store %s missing last key", r.name)
	}

	require.Equal(t, len(storeNames)*keysPerStore, totalKeys)
}
