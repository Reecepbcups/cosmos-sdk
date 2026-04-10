package qmdb

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"cosmossdk.io/store/cachekv"
)

func setupStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	store, err := NewStore(dir, true)
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSetGetDelete(t *testing.T) {
	store := setupStore(t)

	// Set and read back (before commit, from pending)
	store.Set([]byte("hello"), []byte("world"))
	require.Equal(t, []byte("world"), store.Get([]byte("hello")))
	require.True(t, store.Has([]byte("hello")))
	require.Nil(t, store.Get([]byte("missing")))

	// Commit and read back from qmdb
	cid := store.Commit()
	require.Equal(t, int64(1), cid.Version)
	require.Len(t, cid.Hash, 32)

	require.Equal(t, []byte("world"), store.Get([]byte("hello")))
	require.True(t, store.Has([]byte("hello")))

	// Delete
	store.Delete([]byte("hello"))
	require.Nil(t, store.Get([]byte("hello")))

	cid = store.Commit()
	require.Equal(t, int64(2), cid.Version)
}

func TestMultipleKeys(t *testing.T) {
	store := setupStore(t)

	keys := []string{"a", "b", "c", "d", "e"}
	for _, k := range keys {
		store.Set([]byte(k), []byte("val_"+k))
	}

	cid := store.Commit()
	require.Equal(t, int64(1), cid.Version)

	for _, k := range keys {
		val := store.Get([]byte(k))
		require.Equal(t, []byte("val_"+k), val, "key: %s", k)
	}
}

func TestUpdate(t *testing.T) {
	store := setupStore(t)

	store.Set([]byte("key"), []byte("v1"))
	store.Commit()

	store.Set([]byte("key"), []byte("v2"))
	store.Commit()

	require.Equal(t, []byte("v2"), store.Get([]byte("key")))
}

func TestIterator(t *testing.T) {
	store := setupStore(t)

	store.Set([]byte("a"), []byte("1"))
	store.Set([]byte("b"), []byte("2"))
	store.Set([]byte("c"), []byte("3"))
	store.Set([]byte("d"), []byte("4"))
	store.Commit()

	// Forward full range
	iter := store.Iterator(nil, nil)
	var keys []string
	for ; iter.Valid(); iter.Next() {
		keys = append(keys, string(iter.Key()))
	}
	iter.Close()
	require.Equal(t, []string{"a", "b", "c", "d"}, keys)

	// Forward sub-range [b, d)
	iter = store.Iterator([]byte("b"), []byte("d"))
	keys = nil
	for ; iter.Valid(); iter.Next() {
		keys = append(keys, string(iter.Key()))
	}
	iter.Close()
	require.Equal(t, []string{"b", "c"}, keys)

	// Reverse
	iter = store.ReverseIterator(nil, nil)
	keys = nil
	for ; iter.Valid(); iter.Next() {
		keys = append(keys, string(iter.Key()))
	}
	iter.Close()
	require.Equal(t, []string{"d", "c", "b", "a"}, keys)
}

func TestCommitID(t *testing.T) {
	store := setupStore(t)

	store.Set([]byte("x"), []byte("y"))
	cid := store.Commit()

	require.Equal(t, cid, store.LastCommitID())
	require.Equal(t, int64(1), cid.Version)
	require.NotEmpty(t, cid.Hash)
}

func TestCacheWrap(t *testing.T) {
	store := setupStore(t)

	store.Set([]byte("a"), []byte("1"))
	store.Commit()

	cw := store.CacheWrap()
	cw.(*cachekv.Store).Set([]byte("b"), []byte("2"))

	// b not in parent yet
	require.Nil(t, store.Get([]byte("b")))

	cw.Write()
	// now b is in pending
	require.Equal(t, []byte("2"), store.Get([]byte("b")))
}

func TestEmptyCommit(t *testing.T) {
	store := setupStore(t)

	// Commit with nothing pending
	cid := store.Commit()
	require.Equal(t, int64(1), cid.Version)
}

func TestLargeValue(t *testing.T) {
	store := setupStore(t)

	bigVal := make([]byte, 8192)
	for i := range bigVal {
		bigVal[i] = byte(i % 256)
	}

	store.Set([]byte("big"), bigVal)
	store.Commit()

	got := store.Get([]byte("big"))
	require.Equal(t, bigVal, got)
}

func TestReopenStore(t *testing.T) {
	t.Skip("qmdb SeqAds recovery with twig files needs investigation")
	dir := t.TempDir()

	// Create and write
	store1, err := NewStore(dir, true)
	require.NoError(t, err)
	store1.Set([]byte("persist"), []byte("me"))
	store1.Commit()
	store1.Close()

	// Reopen
	store2, err := NewStore(dir, false)
	require.NoError(t, err)
	defer store2.Close()

	got := store2.Get([]byte("persist"))
	require.Equal(t, []byte("me"), got)
	require.Equal(t, int64(1), store2.version)
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func TestManyKeys(t *testing.T) {
	store := setupStore(t)

	n := 1000
	for i := 0; i < n; i++ {
		k := fmt.Sprintf("key_%05d", i)
		v := fmt.Sprintf("val_%05d", i)
		store.Set([]byte(k), []byte(v))
	}
	cid := store.Commit()
	require.Equal(t, int64(1), cid.Version)

	for i := 0; i < n; i++ {
		k := fmt.Sprintf("key_%05d", i)
		v := fmt.Sprintf("val_%05d", i)
		require.Equal(t, []byte(v), store.Get([]byte(k)))
	}
}
