package trie

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/ledgerwatch/turbo-geth/common"
	"github.com/ledgerwatch/turbo-geth/common/dbutils"
	"github.com/ledgerwatch/turbo-geth/core/types/accounts"
	"github.com/ledgerwatch/turbo-geth/crypto"
	"github.com/stretchr/testify/assert"
)

type partialObserver struct {
	NoopTrieObserver
}

func (*partialObserver) AccountDeleted(_ []byte) {}

type mockObserver struct {
	createdAccounts map[string]uint
	deletedAccounts map[string]struct{}
	touchedAccounts map[string]int

	unloadedNodes      map[string]int
	unloadedNodeHashes map[string][]byte
	reloadedNodes      map[string]int
}

func newMockObserver() *mockObserver {
	return &mockObserver{
		createdAccounts:    make(map[string]uint),
		deletedAccounts:    make(map[string]struct{}),
		touchedAccounts:    make(map[string]int),
		unloadedNodes:      make(map[string]int),
		unloadedNodeHashes: make(map[string][]byte),
		reloadedNodes:      make(map[string]int),
	}
}

func (m *mockObserver) AccountCreated(hex []byte, size uint) {
	key := hexToKeybytes(hex)
	m.createdAccounts[common.Bytes2Hex(key)] = size
}
func (m *mockObserver) AccountDeleted(hex []byte) {
	key := hexToKeybytes(hex)
	m.deletedAccounts[common.Bytes2Hex(key)] = struct{}{}
}
func (m *mockObserver) AccountTouched(hex []byte) {
	key := hexToKeybytes(hex)
	value, _ := m.touchedAccounts[common.Bytes2Hex(key)]
	value += 1
	m.touchedAccounts[common.Bytes2Hex(key)] = value

}
func (m *mockObserver) AccountSizeChanged(hex []byte, newSize uint) {
	key := hexToKeybytes(hex)
	m.createdAccounts[common.Bytes2Hex(key)] = newSize
}

func (m *mockObserver) WillUnloadStructNode(hex []byte, hash []byte) {
	dictKey := common.Bytes2Hex(hex)
	value, _ := m.unloadedNodes[dictKey]
	value += 1
	m.unloadedNodes[dictKey] = value
	m.unloadedNodeHashes[dictKey] = common.CopyBytes(hash)
}

func (m *mockObserver) StructNodeLoaded(hex []byte) {
	dictKey := common.Bytes2Hex(hex)
	value, _ := m.reloadedNodes[dictKey]
	value += 1
	m.reloadedNodes[dictKey] = value
}

func genAccount() *accounts.Account {
	acc := &accounts.Account{}
	acc.Nonce = 123
	return acc
}

func genNKeys(n int) [][]byte {
	result := make([][]byte, n)
	for i, _ := range result {
		result[i] = crypto.Keccak256([]byte{0x0, 0x0, 0x0, byte(i % 256), byte(i / 256)})
	}
	return result
}

func genByteArrayOfLen(n int) []byte {
	result := make([]byte, n)
	for i := range result {
		result[i] = byte(rand.Intn(255))
	}
	return result
}

func TestObserversAccountsCreateDelete(t *testing.T) {
	trie := newEmpty()

	observer := newMockObserver()

	trie.AddObserver(observer)

	keys := genNKeys(100)

	for _, key := range keys {
		trie.UpdateAccount(key, genAccount())
		trie.Delete(key)
	}

	for _, key := range keys {
		size, ok := observer.createdAccounts[common.Bytes2Hex(key)]
		assert.True(t, ok, "account should be registed as created")
		// we don't care exactly which size it is, but we need to receive something
		assert.True(t, size > 0, "account should have some size")

		_, ok = observer.deletedAccounts[common.Bytes2Hex(key)]
		assert.True(t, ok, "account should be registered as deleted")
	}
}

