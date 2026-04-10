package qmdb

import (
	"github.com/klauspost/compress/zstd"
)

// Compression for values going to/from qmdb disk storage.
// The btree index holds uncompressed values for fast reads.
// Compression only happens at the Commit/FFI boundary.
//
// Format: compressed values are prefixed with a magic byte (0x01)
// so we can distinguish compressed from uncompressed on cold reads.
// Uncompressed values have no prefix when stored in qmdb.
//
// We use zstd level 1 (fastest) since this is on the commit hot path.

const (
	compressPrefix   byte = 0x01
	compressMinSize       = 64 // don't bother compressing tiny values
)

var (
	zstdEncoder *zstd.Encoder
	zstdDecoder *zstd.Decoder
)

func init() {
	var err error
	zstdEncoder, err = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedFastest))
	if err != nil {
		panic(err)
	}
	zstdDecoder, err = zstd.NewReader(nil)
	if err != nil {
		panic(err)
	}
}

// compressValue compresses a value for disk storage.
// Returns the original value if it's too small or compression doesn't help.
func compressValue(v []byte) []byte {
	if len(v) < compressMinSize {
		return v
	}
	compressed := zstdEncoder.EncodeAll(v, make([]byte, 0, len(v)))

	// Only use compressed form if it's actually smaller (with prefix overhead)
	if len(compressed)+1 >= len(v) {
		return v
	}

	out := make([]byte, 1+len(compressed))
	out[0] = compressPrefix
	copy(out[1:], compressed)
	return out
}

// decompressValue decompresses a value read from disk.
// Values without the compress prefix are returned as-is.
func decompressValue(v []byte) []byte {
	if len(v) == 0 || v[0] != compressPrefix {
		return v
	}
	decoded, err := zstdDecoder.DecodeAll(v[1:], nil)
	if err != nil {
		// If decompression fails, the value wasn't actually compressed
		// (it just happened to start with 0x01). Return as-is.
		return v
	}
	return decoded
}
