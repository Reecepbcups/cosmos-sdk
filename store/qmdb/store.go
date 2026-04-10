package qmdb

/*
#cgo LDFLAGS: -L${SRCDIR}/qmdb-rs/target/release -lqmdb_ffi -lm -ldl -lpthread
#cgo CFLAGS: -I${SRCDIR}/ffi
#include "qmdb_ffi.h"
#include <stdlib.h>
*/
import "C"

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"sync"
	"unsafe"

	"github.com/tidwall/btree"

	"cosmossdk.io/store/cachekv"
	pruningtypes "cosmossdk.io/store/pruning/types"
	"cosmossdk.io/store/tracekv"
	"cosmossdk.io/store/types"
)

var (
	_ types.KVStore                 = (*Store)(nil)
	_ types.CommitStore             = (*Store)(nil)
	_ types.Committer               = (*Store)(nil)
	_ types.StoreWithInitialVersion = (*Store)(nil)
)

// Store implements types.CommitKVStore backed by qmdb (Rust).
// It maintains a btree index in Go for iteration support since
// qmdb is hash-indexed and doesn't support range scans.
type Store struct {
	handle *C.QmdbHandle

	// pending writes buffered until Commit
	mu      sync.RWMutex
	pending map[string][]byte // nil value = delete
	version int64
	hash    []byte

	// btree index for iteration (keys -> values)
	index *btree.BTreeG[item]
}

type item struct {
	key   []byte
	value []byte
}

func byKeys(a, b item) bool {
	return bytes.Compare(a.key, b.key) < 0
}

// NewStore creates a new qmdb-backed store at the given directory path.
// If init is true, initializes a fresh database (destroys existing data).
func NewStore(dir string, init bool) (*Store, error) {
	cDir := C.CString(dir)
	defer C.free(unsafe.Pointer(cDir))

	if init {
		rc := C.qmdb_init(cDir)
		if rc != 0 {
			return nil, fmt.Errorf("qmdb_init failed: %d", rc)
		}
	}

	handle := C.qmdb_open(cDir)
	if handle == nil {
		return nil, errors.New("qmdb_open returned null")
	}

	height := int64(C.qmdb_height(handle))

	return &Store{
		handle:  handle,
		pending: make(map[string][]byte),
		version: height,
		index: btree.NewBTreeGOptions(byKeys, btree.Options{
			Degree:  32,
			NoLocks: false,
		}),
	}, nil
}

// Close releases the underlying qmdb handle.
func (s *Store) Close() {
	if s.handle != nil {
		C.qmdb_close(s.handle)
		s.handle = nil
	}
}

// SetInitialVersion implements types.StoreWithInitialVersion.
// Sets the reported height so LastCommitID returns the chain's halt height
// after migration. Also advances the Rust-side height via FFI so the next
// Commit() produces version+1.
//
// Only the in-memory height is updated. The metadb persists on the next
// commit_block, so don't crash between SetInitialVersion and the first Commit.
func (s *Store) SetInitialVersion(version int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rc := C.qmdb_set_height(s.handle, C.int64_t(version))
	if rc != 0 {
		panic(fmt.Sprintf("qmdb_set_height failed: %d", rc))
	}
	s.version = version
}

func (s *Store) GetStoreType() types.StoreType {
	return types.StoreTypePersistent
}

func (s *Store) CacheWrap() types.CacheWrap {
	return cachekv.NewStore(s)
}

func (s *Store) CacheWrapWithTrace(w io.Writer, tc types.TraceContext) types.CacheWrap {
	return cachekv.NewStore(tracekv.NewStore(s, w, tc))
}

// Get retrieves a value by key. Checks: pending -> btree index -> qmdb FFI.
func (s *Store) Get(key []byte) []byte {
	s.mu.RLock()
	if val, ok := s.pending[string(key)]; ok {
		s.mu.RUnlock()
		return val // nil means deleted
	}
	s.mu.RUnlock()

	// Check btree index (all committed keys are here)
	it, found := s.index.Get(item{key: key})
	if found {
		return it.value
	}

	// Cold read fallback (e.g. after restart before index is populated)
	return s.getFromQmdb(key)
}

