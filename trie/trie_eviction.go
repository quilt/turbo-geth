// Copyright 2019 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty off
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// Pruning of the Merkle Patricia trees

package trie

import (
	"sort"
)

type AccountEvicter interface {
	EvictLeaf([]byte)
}

type generations struct {
	generations     []*generation
	keyToGeneration map[string]*generation
	latestBlockNum  uint64
	totalSize       int64
}

func newGenerations() *generations {
	return &generations{
		make([]*generation, 0),
		make(map[string]*generation),
		0,
		0,
	}
}

func (gs *generations) add(blockNum uint64, key []byte, size uint) {
	if _, ok := gs.keyToGeneration[string(key)]; ok {
		gs.updateSize(blockNum, key, size)
		return
	}
	var generation *generation
	if blockNum > gs.latestBlockNum || len(gs.generations) == 0 {
		generation = newGeneration()
		gs.generations = append(gs.generations, generation)
		gs.latestBlockNum = blockNum
	} else {
		generation = gs.generations[len(gs.generations)-1]
	}
	generation.add(key, size)
	gs.keyToGeneration[string(key)] = generation
	gs.totalSize += int64(size)
}

func (gs *generations) touch(blockNum uint64, key []byte) {
	if len(gs.generations) == 0 {
		return
	}
	oldGeneration, ok := gs.keyToGeneration[string(key)]
	if !ok {
		return
	}
	var currentGeneration *generation
	if gs.latestBlockNum < blockNum {
		currentGeneration = newGeneration()
		gs.generations = append(gs.generations, currentGeneration)
		gs.latestBlockNum = blockNum
	} else {
		currentGeneration = gs.generations[len(gs.generations)-1]
	}
	currentGeneration.grabFrom(key, oldGeneration)
	gs.keyToGeneration[string(key)] = currentGeneration
}

func (gs *generations) remove(key []byte) {
	generation := gs.keyToGeneration[string(key)]
	if generation == nil {
		return
	}
	sizeDiff := generation.remove(key)
	gs.totalSize += sizeDiff
}

func (gs *generations) updateSize(blockNum uint64, key []byte, newSize uint) {
	generation := gs.keyToGeneration[string(key)]
	if generation == nil {
		gs.add(blockNum, key, newSize)
		return
	}
	sizeDiff := generation.updateAccountSize(key, newSize)
	gs.totalSize += sizeDiff
	gs.touch(blockNum, key)
}

// popKeysToEvict returns the keys to evict from the trie,
// also removing them from generations
func (gs *generations) popKeysToEvict(threshold uint64) []string {
	keys := make([]string, 0)
	for uint64(gs.totalSize) > threshold && len(gs.generations) > 0 {
		generation := gs.generations[0]
		gs.generations = gs.generations[1:]
		gs.totalSize -= generation.totalSize
		if gs.totalSize < 0 {
			gs.totalSize = 0
		}
		keysToEvict := generation.keys()
		keys = append(keys, keysToEvict...)
		for _, k := range keysToEvict {
			delete(gs.keyToGeneration, k)
		}
	}
	return keys
}

type generation struct {
	sizesByKey map[string]uint
	totalSize  int64
}

func newGeneration() *generation {
	return &generation{
		make(map[string]uint, 0),
		0,
	}
}

func (g *generation) empty() bool {
	return len(g.sizesByKey) == 0
}

func (g *generation) grabFrom(key []byte, other *generation) {
	if g == other {
		return
	}

	g.sizesByKey[string(key)] = other.sizesByKey[string(key)]
	g.totalSize += int64(g.sizesByKey[string(key)])
	other.totalSize -= int64(other.sizesByKey[string(key)])
	if other.totalSize < 0 {
		other.totalSize = 0
	}
	delete(other.sizesByKey, string(key))
}

func (g *generation) add(key []byte, size uint) {
	g.sizesByKey[string(key)] = size
	g.totalSize += int64(size)
}

func (g *generation) updateAccountSize(key []byte, size uint) int64 {
	oldSize := g.sizesByKey[string(key)]
	g.sizesByKey[string(key)] = size
	diff := int64(size) - int64(oldSize)
	g.totalSize += diff
	return diff
}

func (g *generation) remove(key []byte) int64 {
	oldSize := g.sizesByKey[string(key)]
	delete(g.sizesByKey, string(key))
	g.totalSize -= int64(oldSize)
	if g.totalSize < 0 {
		g.totalSize = 0
	}

	return -1 * int64(oldSize)
}

func (g *generation) keys() []string {
	keys := make([]string, len(g.sizesByKey))
	i := 0
	for k, _ := range g.sizesByKey {
		keys[i] = k
		i++
	}
	return keys
}

type TrieEviction struct {
	NoopTrieObserver // make sure that we don't need to implement unnecessary observer methods

	blockNumber uint64

	generations *generations
}

func NewTrieEviction() *TrieEviction {
	return &TrieEviction{
		generations: newGenerations(),
	}
}

func (tp *TrieEviction) SetBlockNumber(blockNumber uint64) {
	tp.blockNumber = blockNumber
}

func (tp *TrieEviction) BlockNumber() uint64 {
	return tp.blockNumber
}

func (tp *TrieEviction) AccountCreated(hex []byte, size uint) {
	tp.generations.add(tp.blockNumber, hex, size)
}

func (tp *TrieEviction) AccountDeleted(hex []byte) {
	tp.generations.remove(hex)
}

func (tp *TrieEviction) AccountTouched(hex []byte) {
	tp.generations.touch(tp.blockNumber, hex)
}

func (tp *TrieEviction) AccountSizeChanged(hex []byte, newSize uint) {
	tp.generations.updateSize(tp.blockNumber, hex, newSize)
}

func evictList(evicter AccountEvicter, hexes []string) bool {
	var empty = false
	sort.Strings(hexes)

	// from long to short -- a naive way to first clean up nodes and then accounts
	// FIXME: optimize to avoid the same paths
	for i := len(hexes) - 1; i >= 0; i-- {
		key := hexToKeybytes([]byte(hexes[i]))
		evicter.EvictLeaf(key)
	}
	return empty
}

// EvictToFitSize evicts mininum number of generations necessary so that the total
// size of accounts left is fits into the provided threshold
func (tp *TrieEviction) EvictToFitSize(
	evicter AccountEvicter,
	threshold uint64,
) bool {
	if uint64(tp.generations.totalSize) <= threshold {
		return false
	}

	keys := tp.generations.popKeysToEvict(threshold)
	return evictList(evicter, keys)
}

func (tp *TrieEviction) TotalSize() uint64 {
	return uint64(tp.generations.totalSize)
}

func (tp *TrieEviction) NumberOf() uint64 {
	total := uint64(0)
	for _, gen := range tp.generations.generations {
		total += uint64(len(gen.sizesByKey))
	}
	return total
}
