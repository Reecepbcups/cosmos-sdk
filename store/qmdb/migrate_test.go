package qmdb

import (
	"fmt"
	"testing"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log"
	iavlstore "cosmossdk.io/store/iavl"
	"cosmossdk.io/store/metrics"
	"cosmossdk.io/store/types"
)

func newIAVLStore(t *testing.T) types.CommitKVStore {
	t.Helper()
	db := dbm.NewMemDB()
	key := types.NewKVStoreKey("test")
	store, err := iavlstore.LoadStore(db, log.NewNopLogger(), key, types.CommitID{}, 128, false, metrics.NewNoOpMetrics())
	require.NoError(t, err)
	return store
}

func collectKeys(iter types.Iterator) []string {
	defer iter.Close()
	var keys []string
	for ; iter.Valid(); iter.Next() {
		keys = append(keys, string(iter.Key()))
	}
	return keys
}

func TestMigrateFromIAVL(t *testing.T) {
	src := newIAVLStore(t)

	data := map[string]string{
		"bank/balances/cosmos1abc": "1000uatom",
		"bank/balances/cosmos1def": "2000uatom",
		"staking/validators/val1": "power:100",
		"staking/validators/val2": "power:200",
		"gov/proposals/1":         "passed",
		"gov/proposals/2":         "voting",
	}
	for k, v := range data {
		src.Set([]byte(k), []byte(v))
	}
	src.Commit()

	dir := t.TempDir()
	result, err := MigrateKVStore(src, dir, 0, nil)
	require.NoError(t, err)
	defer result.Store.Close()

	require.Equal(t, len(data), result.KeyCount)
	require.Equal(t, 1, result.Batches)

	// All KV pairs present
	for k, v := range data {
		got := result.Store.Get([]byte(k))
		require.Equal(t, []byte(v), got, "key %s", k)
	}

	// Iteration order matches IAVL (both sorted)
	iavlKeys := collectKeys(src.Iterator(nil, nil))
	qmdbKeys := collectKeys(result.Store.Iterator(nil, nil))
	require.Equal(t, iavlKeys, qmdbKeys)
}

func TestMigrateEmpty(t *testing.T) {
	src := newIAVLStore(t)
	src.Commit()

	dir := t.TempDir()
	result, err := MigrateKVStore(src, dir, 0, nil)
	require.NoError(t, err)
	defer result.Store.Close()

	require.Equal(t, 0, result.KeyCount)
	require.Equal(t, 1, result.Batches)
}

func TestMigrateBatching(t *testing.T) {
	src := newIAVLStore(t)

	n := 250
	for i := 0; i < n; i++ {
		src.Set([]byte(fmt.Sprintf("key_%05d", i)), []byte(fmt.Sprintf("val_%05d", i)))
	}
	src.Commit()

	dir := t.TempDir()
	var progressCalls []int
	result, err := MigrateKVStore(src, dir, 100, func(migrated int) {
		progressCalls = append(progressCalls, migrated)
	})
	require.NoError(t, err)
	defer result.Store.Close()

	require.Equal(t, n, result.KeyCount)
	require.Equal(t, 3, result.Batches) // 100 + 100 + 50
	require.Equal(t, []int{100, 200, 250}, progressCalls)

	// Spot check data integrity
	for i := 0; i < n; i++ {
		k := fmt.Sprintf("key_%05d", i)
		v := fmt.Sprintf("val_%05d", i)
		require.Equal(t, []byte(v), result.Store.Get([]byte(k)))
	}
}

func TestMigrateSetInitialVersion(t *testing.T) {
	src := newIAVLStore(t)
	src.Set([]byte("a"), []byte("1"))
	src.Commit()

	dir := t.TempDir()
	result, err := MigrateKVStore(src, dir, 0, nil)
	require.NoError(t, err)
	defer result.Store.Close()

	result.Store.SetInitialVersion(5_000_000)
	require.Equal(t, int64(5_000_000), result.Store.LastCommitID().Version)
}

func TestMigrateLargeValues(t *testing.T) {
	src := newIAVLStore(t)

	// Mix of small and large values
	small := []byte("tiny")
	large := make([]byte, 8192)
	for i := range large {
		large[i] = byte(i % 256)
	}

	src.Set([]byte("small"), small)
	src.Set([]byte("large"), large)
	src.Commit()

	dir := t.TempDir()
	result, err := MigrateKVStore(src, dir, 0, nil)
	require.NoError(t, err)
	defer result.Store.Close()

	require.Equal(t, 2, result.KeyCount)
	require.Equal(t, small, result.Store.Get([]byte("small")))
	require.Equal(t, large, result.Store.Get([]byte("large")))
}
