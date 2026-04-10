package qmdb

import (
	"fmt"
	"runtime"
	"testing"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/iavl"

	"cosmossdk.io/log"
	iavlstore "cosmossdk.io/store/iavl"
	"cosmossdk.io/store/types"
	"cosmossdk.io/store/wrapper"
)

// Scale benchmarks: how Get/Has/Set perform as the dataset grows.
// IAVL uses MemDB (best case for IAVL). QMDB uses real disk.

func setupIAVLN(b *testing.B, n int) types.KVStore {
	b.Helper()
	db := wrapper.NewDBWrapper(dbm.NewMemDB())
	tree := iavl.NewMutableTree(db, 500, false, log.NewNopLogger())
	for i := range n {
		key := fmt.Appendf(nil, "key_%08d", i)
		_, err := tree.Set(key, randBytes(100))
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

func setupQMDBN(b *testing.B, n int) *Store {
	b.Helper()
	dir := b.TempDir()
	store, err := NewStore(dir, true)
	if err != nil {
		b.Fatal(err)
	}
	// Insert in batches of 10K to avoid huge single changeset
	batchSize := 10_000
	for i := range n {
		key := fmt.Appendf(nil, "key_%08d", i)
		store.Set(key, randBytes(100))
		if (i+1)%batchSize == 0 || i == n-1 {
			store.Commit()
		}
	}
	return store
}

// --- Get scaling ---

func BenchmarkGetScale(b *testing.B) {
	sizes := []int{1_000, 10_000, 100_000, 500_000, 1_000_000, 2_000_000}

	for _, n := range sizes {
		keys := make([][]byte, n)
		for i := range n {
			keys[i] = fmt.Appendf(nil, "key_%08d", i)
		}

		b.Run(fmt.Sprintf("IAVL/%dk", n/1000), func(b *testing.B) {
			store := setupIAVLN(b, n)
			runtime.GC()
			b.ResetTimer()
			b.ReportAllocs()
			for i := range b.N {
				store.Get(keys[i%n])
			}
		})

		b.Run(fmt.Sprintf("QMDB/%dk", n/1000), func(b *testing.B) {
			store := setupQMDBN(b, n)
			defer store.Close()
			runtime.GC()
			b.ResetTimer()
			b.ReportAllocs()
			for i := range b.N {
				store.Get(keys[i%n])
			}
		})
	}
}

// --- Has scaling ---

func BenchmarkHasScale(b *testing.B) {
	sizes := []int{1_000, 10_000, 100_000, 500_000}

	for _, n := range sizes {
		keys := make([][]byte, n)
		for i := range n {
			keys[i] = fmt.Appendf(nil, "key_%08d", i)
		}

		b.Run(fmt.Sprintf("IAVL/%dk", n/1000), func(b *testing.B) {
			store := setupIAVLN(b, n)
			runtime.GC()
			b.ResetTimer()
			b.ReportAllocs()
			for i := range b.N {
				store.Has(keys[i%n])
			}
		})

		b.Run(fmt.Sprintf("QMDB/%dk", n/1000), func(b *testing.B) {
			store := setupQMDBN(b, n)
			defer store.Close()
			runtime.GC()
			b.ResetTimer()
			b.ReportAllocs()
			for i := range b.N {
				store.Has(keys[i%n])
			}
		})
	}
}

// --- Set scaling (into populated store) ---

func BenchmarkSetScale(b *testing.B) {
	sizes := []int{1_000, 10_000, 100_000, 500_000}

	for _, n := range sizes {
		b.Run(fmt.Sprintf("IAVL/%dk", n/1000), func(b *testing.B) {
			store := setupIAVLN(b, n)
			keys := make([][]byte, b.N)
			vals := make([][]byte, b.N)
			for i := range b.N {
				keys[i] = fmt.Appendf(nil, "new_%08d", i)
				vals[i] = randBytes(100)
			}
			runtime.GC()
			b.ResetTimer()
			b.ReportAllocs()
			for i := range b.N {
				store.Set(keys[i], vals[i])
			}
		})

		b.Run(fmt.Sprintf("QMDB/%dk", n/1000), func(b *testing.B) {
			store := setupQMDBN(b, n)
			defer store.Close()
			keys := make([][]byte, b.N)
			vals := make([][]byte, b.N)
			for i := range b.N {
				keys[i] = fmt.Appendf(nil, "new_%08d", i)
				vals[i] = randBytes(100)
			}
			runtime.GC()
			b.ResetTimer()
			b.ReportAllocs()
			for i := range b.N {
				store.Set(keys[i], vals[i])
			}
		})
	}
}

// --- Miss rate: Get keys that don't exist ---

func BenchmarkGetMissScale(b *testing.B) {
	sizes := []int{1_000, 100_000, 500_000}

	for _, n := range sizes {
		missKeys := make([][]byte, 10_000)
		for i := range missKeys {
			missKeys[i] = fmt.Appendf(nil, "miss_%08d", i)
		}

		b.Run(fmt.Sprintf("IAVL/%dk", n/1000), func(b *testing.B) {
			store := setupIAVLN(b, n)
			runtime.GC()
			b.ResetTimer()
			b.ReportAllocs()
			for i := range b.N {
				store.Get(missKeys[i%len(missKeys)])
			}
		})

		b.Run(fmt.Sprintf("QMDB/%dk", n/1000), func(b *testing.B) {
			store := setupQMDBN(b, n)
			defer store.Close()
			runtime.GC()
			b.ResetTimer()
			b.ReportAllocs()
			for i := range b.N {
				store.Get(missKeys[i%len(missKeys)])
			}
		})
	}
}

// --- Memory usage report ---

func BenchmarkMemoryUsage(b *testing.B) {
	sizes := []int{10_000, 100_000, 500_000}

	for _, n := range sizes {
		b.Run(fmt.Sprintf("IAVL/%dk", n/1000), func(b *testing.B) {
			var mBefore, mAfter runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&mBefore)
			_ = setupIAVLN(b, n)
			runtime.GC()
			runtime.ReadMemStats(&mAfter)
			heapMB := float64(mAfter.HeapInuse-mBefore.HeapInuse) / (1024 * 1024)
			b.ReportMetric(heapMB, "heap_MB")
			b.ReportMetric(float64(mAfter.HeapInuse-mBefore.HeapInuse)/float64(n), "bytes/key")
		})

		b.Run(fmt.Sprintf("QMDB/%dk", n/1000), func(b *testing.B) {
			var mBefore, mAfter runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&mBefore)
			s := setupQMDBN(b, n)
			defer s.Close()
			runtime.GC()
			runtime.ReadMemStats(&mAfter)
			heapMB := float64(mAfter.HeapInuse-mBefore.HeapInuse) / (1024 * 1024)
			b.ReportMetric(heapMB, "heap_MB")
			b.ReportMetric(float64(mAfter.HeapInuse-mBefore.HeapInuse)/float64(n), "bytes/key")
		})
	}
}
