package state

import (
	"fmt"
	"github.com/idena-network/idena-go/common"
	"github.com/idena-network/idena-go/log"
	"sync"
)

type account struct {
	stateObject *stateAccount
	nonce       uint32
}

type NonceCache struct {
	fallback *StateDB

	mu sync.Mutex

	accounts map[common.Address]map[uint16]*account
	Coinbase common.Address
	Version  int64
}

func NewNonceCache(sdb *StateDB) (*NonceCache, error) {
	readonly, err := sdb.Readonly(-1)
	if err != nil {
		return nil, err
	}
	return &NonceCache{
		fallback: readonly,
		accounts: make(map[common.Address]map[uint16]*account),
		Version:  readonly.Version(),
	}, nil
}

// GetNonce returns the canonical nonce for the managed or unmanaged account.
// Because GetNonce mutates the DB, we must take a write lock.
func (ns *NonceCache) GetNonce(addr common.Address, epoch uint16) uint32 {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	nonce := ns.getAccount(addr, epoch).nonce
	if addr == ns.Coinbase {
		log.Info("NonceCache.GetNonce", "nonce", nonce, "version", ns.Version, "addr", fmt.Sprintf("%p", ns))
	}
	return nonce
}

func (ns *NonceCache) Lock() {
	ns.mu.Lock()
}

func (ns *NonceCache) UnLock() {
	ns.mu.Unlock()
}

func (ns *NonceCache) ReloadFallback() error {
	readonly, err := ns.fallback.Readonly(-1)
	if err != nil {
		return err
	}
	ns.fallback = readonly
	return nil
}

// SetNonce sets the new canonical nonce for the managed state
func (ns *NonceCache) SetNonce(addr common.Address, txEpoch uint16, nonce uint32) {
	ns.mu.Lock()
	defer ns.mu.Unlock()

	ns.UnsafeSetNonce(addr, txEpoch, nonce)
}

func (ns *NonceCache) UnsafeSetNonce(addr common.Address, txEpoch uint16, nonce uint32) {
	acc := ns.getAccount(addr, txEpoch)
	if addr == ns.Coinbase {
		log.Info("NonceCache.SetNonce", "current", acc.nonce, "new", nonce, "version", ns.Version, "addr", fmt.Sprintf("%p", ns))
	}
	if acc.nonce < nonce {
		acc.nonce = nonce
	}
}

// populate the managed state
func (ns *NonceCache) getAccount(addr common.Address, epoch uint16) *account {
	if epochs, ok := ns.accounts[addr]; !ok {
		so := ns.fallback.GetOrNewAccountObject(addr)
		ns.accounts[addr] = make(map[uint16]*account)
		ns.accounts[addr][epoch] = ns.newAccount(so, epoch)
		if addr == ns.Coinbase {
			log.Info("NonceCache.getAccount: addr is not found in memory, read from state", "nonce", ns.accounts[addr][epoch].nonce, "version", ns.Version, "addr", fmt.Sprintf("%p", ns))
		}
	} else {
		if acc, ok := epochs[epoch]; !ok {
			so := ns.fallback.GetOrNewAccountObject(addr)
			ns.accounts[addr][epoch] = ns.newAccount(so, epoch)
			if addr == ns.Coinbase {
				log.Info("NonceCache.getAccount: epoch is not found in memory, read from state", "nonce", ns.accounts[addr][epoch].nonce, "version", ns.Version, "addr", fmt.Sprintf("%p", ns))
			}
		} else {
			// Always make sure the state account nonce isn't actually higher
			// than the tracked one.
			so := ns.fallback.getStateAccount(addr)
			if addr == ns.Coinbase && so != nil {
				log.Info("NonceCache.getAccount", "state-nonce", so.Nonce(), "version", ns.Version, "addr", fmt.Sprintf("%p", ns))
			}
			if so != nil && acc.nonce < so.Nonce() && so.Epoch() == epoch {
				if addr == ns.Coinbase {
					log.Info("NonceCache.getAccount: nonce in memory is less that state-nonce", "state-nonce", so.Nonce(), "in-mem", acc.nonce, "version", ns.Version, "addr", fmt.Sprintf("%p", ns))
				}
				ns.accounts[addr][epoch] = ns.newAccount(so, epoch)
				if addr == ns.Coinbase {
					log.Info("NonceCache.getAccount: recreated account in memory", "nonce", ns.accounts[addr][epoch].nonce, "version", ns.Version, "addr", fmt.Sprintf("%p", ns))
				}
			}
		}
	}

	return ns.accounts[addr][epoch]
}

func (ns *NonceCache) newAccount(so *stateAccount, epoch uint16) *account {

	nonce := so.Nonce()
	if so.Epoch() < ns.fallback.Epoch() || so.Epoch() < epoch {
		nonce = 0
	}

	return &account{so, nonce}
}

func (ns *NonceCache) Clear() {
	ns.accounts = make(map[common.Address]map[uint16]*account)
}