func TestObserverUpdateAccountSizeViaCode(t *testing.T) {
	rand.Seed(9999)

	trie := newEmpty()

	observer := newMockObserver()

	trie.AddObserver(observer)

	keys := genNKeys(10)

	for _, key := range keys {
		acc := genAccount()
		trie.UpdateAccount(key, acc)
		oldSize, ok := observer.createdAccounts[common.Bytes2Hex(key)]
		assert.True(t, ok, "account should be registed as created")
		assert.True(t, oldSize > 0, "account should have some size")
		code := genByteArrayOfLen(100)
		codeHash := crypto.Keccak256(code)
		acc.CodeHash = common.BytesToHash(codeHash)
		trie.UpdateAccount(key, acc)
		trie.UpdateAccountCode(key, codeNode(code))

		newSize, ok := observer.createdAccounts[common.Bytes2Hex(key)]
		assert.True(t, ok, "account should be registed as created")
		assert.Equal(t, 100, int(newSize)-int(oldSize), "account size should increase when the account code grows")

		code2 := genByteArrayOfLen(50)
		codeHash2 := crypto.Keccak256(code2)
		acc.CodeHash = common.BytesToHash(codeHash2)
		trie.UpdateAccount(key, acc)
		trie.UpdateAccountCode(key, codeNode(code2))

		newSize2, ok := observer.createdAccounts[common.Bytes2Hex(key)]
		assert.True(t, ok, "account should be registed as created")
		assert.Equal(t, -50, int(newSize2)-int(newSize), "account size should decrease when the account code shrinks")
	}
}

func TestObserverUnloadNodes(t *testing.T) {
	rand.Seed(9999)

	trie := newEmpty()

	observer := newMockObserver()

	trie.AddObserver(observer)

	// this test needs a specific trie structure
	//                            ( full )
	//         (full)              (duo)               (duo)
	// (short)(short)(short)    (short)(short)      (short)(short)
	// (acc1) (acc2) (acc3)      (acc4) (acc5)       (acc6) (acc7)
	//
	// to ensure this structure we override prefixes of
	// random account keys with the follwing paths

	prefixes := [][]byte{
		[]byte{0x00, 0x00}, //acc1
		[]byte{0x00, 0x02}, //acc2
		[]byte{0x00, 0x05}, //acc3
		[]byte{0x02, 0x02}, //acc4
		[]byte{0x02, 0x05}, //acc5
		[]byte{0x0A, 0x00}, //acc6
		[]byte{0x0A, 0x03}, //acc7
	}

	keys := genNKeys(7)
	for i := range keys {
		copy(keys[i][:2], prefixes[i])
	}

	storageKeys := genNKeys(10)
	// group all storage keys into a single fullNode
	for i := range storageKeys {
		storageKeys[i][0] = byte(0)
		storageKeys[i][1] = byte(i)
	}

	for _, key := range keys {
		acc := genAccount()
		trie.UpdateAccount(key, acc)

		for i, storageKey := range storageKeys {
			fullKey := dbutils.GenerateCompositeTrieKey(common.BytesToHash(key), common.BytesToHash(storageKey))
			trie.Update(fullKey, []byte(fmt.Sprintf("test-value-%d", i)))
		}
	}

	rootHash := trie.Hash()

	// adding nodes doesn't add anything
	assert.Equal(t, 0, len(observer.reloadedNodes), "adding nodes doesn't add anything")

	// unloading nodes adds to the list
	fmt.Println("evict1")
	trie.EvictLeaf(keys[0])
	fmt.Println("evict2")
	trie.EvictLeaf(keys[1])
	trie.EvictLeaf(keys[2])

	newRootHash := trie.Hash()
	assert.Equal(t, rootHash, newRootHash, "root hash shouldn't change")

	assert.Equal(t, 1, observer.unloadedNodes["000000"], "should unload one full node")
	assert.NotEqual(t, common.Hash{}, observer.unloadedNodeHashes["000000"], "hashes should match")

	for _, key := range keys[:3] {
		hex := keybytesToHex(key)
		storageKey := fmt.Sprintf("%s000000", common.Bytes2Hex(hex))
		assert.Equal(t, 1, observer.unloadedNodes[storageKey], "should unload structure nodes")
	}

	// unloading nodes adds to the list
	trie.EvictLeaf(keys[3])
	trie.EvictLeaf(keys[4])

	newRootHash = trie.Hash()
	assert.Equal(t, rootHash, newRootHash, "root hash shouldn't change")

	assert.Equal(t, 1, observer.unloadedNodes["000200"], "should unload one duo node")
	assert.NotEqual(t, common.Hash{}, observer.unloadedNodeHashes["000200"], "hashes should match")

	for _, key := range keys[3:5] {
		hex := keybytesToHex(key)
		storageKey := fmt.Sprintf("%s000000", common.Bytes2Hex(hex))
		assert.Equal(t, 1, observer.unloadedNodes[storageKey], "should unload structure nodes")
	}

	// unloading nodes adds to the list
	trie.EvictLeaf(keys[5])
	trie.EvictLeaf(keys[6])

	newRootHash = trie.Hash()
	assert.Equal(t, rootHash, newRootHash, "root hash shouldn't change")

	assert.Equal(t, 1, observer.unloadedNodes["000a00"], "should unload one due node")
	assert.NotEqual(t, common.Hash{}, observer.unloadedNodeHashes["000a00"], "hashes should match")

	assert.Equal(t, 1, observer.unloadedNodes["00"], "should unload root tooe")

	for _, key := range keys[5:] {
		hex := keybytesToHex(key)
		storageKey := fmt.Sprintf("%s000000", common.Bytes2Hex(hex))
		assert.Equal(t, 1, observer.unloadedNodes[storageKey], "should unload structure nodes")
	}
}

