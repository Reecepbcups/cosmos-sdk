package qmdb

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/iavl"

	"cosmossdk.io/log"
	iavlstore "cosmossdk.io/store/iavl"
	"cosmossdk.io/store/wrapper"
)

// diskSize walks a directory and sums file sizes.
func diskSize(dir string) int64 {
	var total int64
	filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}

func BenchmarkStorageScale(b *testing.B) {
	sizes := []int{1_000, 10_000, 100_000, 500_000, 1_000_000}
	keySize := 20   // typical cosmos address-ish key
	valSize := 100  // typical balance/state blob

	for _, n := range sizes {
		rawDataBytes := int64(n) * int64(keySize+valSize)

		// --- IAVL on LevelDB (real disk) ---
		b.Run(fmt.Sprintf("IAVL_disk/%dk", n/1000), func(b *testing.B) {
			dir := b.TempDir()
			db, err := dbm.NewGoLevelDB("iavl", dir, nil)
			if err != nil {
				b.Fatal(err)
			}
			tree := iavl.NewMutableTree(wrapper.NewDBWrapper(db), 500, false, log.NewNopLogger())
			for i := range n {
				key := fmt.Appendf(nil, "k%019d", i) // 20 bytes
				val := make([]byte, valSize)
				val[0] = byte(i)
				_, err := tree.Set(key, val)
				if err != nil {
					b.Fatal(err)
				}
			}
			_, _, err = tree.SaveVersion()
			if err != nil {
				b.Fatal(err)
			}
			db.Close()

			sz := diskSize(dir)
			b.ReportMetric(float64(sz)/(1024*1024), "disk_MB")
			b.ReportMetric(float64(sz)/float64(n), "bytes/key")
			b.ReportMetric(float64(sz)/float64(rawDataBytes), "amplification")
		})

		// --- QMDB (real disk) ---
		b.Run(fmt.Sprintf("QMDB_disk/%dk", n/1000), func(b *testing.B) {
			dir := b.TempDir()
			store, err := NewStore(dir, true)
			if err != nil {
				b.Fatal(err)
			}
			batchSize := 10_000
			for i := range n {
				key := fmt.Appendf(nil, "k%019d", i)
				val := make([]byte, valSize)
				val[0] = byte(i)
				store.Set(key, val)
				if (i+1)%batchSize == 0 || i == n-1 {
					store.Commit()
				}
			}
			store.Close()

			sz := diskSize(dir)
			b.ReportMetric(float64(sz)/(1024*1024), "disk_MB")
			b.ReportMetric(float64(sz)/float64(n), "bytes/key")
			b.ReportMetric(float64(sz)/float64(rawDataBytes), "amplification")
		})

		// --- IAVL heap (MemDB, measures Go heap for tree nodes) ---
		b.Run(fmt.Sprintf("IAVL_heap/%dk", n/1000), func(b *testing.B) {
			runtime.GC()
			var m1 runtime.MemStats
			runtime.ReadMemStats(&m1)

			db := wrapper.NewDBWrapper(dbm.NewMemDB())
			tree := iavl.NewMutableTree(db, 500, false, log.NewNopLogger())
			for i := range n {
				key := fmt.Appendf(nil, "k%019d", i)
				val := make([]byte, valSize)
				val[0] = byte(i)
				tree.Set(key, val)
			}
			tree.SaveVersion()
			_ = iavlstore.UnsafeNewStore(tree)

			runtime.GC()
			var m2 runtime.MemStats
			runtime.ReadMemStats(&m2)

			heap := int64(m2.HeapAlloc - m1.HeapAlloc)
			b.ReportMetric(float64(heap)/(1024*1024), "heap_MB")
			b.ReportMetric(float64(heap)/float64(n), "bytes/key")
		})

		// --- QMDB heap (btree index only, Rust side not counted) ---
		b.Run(fmt.Sprintf("QMDB_heap/%dk", n/1000), func(b *testing.B) {
			runtime.GC()
			var m1 runtime.MemStats
			runtime.ReadMemStats(&m1)

			store := setupQMDBN(b, n)

			runtime.GC()
			var m2 runtime.MemStats
			runtime.ReadMemStats(&m2)

			heap := int64(m2.HeapAlloc - m1.HeapAlloc)
			b.ReportMetric(float64(heap)/(1024*1024), "heap_MB")
			b.ReportMetric(float64(heap)/float64(n), "bytes/key")
			store.Close()
		})
	}
}

// Measure how storage grows with multiple versions (commits).
// Cosmos chains accumulate versions over time.
func BenchmarkVersionGrowth(b *testing.B) {
	numKeys := 10_000
	versions := []int{1, 10, 50, 100}
	keySize := 20
	valSize := 100

	for _, v := range versions {
		b.Run(fmt.Sprintf("IAVL/%d_versions", v), func(b *testing.B) {
			dir := b.TempDir()
			db, err := dbm.NewGoLevelDB("iavl", dir, nil)
			if err != nil {
				b.Fatal(err)
			}
			tree := iavl.NewMutableTree(wrapper.NewDBWrapper(db), 500, false, log.NewNopLogger())

			// Initial population
			for i := range numKeys {
				key := fmt.Appendf(nil, "k%019d", i)
				val := make([]byte, valSize)
				tree.Set(key, val)
			}
			tree.SaveVersion()

			// Subsequent versions: update 10% of keys each
			updatePer := numKeys / 10
			for ver := 1; ver < v; ver++ {
				for i := range updatePer {
					idx := (ver*updatePer + i) % numKeys
					key := fmt.Appendf(nil, "k%019d", idx)
					val := make([]byte, valSize)
					val[0] = byte(ver)
					tree.Set(key, val)
				}
				tree.SaveVersion()
			}
			db.Close()

			sz := diskSize(dir)
			rawData := int64(numKeys) * int64(keySize+valSize)
			b.ReportMetric(float64(sz)/(1024*1024), "disk_MB")
			b.ReportMetric(float64(sz)/float64(rawData), "amplification")
		})

		b.Run(fmt.Sprintf("QMDB/%d_versions", v), func(b *testing.B) {
			dir := b.TempDir()
			store, err := NewStore(dir, true)
			if err != nil {
				b.Fatal(err)
			}

			// Initial population
			for i := range numKeys {
				key := fmt.Appendf(nil, "k%019d", i)
				val := make([]byte, valSize)
				store.Set(key, val)
			}
			store.Commit()

			// Subsequent versions
			updatePer := numKeys / 10
			for ver := 1; ver < v; ver++ {
				for i := range updatePer {
					idx := (ver*updatePer + i) % numKeys
					key := fmt.Appendf(nil, "k%019d", idx)
					val := make([]byte, valSize)
					val[0] = byte(ver)
					store.Set(key, val)
				}
				store.Commit()
			}
			store.Close()

			sz := diskSize(dir)
			rawData := int64(numKeys) * int64(keySize+valSize)
			b.ReportMetric(float64(sz)/(1024*1024), "disk_MB")
			b.ReportMetric(float64(sz)/float64(rawData), "amplification")
		})
	}
}
