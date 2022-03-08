// https://github.com/google/starlark-go/pull/403

package starext

import (
	"math"
	"sort"
	_ "unsafe" // for go:linkname hack

	"go.starlark.net/starlark"
)

const (
	bucketSize = 8
)

// hashString computes the hash of s.
func hashString(s string) uint32 {
	if len(s) >= 12 {
		// Call the Go runtime's optimized hash implementation,
		// which uses the AESENC instruction on amd64 machines.
		return uint32(goStringHash(s, 0))
	}
	return softHashString(s)
}

//go:linkname goStringHash runtime.stringHash
func goStringHash(s string, seed uintptr) uintptr

// softHashString computes the 32-bit FNV-1a hash of s in software.
func softHashString(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}

// An OrderedStringDict is a mutable mapping from names to values with
// support for fast indexing. Keys are stored in order of unique insertion.
// Keys are fast to add and access but slow to delete.
// It is not a true starlark.Value.
type OrderedStringDict struct {
	// Hash table that maps names to indicies within entries.
	table   []osdBucket        // len is zero or a power of two
	bucket0 [bucketSize]uint32 // inline allocation for small maps
	entries []osdEntry         // entries in order of insertion
}

// NewOrderedStringDict returns a dictionary with initial space for
// at least size insertions before rehashing.
func NewOrderedStringDict(size int) *OrderedStringDict {
	dict := new(OrderedStringDict)
	dict.init(size)
	return dict
}

type osdBucket []uint32 // index to entries

type osdEntry struct {
	key   string
	value starlark.Value
	hash  uint32
}

func (d *OrderedStringDict) init(size int) {
	if size < 0 {
		panic("size < 0")
	}
	nb := 1
	for overloaded(size, nb) {
		nb = nb << 1
	}
	if nb < 2 {
		d.table = []osdBucket{d.bucket0[:0]}
	} else {
		d.table = make([]osdBucket, nb)
		for i := range d.table {
			d.table[i] = make(osdBucket, 0, bucketSize)
		}
	}
	d.entries = make([]osdEntry, 0, size)
}

func (d *OrderedStringDict) rehash() {
	// TODO: shrink?
	for i, l := 0, len(d.table); i < l; i++ {
		d.table[i] = d.table[i][:0]
	}
	oldEntries := d.entries
	d.entries = d.entries[:0]
	for i, l := 0, len(oldEntries); i < l; i++ {
		e := oldEntries[i]
		d.insert(e.hash, e.key, e.value)
	}
}

func (d *OrderedStringDict) grow() {
	// Double the number of buckets and rehash.
	newTable := make([]osdBucket, len(d.table)<<1)
	for i, l := 0, len(d.table); i < l; i++ {
		// Reuse bucket if below bucketSize.
		p := d.table[i]
		if cap(p) <= bucketSize {
			newTable[i] = p[:0]
		} else {
			newTable[i] = make(osdBucket, 0, bucketSize)
		}
	}
	for i, l := len(d.table), len(newTable); i < l; i++ {
		newTable[i] = make(osdBucket, 0, bucketSize)
	}
	d.table = newTable
	oldEntries := d.entries
	d.entries = make([]osdEntry, 0, len(d.entries)<<1)
	for i, l := 0, len(oldEntries); i < l; i++ {
		e := oldEntries[i]
		d.insert(e.hash, e.key, e.value)
	}
}

func overloaded(elems, buckets int) bool {
	const loadFactor = 6.5 // just a guess
	return elems >= bucketSize && float64(elems) >= loadFactor*float64(buckets)
}

func (d *OrderedStringDict) insert(h uint32, k string, v starlark.Value) {
	if d.table == nil {
		d.init(1)
	}

	// Does the number of elements exceed the buckets' load factor?
	for overloaded(len(d.entries), len(d.table)) {
		d.grow()
	}

	n := h & (uint32(len(d.table) - 1))
	p := d.table[n]
	for i, l := 0, len(p); i < l; i++ {
		e := &d.entries[p[i]]
		if h == e.hash && k == e.key {
			e.value = v
			return
		}
	}

	// Append value to entries, linking the bucket to the entries list.
	d.entries = append(d.entries, osdEntry{
		hash:  h,
		key:   k,
		value: v,
	})
	i := len(d.entries) - 1
	if i > math.MaxUint32 {
		panic("max entries")
	}
	d.table[n] = append(p, uint32(i))
}

func (d *OrderedStringDict) Insert(k string, v starlark.Value) {
	h := hashString(k)
	d.insert(h, k, v)
}

func (d *OrderedStringDict) Get(k string) (v starlark.Value, found bool) {
	if d.table == nil {
		return starlark.None, false // empty
	}

	if l := len(d.entries); l <= bucketSize {
		for i := 0; i < l; i++ {
			e := &d.entries[i]
			if k == e.key {
				return e.value, true // found
			}
		}
		return starlark.None, false // not found
	}

	h := hashString(k)

	// Inspect each entry in the bucket slice.
	p := d.table[h&(uint32(len(d.table)-1))]
	for i, l := 0, len(p); i < l; i++ {
		e := &d.entries[p[i]]
		if h == e.hash && k == e.key {
			return e.value, true // found
		}
	}
	return starlark.None, false // not found
}

func (d *OrderedStringDict) Delete(k string) (v starlark.Value, found bool) {
	if d.table == nil {
		return starlark.None, false // empty
	}

	h := hashString(k)
	n := h & (uint32(len(d.table) - 1))
	p := d.table[n]
	for i, l := 0, len(p); i < l; i++ {
		j := p[i]
		e := &d.entries[j]
		if e.hash == h && k == e.key {
			v := e.value
			e.value = nil // remove pointers

			d.entries = append(d.entries[:j], d.entries[j+1:]...)
			d.rehash()

			return v, true // deleted
		}
	}
	return starlark.None, false // not found
}

func (d *OrderedStringDict) Clear() { d.init(1) }

func (d *OrderedStringDict) Keys() []string {
	keys := make([]string, 0, len(d.entries))
	for i, l := 0, len(d.entries); i < l; i++ {
		e := &d.entries[i]
		keys = append(keys, e.key)
	}
	return keys
}

func (d *OrderedStringDict) Index(i int) starlark.Value {
	return d.entries[i].value
}
func (d *OrderedStringDict) Len() int {
	return len(d.entries)
}
func (d *OrderedStringDict) KeyIndex(i int) (string, starlark.Value) {
	e := &d.entries[i]
	return e.key, e.value
}

type osdKeySorter []osdEntry

func (d osdKeySorter) Len() int           { return len(d) }
func (d osdKeySorter) Swap(i, j int)      { d[i], d[j] = d[j], d[i] }
func (d osdKeySorter) Less(i, j int) bool { return d[i].key < d[j].key }

// Sort the dict by name.
func (d *OrderedStringDict) Sort() {
	sort.Sort(osdKeySorter(d.entries))
	d.rehash()
}
