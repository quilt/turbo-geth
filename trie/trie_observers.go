package trie

type TrieObserver interface {
	AccountCreated(hex []byte, size uint)
	AccountDeleted(hex []byte)
	AccountTouched(hex []byte)
	AccountSizeChanged(hex []byte, newSize uint)

	WillUnloadStructNode(key []byte, nodeHash []byte)
	StructNodeLoaded(prefixAsNibbles []byte)
}

var _ TrieObserver = (*NoopTrieObserver)(nil) // make sure that NoopTrieObserver is compliant

// NoopTrieObserver might be used to emulate optional methods in observers
type NoopTrieObserver struct{}

func (*NoopTrieObserver) AccountCreated(_ []byte, _ uint)         {}
func (*NoopTrieObserver) AccountDeleted(_ []byte)                 {}
func (*NoopTrieObserver) AccountTouched(_ []byte)                 {}
func (*NoopTrieObserver) AccountSizeChanged(_ []byte, _ uint)     {}
func (*NoopTrieObserver) WillUnloadStructNode(_ []byte, _ []byte) {}
func (*NoopTrieObserver) StructNodeLoaded(_ []byte)               {}

// TrieObserverMux multiplies the callback methods and sends them to
// all it's children.
type TrieObserversMux struct {
	children []TrieObserver
}

func NewTrieObserverMux() *TrieObserversMux {
	return &TrieObserversMux{make([]TrieObserver, 0)}
}

func (mux *TrieObserversMux) AddChild(child TrieObserver) {
	if child == nil {
		return
	}

	mux.children = append(mux.children, child)
}

func (mux *TrieObserversMux) AccountCreated(hex []byte, size uint) {
	for _, child := range mux.children {
		child.AccountCreated(hex, size)
	}
}

func (mux *TrieObserversMux) AccountDeleted(hex []byte) {
	for _, child := range mux.children {
		child.AccountDeleted(hex)
	}
}

func (mux *TrieObserversMux) AccountTouched(hex []byte) {
	for _, child := range mux.children {
		child.AccountTouched(hex)
	}
}

func (mux *TrieObserversMux) AccountSizeChanged(hex []byte, newSize uint) {
	for _, child := range mux.children {
		child.AccountSizeChanged(hex, newSize)
	}
}

func (mux *TrieObserversMux) WillUnloadStructNode(key []byte, nodeHash []byte) {
	for _, child := range mux.children {
		child.WillUnloadStructNode(key, nodeHash)
	}
}

func (mux *TrieObserversMux) StructNodeLoaded(prefixAsNibbles []byte) {
	for _, child := range mux.children {
		child.StructNodeLoaded(prefixAsNibbles)
	}
}
