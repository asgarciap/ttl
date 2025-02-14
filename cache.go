package ttl

import (
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// CheckExpireCallback is used as a callback for an external check on item expiration
type CheckExpireCallback func(key string, value interface{}) bool

// ExpireCallback is used as a callback on item expiration or when notifying of an item new to the cache
// Note that ExpireReasonCallback will be the succesor of this function in the next major release.
type ExpireCallback func(key string, value interface{})

// ExpireReasonCallback is used as a callback on item expiration with extra information why the item expired.
type ExpireReasonCallback func(key string, reason EvictionReason, value interface{})

// LoaderFunction can be supplied to retrieve an item where a cache miss occurs. Supply an item specific ttl or Duration.Zero
type LoaderFunction func(key string) (data interface{}, ttl time.Duration, err error)

// SimpleCache interface enables a quick-start. Interface for basic usage.
type SimpleCache interface {
	Get(key string) (interface{}, error)
	GetWithTTL(key string) (interface{}, time.Duration, error)
	Set(key string, data interface{}) error
	SetTTL(ttl time.Duration) error
	SetWithTTL(key string, data interface{}, ttl time.Duration) error
	Remove(key string) error
	Close() error
	Purge() error
}

// Cache is a synchronized map of items that can auto-expire once stale
type Cache struct {
	mutex                  sync.Mutex
	ttl                    time.Duration
	items                  map[string]*item
	loaderLock             *singleflight.Group
	expireCallback         ExpireCallback
	expireReasonCallback   ExpireReasonCallback
	checkExpireCallback    CheckExpireCallback
	newItemCallback        ExpireCallback
	expirationHeap         *ExpirationHeap
	expirationNotification chan bool
	expirationTime         time.Time
	skipTTLExtension       bool
	shutdownSignal         chan (chan struct{})
	isShutDown             bool
	loaderFunction         LoaderFunction
	sizeLimit              int
	metrics                Metrics
}

// EvictionReason is an enum that explains why an item was evicted
type EvictionReason int

const (
	// Removed : explicitly removed from cache via API call
	Removed EvictionReason = iota
	// EvictedSize : evicted due to exceeding the cache size
	EvictedSize
	// Expired : the time to live is zero and therefore the item is removed
	Expired
	// Closed : the cache was closed
	Closed
)

const (
	// ErrClosed is raised when operating on a cache where Close() has already been called.
	ErrClosed = constError("cache already closed")
	// ErrNotFound indicates that the requested key is not present in the cache
	ErrNotFound = constError("key not found")
)

type constError string

func (err constError) Error() string {
	return string(err)
}

func (cache *Cache) getItem(key string) (*item, bool, bool) {
	item, exists := cache.items[key]
	if !exists || item.expired() {
		return nil, false, false
	}

	if item.ttl >= 0 && (item.ttl > 0 || cache.ttl > 0) {
		if cache.ttl > 0 && item.ttl == 0 {
			item.ttl = cache.ttl
		}

		if !cache.skipTTLExtension {
			item.touch()
		}
		cache.expirationHeap.Update(item)
	}

	expirationNotification := false
	if cache.expirationTime.After(time.Now().Add(item.ttl)) {
		expirationNotification = true
	}
	return item, exists, expirationNotification
}

func (cache *Cache) startExpirationProcessing() {
	timer := time.NewTimer(time.Hour)
	for {
		var sleepTime time.Duration
		cache.mutex.Lock()
		if cache.expirationHeap.Len() > 0 {
			sleepTime = time.Until(cache.expirationHeap.Peek().ExpiresAt())
			if sleepTime < 0 && cache.expirationHeap.Peek().ExpiresAt().IsZero() {
				sleepTime = time.Hour
			} else if sleepTime < 0 {
				sleepTime = time.Microsecond
			}
			if cache.ttl > 0 {
				sleepTime = min(sleepTime, cache.ttl)
			}

		} else if cache.ttl > 0 {
			sleepTime = cache.ttl
		} else {
			sleepTime = time.Hour
		}

		cache.expirationTime = time.Now().Add(sleepTime)
		cache.mutex.Unlock()
		timer.Reset(sleepTime)
		select {
		case shutdownFeedback := <-cache.shutdownSignal:
			timer.Stop()
			cache.mutex.Lock()
			if cache.expirationHeap.Len() > 0 {
				cache.evictjob(Closed)
			}
			cache.mutex.Unlock()
			shutdownFeedback <- struct{}{}
			return
		case <-timer.C:
			timer.Stop()
			cache.mutex.Lock()
			if cache.expirationHeap.Len() == 0 {
				cache.mutex.Unlock()
				continue
			}
			cache.cleanjob()
			cache.mutex.Unlock()

		case <-cache.expirationNotification:
			timer.Stop()
			continue
		}
	}
}

func (cache *Cache) checkExpirationCallback(item *item, reason EvictionReason) {
	if cache.expireCallback != nil {
		go cache.expireCallback(item.key, item.data)
	}
	if cache.expireReasonCallback != nil {
		go cache.expireReasonCallback(item.key, reason, item.data)
	}
}

func (cache *Cache) removeItem(item *item, reason EvictionReason) {
	switch reason {
	case EvictedSize:
		cache.metrics.EvictedFull++
	case Expired:
		cache.metrics.EvictedExpired++
	case Closed:
		cache.metrics.EvictedClosed++
	}
	cache.checkExpirationCallback(item, reason)
	cache.expirationHeap.Remove(item)
	delete(cache.items, item.key)

}

func (cache *Cache) evictjob(reason EvictionReason) {
	for citem := cache.expirationHeap.Peek(); citem != nil; citem = cache.expirationHeap.Peek() {
		cache.removeItem(citem.(*item), reason)
	}
}

func (cache *Cache) cleanjob() {
	for citem := cache.expirationHeap.Peek(); citem != nil && citem.(*item).expired(); citem = cache.expirationHeap.Peek() {
		nitem := citem.(*item)
		if cache.checkExpireCallback != nil {
			if !cache.checkExpireCallback(nitem.key, nitem.data) {
				nitem.touch()
				cache.expirationHeap.Update(citem)
				continue
			}
		}
		cache.removeItem(nitem, Expired)
	}
}

// Close calls Purge after stopping the goroutine that does ttl checking, for a clean shutdown.
// The cache is no longer cleaning up after the first call to Close, repeated calls are safe and return ErrClosed.
func (cache *Cache) Close() error {
	cache.mutex.Lock()
	var err error
	if !cache.isShutDown {
		cache.isShutDown = true
		cache.mutex.Unlock()
		feedback := make(chan struct{})
		cache.shutdownSignal <- feedback
		<-feedback
		close(cache.shutdownSignal)
		err = cache.Purge()
	} else {
		cache.mutex.Unlock()
		err = ErrClosed
	}
	return err
}

// Set is a thread-safe way to add new items to the map.
func (cache *Cache) Set(key string, data interface{}) error {
	return cache.SetWithTTL(key, data, ItemExpireWithGlobalTTL)
}

// SetWithTTL is a thread-safe way to add new items to the map with individual ttl.
func (cache *Cache) SetWithTTL(key string, data interface{}, ttl time.Duration) error {
	cache.mutex.Lock()
	if cache.isShutDown {
		cache.mutex.Unlock()
		return ErrClosed
	}
	citem, exists, _ := cache.getItem(key)

	if exists {
		citem.data = data
		citem.ttl = ttl
	} else {
		if cache.sizeLimit != 0 && len(cache.items) >= cache.sizeLimit {
			cache.removeItem(cache.expirationHeap.Peek().(*item), EvictedSize)
		}
		citem = newItem(key, data, ttl)
		cache.items[key] = citem
	}
	cache.metrics.Inserted++

	if citem.ttl >= 0 && (citem.ttl > 0 || cache.ttl > 0) {
		if cache.ttl > 0 && citem.ttl == 0 {
			citem.ttl = cache.ttl
		}
		citem.touch()
	}

	if exists {
		cache.expirationHeap.Update(citem)
	} else {
		cache.expirationHeap.Add(citem)
	}

	cache.mutex.Unlock()
	if !exists && cache.newItemCallback != nil {
		cache.newItemCallback(key, data)
	}
	cache.expirationNotification <- true
	return nil
}

// Get is a thread-safe way to lookup items
// Every lookup, also touches the item, hence extending it's life
func (cache *Cache) Get(key string) (interface{}, error) {
	data, _, err := cache.GetByLoader(key, nil)
	return data, err
}

// GetWithTTL has exactly the same behaviour as Get but also returns
// the remaining TTL for an specific item at the moment it its retrieved
func (cache *Cache) GetWithTTL(key string) (interface{}, time.Duration, error) {
	return cache.GetByLoader(key, nil)
}

// GetByLoader can take a per key loader function (ie. to propagate context)
func (cache *Cache) GetByLoader(key string, customLoaderFunction LoaderFunction) (interface{}, time.Duration, error) {
	cache.mutex.Lock()
	if cache.isShutDown {
		cache.mutex.Unlock()
		return nil, 0, ErrClosed
	}

	cache.metrics.Hits++
	item, exists, triggerExpirationNotification := cache.getItem(key)

	var dataToReturn interface{}
	ttlToReturn := time.Duration(0)
	if exists {
		cache.metrics.Retrievals++
		dataToReturn = item.data
		ttlToReturn = time.Until(item.expireAt)
		if ttlToReturn < 0 {
			ttlToReturn = 0
		}
	}

	var err error
	if !exists {
		cache.metrics.Misses++
		err = ErrNotFound
	}

	loaderFunction := cache.loaderFunction
	if customLoaderFunction != nil {
		loaderFunction = customLoaderFunction
	}

	if loaderFunction == nil || exists {
		cache.mutex.Unlock()
	}

	if loaderFunction != nil && !exists {
		type loaderResult struct {
			data interface{}
			ttl  time.Duration
		}
		ch := cache.loaderLock.DoChan(key, func() (interface{}, error) {
			// cache is not blocked during io
			invokeData, ttl, err := cache.invokeLoader(key, loaderFunction)
			lr := &loaderResult{
				data: invokeData,
				ttl:  ttl,
			}
			return lr, err
		})
		cache.mutex.Unlock()
		res := <-ch
		dataToReturn = res.Val.(*loaderResult).data
		ttlToReturn = res.Val.(*loaderResult).ttl
		err = res.Err
	}

	if triggerExpirationNotification {
		cache.expirationNotification <- true
	}

	return dataToReturn, ttlToReturn, err
}

func (cache *Cache) invokeLoader(key string, loaderFunction LoaderFunction) (dataToReturn interface{}, ttl time.Duration, err error) {
	dataToReturn, ttl, err = loaderFunction(key)
	if err == nil {
		err = cache.SetWithTTL(key, dataToReturn, ttl)
		if err != nil {
			dataToReturn = nil
			ttl = 0
		}
	}
	return dataToReturn, ttl, err
}

// Remove removes an item from the cache if it exists, triggers expiration callback when set. Can return ErrNotFound if the entry was not present.
func (cache *Cache) Remove(key string) error {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	if cache.isShutDown {
		return ErrClosed
	}

	object, exists := cache.items[key]
	if !exists {
		return ErrNotFound
	}
	cache.removeItem(object, Removed)

	return nil
}

// Count returns the number of items in the cache. Returns zero when the cache has been closed.
func (cache *Cache) Count() int {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	if cache.isShutDown {
		return 0
	}
	length := len(cache.items)
	return length
}

// GetKeys returns all keys of items in the cache. Returns nil when the cache has been closed.
func (cache *Cache) GetKeys() []string {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()

	if cache.isShutDown {
		return nil
	}
	keys := make([]string, len(cache.items))
	i := 0
	for k := range cache.items {
		keys[i] = k
		i++
	}
	return keys
}

// SetTTL sets the global TTL value for items in the cache, which can be overridden at the item level.
func (cache *Cache) SetTTL(ttl time.Duration) error {
	cache.mutex.Lock()

	if cache.isShutDown {
		cache.mutex.Unlock()
		return ErrClosed
	}
	cache.ttl = ttl
	cache.mutex.Unlock()
	cache.expirationNotification <- true
	return nil
}

// SetExpirationCallback sets a callback that will be called when an item expires
func (cache *Cache) SetExpirationCallback(callback ExpireCallback) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	cache.expireCallback = callback
}

