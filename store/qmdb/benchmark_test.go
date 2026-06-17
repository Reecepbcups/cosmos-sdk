package qmdb

import (
	"crypto/rand"
	"fmt"
	"testing"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/iavl"

	"cosmossdk.io/log"
	iavlstore "cosmossdk.io/store/iavl"
	"cosmossdk.io/store/metrics"
	"cosmossdk.io/store/types"
	"cosmossdk.io/store/wrapper"
)

func randBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

// setupIAVL creates a fresh IAVL store with n pre-populated keys.
func setupIAVL(b *testing.B, n int) types.CommitKVStore {
	b.Helper()
	db := wrapper.NewDBWrapper(dbm.NewMemDB())
	tree := iavl.NewMutableTree(db, 500, false, log.NewNopLogger())

	for i := 0; i < n; i++ {
		key := []byte(fmt.Sprintf("key_%08d", i))
		val := randBytes(100)
		_, err := tree.Set(key, val)
		if err != nil {
			b.Fatal(err)
		}
	}
	_, _, err := tree.SaveVersion()
	if err != nil {
		b.Fatal(err)
	}
	return iavlstore.UnsafeNewStore(tree)
}

// setupQMDB creates a fresh qmdb store with n pre-populated keys.
func setupQMDB(b *testing.B, n int) *Store {
	b.Helper()
	dir := b.TempDir()
	store, err := NewStore(dir, true)
	if err != nil {
		b.Fatal(err)
	}

	for i := 0; i < n; i++ {
		key := []byte(fmt.Sprintf("key_%08d", i))
		val := randBytes(100)
		store.Set(key, val)
	}
	store.Commit()
	return store
}

// --- Set benchmarks ---

func BenchmarkIAVLSet(b *testing.B) {
	store := setupIAVL(b, 0)
	keys := make([][]byte, b.N)
	vals := make([][]byte, b.N)
	for i := range keys {
		keys[i] = []byte(fmt.Sprintf("bench_%08d", i))
		vals[i] = randBytes(100)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		store.Set(keys[i], vals[i])
	}
}

func BenchmarkQMDBSet(b *testing.B) {
	dir := b.TempDir()
	store, err := NewStore(dir, true)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	keys := make([][]byte, b.N)
	vals := make([][]byte, b.N)
	for i := range keys {
		keys[i] = []byte(fmt.Sprintf("bench_%08d", i))
		vals[i] = randBytes(100)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		store.Set(keys[i], vals[i])
	}
}

// --- Get benchmarks (from populated store) ---

func BenchmarkIAVLGet1K(b *testing.B) {
	benchmarkGet(b, "iavl", 1000)
}

func BenchmarkQMDBGet1K(b *testing.B) {
	benchmarkGet(b, "qmdb", 1000)
}

func BenchmarkIAVLGet10K(b *testing.B) {
	benchmarkGet(b, "iavl", 10_000)
}

func BenchmarkQMDBGet10K(b *testing.B) {
	benchmarkGet(b, "qmdb", 10_000)
}

func benchmarkGet(b *testing.B, backend string, n int) {
	b.Helper()
	keys := make([][]byte, n)
	for i := range keys {
		keys[i] = []byte(fmt.Sprintf("key_%08d", i))
	}

	var store types.KVStore
	switch backend {
	case "iavl":
		store = setupIAVL(b, n)
	case "qmdb":
		s := setupQMDB(b, n)
		defer s.Close()
		store = s
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		store.Get(keys[i%n])
	}
}

// --- Commit benchmarks (batch write + commit) ---

func BenchmarkIAVLCommit100(b *testing.B) {
	benchmarkCommit(b, "iavl", 100)
}

func BenchmarkQMDBCommit100(b *testing.B) {
	benchmarkCommit(b, "qmdb", 100)
}

func BenchmarkIAVLCommit1K(b *testing.B) {
	benchmarkCommit(b, "iavl", 1000)
}

func BenchmarkQMDBCommit1K(b *testing.B) {
	benchmarkCommit(b, "qmdb", 1000)
}

func benchmarkCommit(b *testing.B, backend string, batchSize int) {
	b.Helper()

	// Pre-generate all keys and values
	totalOps := b.N * batchSize
	keys := make([][]byte, totalOps)
	vals := make([][]byte, totalOps)
	for i := range keys {
		keys[i] = []byte(fmt.Sprintf("cmt_%08d", i))
		vals[i] = randBytes(100)
	}

	switch backend {
	case "iavl":
		store := setupIAVL(b, 0)
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			base := i * batchSize
			for j := 0; j < batchSize; j++ {
				store.Set(keys[base+j], vals[base+j])
			}
			store.Commit()
		}
	case "qmdb":
		dir := b.TempDir()
		store, err := NewStore(dir, true)
		if err != nil {
			b.Fatal(err)
		}
		defer store.Close()
		b.ResetTimer()
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			base := i * batchSize
			for j := 0; j < batchSize; j++ {
				store.Set(keys[base+j], vals[base+j])
			}
			store.Commit()
		}
	}
}

// --- Iterator benchmarks ---

func BenchmarkIAVLIterator1K(b *testing.B) {
	store := setupIAVL(b, 1000)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		iter := store.Iterator(nil, nil)
		for iter.Valid() {
			_ = iter.Key()
			_ = iter.Value()
			iter.Next()
		}
		iter.Close()
	}
}

func BenchmarkQMDBIterator1K(b *testing.B) {
	store := setupQMDB(b, 1000)
	defer store.Close()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		iter := store.Iterator(nil, nil)
		for iter.Valid() {
			_ = iter.Key()
			_ = iter.Value()
			iter.Next()
		}
		iter.Close()
	}
}

// --- Has benchmarks ---

func BenchmarkIAVLHas1K(b *testing.B) {
	store := setupIAVL(b, 1000)
	keys := make([][]byte, 1000)
	for i := range keys {
		keys[i] = []byte(fmt.Sprintf("key_%08d", i))
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		store.Has(keys[i%1000])
	}
}

func BenchmarkQMDBHas1K(b *testing.B) {
	store := setupQMDB(b, 1000)
	defer store.Close()
	keys := make([][]byte, 1000)
	for i := range keys {
		keys[i] = []byte(fmt.Sprintf("key_%08d", i))
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		store.Has(keys[i%1000])
	}
}

// --- Mixed workload: Set + Get + Commit ---

func BenchmarkIAVLMixed(b *testing.B) {
	benchmarkMixed(b, "iavl")
}

func BenchmarkQMDBMixed(b *testing.B) {
	benchmarkMixed(b, "qmdb")
}

func benchmarkMixed(b *testing.B, backend string) {
	b.Helper()

	var store interface {
		types.KVStore
		Commit() types.CommitID
	}

	switch backend {
	case "iavl":
		s := setupIAVL(b, 0)
		store = s.(interface {
			types.KVStore
			Commit() types.CommitID
		})
	case "qmdb":
		dir := b.TempDir()
		s, err := NewStore(dir, true)
		if err != nil {
			b.Fatal(err)
		}
		defer s.Close()
		store = s
	}

	_ = metrics.NewNoOpMetrics()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		key := []byte(fmt.Sprintf("mix_%08d", i))
		val := randBytes(100)
		store.Set(key, val)

		// Read back every 10th
		if i%10 == 0 {
			store.Get(key)
		}

		// Commit every 100
		if i%100 == 99 {
			store.Commit()
		}
	}
}
