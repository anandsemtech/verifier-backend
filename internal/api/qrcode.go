package api

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type qrCache interface {
	Get(id string) (any, bool)
	Set(id string, data any, duration time.Duration)
}

// QRcodeStore is a storage of qrCodes in a cache.
type QRcodeStore struct {
	cache qrCache
}

// NewQRCodeStore creates a new QRcodeStore.
func NewQRCodeStore(c qrCache) *QRcodeStore {
	return &QRcodeStore{cache: c}
}

// Get returns a QRCode from the cache using the qr code id as key
func (s *QRcodeStore) Get(id uuid.UUID) (*QRCode, error) {
	data, ok := s.cache.Get(s.key() + id.String())
	if !ok {
		return nil, errors.New("sessionID not found")
	}

	qr, ok := data.(QRCode)
	if !ok {
		return nil, errors.New("failed to cast data to QRCode")
	}
	return &qr, nil
}

// Save stores a QRCode in the cache and returns the id of the qr code.
func (s *QRcodeStore) Save(qrCode QRCode) (uuid.UUID, error) {
	id := uuid.New()
	s.cache.Set(s.key()+id.String(), qrCode, 1*time.Hour)
	return id, nil
}

func (s *QRcodeStore) key() string {
	return "qr-code-"
}