func TestObserverLoadNodes(t *testing.T) {
	rand.Seed(9999)

	subtrie := newEmpty()

	observer := newMockObserver()

	// this test needs a specific trie structure
	//                            ( full )
	//         (full)              (duo)               (duo)
	// (short)(short)(short)    (short)(short)      (short)(short)
	// (acc1) (acc2) (acc3)      (acc4) (acc5)       (acc6) (acc7)
	//
	// to ensure this structure we override prefixes of
	// random account keys with the follwing paths

	prefixes := [][]byte{
		[]byte{0x00, 0x00}, //acc1
		[]byte{0x00, 0x02}, //acc2
		[]byte{0x00, 0x05}, //acc3
		[]byte{0x02, 0x02}, //acc4
		[]byte{0x02, 0x05}, //acc5
		[]byte{0x0A, 0x00}, //acc6
		[]byte{0x0A, 0x03}, //acc7
	}

	keys := genNKeys(7)
	for i := range keys {
		copy(keys[i][:2], prefixes[i])
	}

	storageKeys := genNKeys(10)
	// group all storage keys into a single fullNode
	for i := range storageKeys {
		storageKeys[i][0] = byte(0)
		storageKeys[i][1] = byte(i)
	}

	for _, key := range keys {
		acc := genAccount()
		subtrie.UpdateAccount(key, acc)

		for i, storageKey := range storageKeys {
			fullKey := dbutils.GenerateCompositeTrieKey(common.BytesToHash(key), common.BytesToHash(storageKey))
			subtrie.Update(fullKey, []byte(fmt.Sprintf("test-value-%d", i)))
		}
	}

	trie := newEmpty()
	trie.AddObserver(observer)

	trie.hook([]byte{}, subtrie.root)

	// fullNode
	assert.Equal(t, 1, observer.reloadedNodes["000000"], "should reload structure nodes")
	// duoNode
	assert.Equal(t, 1, observer.reloadedNodes["000200"], "should reload structure nodes")
	// duoNode
	assert.Equal(t, 1, observer.reloadedNodes["000a00"], "should reload structure nodes")
	// root
	assert.Equal(t, 1, observer.reloadedNodes["00"], "should reload structure nodes")

	// check storages (should have a single fullNode per account)
	for _, key := range keys {
		hex := keybytesToHex(key)
		storageKey := fmt.Sprintf("%s000000", common.Bytes2Hex(hex[:len(hex)-1]))
		assert.Equal(t, 1, observer.reloadedNodes[storageKey], "should reload structure nodes")
	}
}

