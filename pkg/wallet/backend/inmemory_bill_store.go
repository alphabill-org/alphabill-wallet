package backend

import (
	"bytes"
	"sync"
)

type InmemoryBillStore struct {
	blockNumber     uint64
	pubkeyIndex     map[string]map[string]*Bill // pubkey => map[bill_id]bill
	keys            map[string]*Pubkey          // pubkey => hashed pubkeys
	expirationIndex map[uint64][]*expiredBill   // expiration order number => list of expired bills

	mu sync.Mutex
}

func NewInmemoryBillStore() *InmemoryBillStore {
	return &InmemoryBillStore{
		pubkeyIndex:     map[string]map[string]*Bill{},
		keys:            map[string]*Pubkey{},
		expirationIndex: map[uint64][]*expiredBill{},
	}
}

func (s *InmemoryBillStore) GetBlockNumber() (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blockNumber, nil
}

func (s *InmemoryBillStore) SetBlockNumber(blockNumber uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blockNumber = blockNumber
	return nil
}

func (s *InmemoryBillStore) GetBills(pubkey []byte) ([]*Bill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var bills []*Bill
	for _, v := range s.pubkeyIndex[string(pubkey)] {
		bills = append(bills, v)
	}
	return bills, nil
}

func (s *InmemoryBillStore) RemoveBill(pubKey []byte, id []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	bills := s.pubkeyIndex[string(pubKey)]
	delete(bills, string(id))
	return nil
}

func (s *InmemoryBillStore) ContainsBill(pubkey []byte, unitID []byte) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.pubkeyIndex[string(pubkey)]
	_, exists := m[string(unitID)]
	return exists, nil
}

func (s *InmemoryBillStore) GetBill(pubkey []byte, billID []byte) (*Bill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pubkeyIndex[string(pubkey)][string(billID)], nil
}

func (s *InmemoryBillStore) SetBills(pubkey []byte, billsIn ...*Bill) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, bill := range billsIn {
		bills, f := s.pubkeyIndex[string(pubkey)]
		if !f {
			bills = map[string]*Bill{}
			s.pubkeyIndex[string(pubkey)] = bills
		}
		bills[string(bill.Id)] = bill
	}
	return nil
}

func (s *InmemoryBillStore) GetKeys() ([]*Pubkey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keys []*Pubkey
	for _, k := range s.keys {
		keys = append(keys, k)
	}
	return keys, nil
}

func (s *InmemoryBillStore) GetKey(pubkey []byte) (*Pubkey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, k := range s.keys {
		if bytes.Equal(pubkey, k.Pubkey) {
			return k, nil
		}
	}
	return nil, nil
}

func (s *InmemoryBillStore) AddKey(k *Pubkey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, f := s.keys[string(k.Pubkey)]
	if !f {
		s.keys[string(k.Pubkey)] = k
	}
	return nil
}

func (s *InmemoryBillStore) SetBillExpirationTime(expirationBlockNumber uint64, pubkey []byte, unitID []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	expiredBills, f := s.expirationIndex[expirationBlockNumber]
	if !f {
		expiredBills = []*expiredBill{}
		s.expirationIndex[expirationBlockNumber] = expiredBills
	}
	expiredBills = append(expiredBills, &expiredBill{Pubkey: pubkey, UnitID: unitID})
	return nil
}

func (s *InmemoryBillStore) DeleteExpiredBills(blockNumber uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	expiredBills := s.expirationIndex[blockNumber]
	for _, b := range expiredBills {
		err := s.RemoveBill(b.Pubkey, b.UnitID)
		if err != nil {
			return err
		}
	}

	delete(s.expirationIndex, blockNumber)
	return nil
}