func (s *Store) getFromQmdb(key []byte) []byte {
	if s.handle == nil || len(key) == 0 {
		return nil
	}

	valBuf := make([]byte, 4096)
	result := C.qmdb_get(
		s.handle,
		(*C.uint8_t)(unsafe.Pointer(&key[0])),
		C.uint32_t(len(key)),
		(*C.uint8_t)(unsafe.Pointer(&valBuf[0])),
		C.uint32_t(len(valBuf)),
	)

	if result.found == 0 {
		return nil
	}

	size := int(result.size)
	if size > len(valBuf) {
		// Retry with bigger buffer
		valBuf = make([]byte, size)
		result = C.qmdb_get(
			s.handle,
			(*C.uint8_t)(unsafe.Pointer(&key[0])),
			C.uint32_t(len(key)),
			(*C.uint8_t)(unsafe.Pointer(&valBuf[0])),
			C.uint32_t(len(valBuf)),
		)
		if result.found == 0 {
			return nil
		}
		size = int(result.size)
	}

	out := make([]byte, size)
	copy(out, valBuf[:size])
	return decompressValue(out)
}

func (s *Store) Has(key []byte) bool {
	s.mu.RLock()
	if val, ok := s.pending[string(key)]; ok {
		s.mu.RUnlock()
		return val != nil
	}
	s.mu.RUnlock()

	_, found := s.index.Get(item{key: key})
	if found {
		return true
	}

	return s.getFromQmdb(key) != nil
}

// Set buffers a write. The actual write happens on Commit.
func (s *Store) Set(key, value []byte) {
	types.AssertValidKey(key)
	types.AssertValidValue(value)
	s.mu.Lock()
	s.pending[string(key)] = value
	s.mu.Unlock()
}

// Delete buffers a deletion. The actual delete happens on Commit.
func (s *Store) Delete(key []byte) {
	s.mu.Lock()
	s.pending[string(key)] = nil
	s.mu.Unlock()
}

// Commit flushes all pending writes to qmdb as a new block.
func (s *Store) Commit() types.CommitID {
	s.mu.Lock()
	defer s.mu.Unlock()

	cs := C.qmdb_changeset_new()

	for k, v := range s.pending {
		key := []byte(k)
		keyPtr := (*C.uint8_t)(unsafe.Pointer(&key[0]))
		keyLen := C.uint32_t(len(key))

		if v == nil {
			// Delete
			C.qmdb_changeset_delete(cs, keyPtr, keyLen)
			s.index.Delete(item{key: key})
		} else {
			// Compress value for disk storage
			diskVal := compressValue(v)

			_, exists := s.index.Get(item{key: key})
			if exists {
				C.qmdb_changeset_write(cs,
					keyPtr, keyLen,
					(*C.uint8_t)(unsafe.Pointer(&diskVal[0])), C.uint32_t(len(diskVal)),
				)
			} else {
				C.qmdb_changeset_create(cs,
					keyPtr, keyLen,
					(*C.uint8_t)(unsafe.Pointer(&diskVal[0])), C.uint32_t(len(diskVal)),
				)
			}

			// Btree index stores UNCOMPRESSED values for fast reads
			keyCopy := make([]byte, len(key))
			copy(keyCopy, key)
			valCopy := make([]byte, len(v))
			copy(valCopy, v)
			s.index.Set(item{key: keyCopy, value: valCopy})
		}
	}

	newHeight := int64(C.qmdb_commit(s.handle, cs))
	if newHeight < 0 {
		panic("qmdb_commit failed")
	}
	s.version = newHeight
	s.pending = make(map[string][]byte)

	// Get root hash
	var hashBuf [32]byte
	C.qmdb_root_hash(s.handle, C.int64_t(newHeight), (*C.uint8_t)(unsafe.Pointer(&hashBuf[0])))
	s.hash = hashBuf[:]

	return types.CommitID{
		Version: s.version,
		Hash:    s.hash,
	}
}

func (s *Store) LastCommitID() types.CommitID {
	return types.CommitID{
		Version: s.version,
		Hash:    s.hash,
	}
}

func (s *Store) WorkingHash() []byte {
	// Compute hash of pending + committed state
	// For now return last committed hash
	return s.hash
}

func (s *Store) SetPruning(_ pruningtypes.PruningOptions) {}

func (s *Store) GetPruning() pruningtypes.PruningOptions {
	return pruningtypes.NewPruningOptions(pruningtypes.PruningNothing)
}

// Iterator returns a forward iterator over [start, end).
// Uses the btree index.
func (s *Store) Iterator(start, end []byte) types.Iterator {
	return newQmdbIterator(s.index, start, end, true)
}

// ReverseIterator returns a reverse iterator over [start, end).
func (s *Store) ReverseIterator(start, end []byte) types.Iterator {
	return newQmdbIterator(s.index, start, end, false)
}

// KeyHash returns the SHA256 hash of a key, matching qmdb's internal hashing.
func KeyHash(key []byte) [32]byte {
	return sha256.Sum256(key)
}