func TestObserverAccountTouches(t *testing.T) {
	rand.Seed(9999)

	trie := newEmpty()

	observer := newMockObserver()

	trie.AddObserver(observer)

	keys := genNKeys(1)

	for _, key := range keys {
		// creation touches the account
		acc := genAccount()
		trie.UpdateAccount(key, acc)
		assert.Equal(t, 1, observer.touchedAccounts[common.Bytes2Hex(key)])

		// updating touches the account
		code := genByteArrayOfLen(100)
		codeHash := crypto.Keccak256(code)
		acc.CodeHash = common.BytesToHash(codeHash)
		trie.UpdateAccount(key, acc)
		assert.Equal(t, 2, observer.touchedAccounts[common.Bytes2Hex(key)])

		// updating code touches the account
		trie.UpdateAccountCode(key, codeNode(code))
		assert.Equal(t, 3, observer.touchedAccounts[common.Bytes2Hex(key)])

		// changing storage touches the account
		storageKey := genNKeys(1)[0]
		fullKey := dbutils.GenerateCompositeTrieKey(common.BytesToHash(key), common.BytesToHash(storageKey))

		trie.Update(fullKey, []byte("value-1"))
		assert.Equal(t, 4, observer.touchedAccounts[common.Bytes2Hex(key)])

		trie.Update(fullKey, []byte("value-2"))
		assert.Equal(t, 5, observer.touchedAccounts[common.Bytes2Hex(key)])

		// getting storage touches the account
		_, ok := trie.Get(fullKey)
		assert.True(t, ok, "should be able to receive storage")
		assert.Equal(t, 6, observer.touchedAccounts[common.Bytes2Hex(key)])

		// deleting storage touches the account
		trie.Delete(fullKey)
		assert.Equal(t, 7, observer.touchedAccounts[common.Bytes2Hex(key)])

		// getting code touches the account
		_, ok = trie.GetAccountCode(key)
		assert.True(t, ok, "should be able to receive code")
		assert.Equal(t, 8, observer.touchedAccounts[common.Bytes2Hex(key)])

		// getting account touches the account
		_, ok = trie.GetAccount(key)
		assert.True(t, ok, "should be able to receive account")
		assert.Equal(t, 9, observer.touchedAccounts[common.Bytes2Hex(key)])
	}
}

func TestObserverPartial(t *testing.T) {
	trie := newEmpty()

	observer := &partialObserver{} // only implements `AccountDeleted`
	trie.AddObserver(observer)

	keys := genNKeys(1)
	for _, key := range keys {
		acc := genAccount()
		trie.UpdateAccount(key, acc)

		code := genByteArrayOfLen(100)
		codeHash := crypto.Keccak256(code)
		acc.CodeHash = common.BytesToHash(codeHash)
		trie.UpdateAccount(key, acc)
		trie.UpdateAccountCode(key, codeNode(code))

		trie.Delete(key)
	}
	// no crashes expected
}

func TestObserverMux(t *testing.T) {
	trie := newEmpty()

	observer1 := newMockObserver()
	observer2 := newMockObserver()
	mux := NewTrieObserverMux()
	mux.AddChild(observer1)
	mux.AddChild(observer2)

	trie.AddObserver(mux)

	keys := genNKeys(100)
	for _, key := range keys {
		acc := genAccount()
		trie.UpdateAccount(key, acc)

		code := genByteArrayOfLen(100)
		codeHash := crypto.Keccak256(code)
		acc.CodeHash = common.BytesToHash(codeHash)
		trie.UpdateAccount(key, acc)
		trie.UpdateAccountCode(key, codeNode(code))

		_, ok := trie.GetAccount(key)
		assert.True(t, ok, "acount should be found")

	}

	trie.Hash()

	for i, key := range keys {
		if i < 80 {
			trie.Delete(key)
		} else {
			trie.EvictLeaf(key)
		}
	}

	assert.Equal(t, observer1.createdAccounts, observer2.createdAccounts, "should propagate created events")
	assert.Equal(t, observer1.deletedAccounts, observer2.deletedAccounts, "should propagate deleted events")
	assert.Equal(t, observer1.touchedAccounts, observer2.touchedAccounts, "should propagate touched events")

	assert.Equal(t, observer1.unloadedNodes, observer2.unloadedNodes, "should propagage unloads")
	assert.Equal(t, observer1.unloadedNodeHashes, observer2.unloadedNodeHashes, "should propagage unloads")
	assert.Equal(t, observer1.reloadedNodes, observer2.reloadedNodes, "should propagage reloads")
}
