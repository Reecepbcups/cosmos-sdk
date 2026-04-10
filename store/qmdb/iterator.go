package qmdb

import (
	"bytes"
	"errors"

	"github.com/tidwall/btree"

	"cosmossdk.io/store/types"
)

var _ types.Iterator = (*qmdbIterator)(nil)

// qmdbIterator wraps a btree iterator for range scans.
type qmdbIterator struct {
	iter      btree.IterG[item]
	start     []byte
	end       []byte
	ascending bool
	valid     bool
}

func newQmdbIterator(tree *btree.BTreeG[item], start, end []byte, ascending bool) *qmdbIterator {
	iter := tree.Iter()
	var valid bool

	if ascending {
		if start != nil {
			valid = iter.Seek(item{key: start})
		} else {
			valid = iter.First()
		}
	} else {
		if end != nil {
			valid = iter.Seek(item{key: end})
			if !valid {
				valid = iter.Last()
			} else {
				// end is exclusive
				valid = iter.Prev()
			}
		} else {
			valid = iter.Last()
		}
	}

	mi := &qmdbIterator{
		iter:      iter,
		start:     start,
		end:       end,
		ascending: ascending,
		valid:     valid,
	}

	if mi.valid {
		mi.valid = mi.keyInRange(mi.Key())
	}

	return mi
}

func (mi *qmdbIterator) Domain() (start, end []byte) {
	return mi.start, mi.end
}

func (mi *qmdbIterator) Close() error {
	mi.iter.Release()
	return nil
}

func (mi *qmdbIterator) Error() error {
	if !mi.Valid() {
		return errors.New("invalid iterator")
	}
	return nil
}

func (mi *qmdbIterator) Valid() bool {
	return mi.valid
}

func (mi *qmdbIterator) Next() {
	if !mi.valid {
		panic("invalid iterator")
	}

	if mi.ascending {
		mi.valid = mi.iter.Next()
	} else {
		mi.valid = mi.iter.Prev()
	}

	if mi.valid {
		mi.valid = mi.keyInRange(mi.Key())
	}
}

func (mi *qmdbIterator) keyInRange(key []byte) bool {
	if mi.ascending && mi.end != nil && bytes.Compare(key, mi.end) >= 0 {
		return false
	}
	if !mi.ascending && mi.start != nil && bytes.Compare(key, mi.start) < 0 {
		return false
	}
	return true
}

func (mi *qmdbIterator) Key() []byte {
	return mi.iter.Item().key
}

func (mi *qmdbIterator) Value() []byte {
	return mi.iter.Item().value
}
