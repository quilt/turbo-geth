package trie

import (
	"github.com/ledgerwatch/turbo-geth/common"
)

// unload traveses the trie, finds the necessary account,
// replaces it with the hashNode
// if possible, it then goes to see it's parents and
// unload those if possible
// (shortNodes, duoNodes, fullNodes)
// non thread-safe
func (t *Trie) EvictLeaf(hex []byte) {
	hex = keybytesToHex(hex)

	if isHashOrEmpty(t.root) {
		return
	}

	switch t.root.(type) {
	case hashNode:
		return
	}

	path := make([]node, len(hex))
	hexes := make([][]byte, len(hex))
	depth := 0
	pos := 0

	currentNode := t.root

	leafFound := false

	// building the path
	for depth < len(hex) && !leafFound {
		switch n := currentNode.(type) {
		case nil:
			// no node found at this path
			return
		case *shortNode:
			matchLen := prefixLen(hex[pos:], n.Key)
			if matchLen == len(n.Key) || n.Key[matchLen] == 16 {
				path[depth] = n
				currentNode = n.Val
				pos += matchLen
				hexes[depth] = hex[:pos]
				depth++
			} else {
				// no path found
				return
			}
		case *duoNode:
			i1, i2 := n.childrenIdx()
			switch hex[pos] {
			case i1:
				path[depth] = n
				currentNode = n.child1
				pos++
				hexes[depth] = hex[:pos]
				depth++
			case i2:
				path[depth] = n
				currentNode = n.child2
				pos++
				hexes[depth] = hex[:pos]
				depth++
			default:
				// no path found
				return
			}
		case *fullNode:
			child := n.Children[hex[pos]]
			if child == nil {
				// no path found
				return
			}
			path[depth] = n
			currentNode = child
			pos++
			hexes[depth] = hex[:pos]
			depth++
		case *accountNode:
			path[depth] = n
			hexes[depth] = hex[:pos]
			if pos == len(hex)-1 {
				leafFound = true
			}
		case valueNode:
			path[depth] = n
			hexes[depth] = hex[:pos]
			leafFound = true
		case hashNode:
			// nothing to unload
			return
		}
	}

	switch path[depth].(type) {
	case *accountNode:
		break
	default:
		// we ended up not on an account
		return
	}

	// now let's try to unload the node subtrie
	for i := depth; i >= 0; i-- {
		nd := path[i]
		canEvict := false
		isStructNode := false
		switch n := nd.(type) {
		case *accountNode:
			canEvict = true
			notifyAccountStorageAsEvicted(n.storage, hexes[i], t.observers)
		case *shortNode:
			canEvict = true
		case *duoNode:
			canEvict = isHashOrEmpty(n.child1) && isHashOrEmpty(n.child2)
			isStructNode = true
		case *fullNode:
			childrenEmpty := true
			for _, child := range n.Children {
				if !isHashOrEmpty(child) {
					childrenEmpty = false
					break
				}
			}
			canEvict = childrenEmpty
			isStructNode = true
		}

		if !canEvict {
			return
		}

		t.evictSubtreeFromHashMap(nd)

		var hn common.Hash

		// nd.reference() will return empty hash for the account node.
		// currently, each account node is always wrapped in to a shortNode,
		// so we don't really care
		copy(hn[:], nd.reference())
		hnode := hashNode(hn[:])

		if isStructNode {
			t.observers.WillUnloadStructNode(hexes[i][:len(hexes[i])-1], hn[:])
		}

		if i == 0 {
			t.root = hnode
			return
		}

		parent := path[i-1]

		switch p := parent.(type) {
		case nil:
			return
		case *shortNode:
			p.Val = hnode
		case *duoNode:
			i1, i2 := p.childrenIdx()
			idx := idxFromHex(hexes[i-1])
			switch idx {
			case i1:
				p.child1 = hnode
			case i2:
				p.child2 = hnode
			}
		case *fullNode:
			idx := idxFromHex(hexes[i-1])
			p.Children[idx] = hnode
		}
	}
}

func notifyAccountStorageAsEvicted(node node, hex []byte, observer TrieObserver) {
	if node == nil {
		return
	}
	switch n := node.(type) {
	case *duoNode:
		//notify
		observer.WillUnloadStructNode(hex, n.reference())
		idx1, idx2 := n.childrenIdx()
		notifyAccountStorageAsEvicted(n.child1, append(hex, idx1), observer)
		notifyAccountStorageAsEvicted(n.child2, append(hex, idx2), observer)
	case *fullNode:
		observer.WillUnloadStructNode(hex, n.reference())
		for idx, child := range n.Children {
			notifyAccountStorageAsEvicted(child, append(hex, byte(idx)), observer)
		}
	case *shortNode:
		key := n.Key
		if key[len(key)-1] == 16 {
			key = key[:len(key)-1]
		}
		notifyAccountStorageAsEvicted(n.Val, append(hex, key...), observer)
	}
}

func idxFromHex(hex []byte) byte {
	return hex[len(hex)-1]
}

func isHashOrEmpty(node node) bool {
	switch node.(type) {
	case nil:
		return true
	case hashNode:
		return true
	default:
		return false
	}
}