// SetExpirationReasonCallback sets a callback that will be called when an item expires, includes reason of expiry
func (cache *Cache) SetExpirationReasonCallback(callback ExpireReasonCallback) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	cache.expireReasonCallback = callback
}

// SetCheckExpirationCallback sets a callback that will be called when an item is about to expire
// in order to allow external code to decide whether the item expires or remains for another TTL cycle
func (cache *Cache) SetCheckExpirationCallback(callback CheckExpireCallback) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	cache.checkExpireCallback = callback
}

// SetNewItemCallback sets a callback that will be called when a new item is added to the cache
func (cache *Cache) SetNewItemCallback(callback ExpireCallback) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	cache.newItemCallback = callback
}

// SkipTTLExtensionOnHit allows the user to change the cache behaviour. When this flag is set to true it will
// no longer extend TTL of items when they are retrieved using Get, or when their expiration condition is evaluated
// using SetCheckExpirationCallback.
func (cache *Cache) SkipTTLExtensionOnHit(value bool) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	cache.skipTTLExtension = value
}

// SetLoaderFunction allows you to set a function to retrieve cache misses. The signature matches that of the Get function.
// Additional Get calls on the same key block while fetching is in progress (groupcache style).
func (cache *Cache) SetLoaderFunction(loader LoaderFunction) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	cache.loaderFunction = loader
}

