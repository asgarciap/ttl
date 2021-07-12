package ttlcache

import (
	"time"
)

const (
	// ItemNotExpire Will avoid the item being expired by TTL, but can still be exired by callback etc.
	ItemNotExpire time.Duration = -1
	// ItemExpireWithGlobalTTL will use the global TTL when set.
	ItemExpireWithGlobalTTL time.Duration = 0
)

func newItem(key string, data interface{}, ttl time.Duration) *item {
	item := &item{
		data: data,
		ttl:  ttl,
		key:  key,
	}
	// since nobody is aware yet of this item, it's safe to touch without lock here
	item.touch()
	return item
}

type item struct {
	key        string
	data       interface{}
	ttl        time.Duration
	expireAt   time.Time
	queueIndex int
}

// Reset the item expiration time
func (item *item) touch() {
	if item.ttl > 0 {
		item.expireAt = time.Now().Add(item.ttl)
	}
}

// expired verify if the item is expired
func (item *item) expired() bool {
	if item.ttl <= 0 {
		return false
	}
	return item.expireAt.Before(time.Now())
}

// ExpiresAt meets the ExpirationHeapEntry interface
func (item *item) ExpiresAt() time.Time {
	return item.expireAt
}

// SetIndex meets the ExpirationHeapEntry interface
func (item *item) SetIndex(index int) {
	item.queueIndex = index
}

// GetIndex meets the ExpirationHeapEntry interface
func (item *item) GetIndex() int {
	return item.queueIndex
}