// Purge will remove all entries
func (cache *Cache) Purge() error {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	cache.metrics.EvictedClosed += int64(len(cache.items))
	cache.items = make(map[string]*item)
	cache.expirationHeap = NewExpirationHeap()
	return nil
}

// SetCacheSizeLimit sets a limit to the amount of cached items.
// If a new item is getting cached, the closes item to being timed out will be replaced
// Set to 0 to turn off
func (cache *Cache) SetCacheSizeLimit(limit int) {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	cache.sizeLimit = limit
}

// NewCache is a helper to create instance of the Cache struct
func NewCache() *Cache {

	shutdownChan := make(chan chan struct{})

	cache := &Cache{
		items:                  make(map[string]*item),
		loaderLock:             &singleflight.Group{},
		expirationHeap:         NewExpirationHeap(),
		expirationNotification: make(chan bool),
		expirationTime:         time.Now(),
		shutdownSignal:         shutdownChan,
		isShutDown:             false,
		loaderFunction:         nil,
		sizeLimit:              0,
		metrics:                Metrics{},
	}
	go cache.startExpirationProcessing()
	return cache
}

// GetMetrics exposes the metrics of the cache. This is a snapshot copy of the metrics.
func (cache *Cache) GetMetrics() Metrics {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	return cache.metrics
}

// Touch resets the TTL of the key when it exists, returns ErrNotFound if the key is not present.
func (cache *Cache) Touch(key string) error {
	cache.mutex.Lock()
	defer cache.mutex.Unlock()
	item, exists := cache.items[key]
	if !exists {
		return ErrNotFound
	}
	item.touch()
	return nil
}

func min(duration time.Duration, second time.Duration) time.Duration {
	if duration < second {
		return duration
	}
	return second
}
