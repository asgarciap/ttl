# TTL Cache + ExpirationHeap
Provides an in-memory cache with expiration and an ExpirationHeap that order the items using a TTL value

[![Documentation](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/github.com/asgarciap/ttl)
[![Release](https://img.shields.io/github/release/asgarciap/ttl.svg?label=Release)](https://github.com/asgarciap/ttl/releases)

`ttl.Cache` is a simple key/value cache in golang with the following functions:

1. Expiration of items based on time, or custom function
2. Loader function to retrieve missing keys can be provided. Additional `Get` calls on the same key block while fetching is in progress (groupcache style).
3. Individual expiring time or global expiring time, you can choose
4. Auto-Extending expiration on `Get` -or- DNS style TTL, see `SkipTTLExtensionOnHit(bool)`
5. Can trigger callback on key expiration
6. Cleanup resources by calling `Close()` at end of lifecycle.
7. Thread-safe with comprehensive testing suite. This code is in production at bol.com on critical systems.

Note (issue #25): by default, due to historic reasons, the TTL will be reset on each cache hit and you need to explicitly configure the cache to use a TTL that will not get extended.

`ttl.ExpirationHeap` is a heap priority queue implementation but using an expiration time as the priority, this means that
the entries in the queue are ordered using a TTL value.
A NotifyCh es used to know when the first element in the queue is updated.


[![Build Status](https://github.com/asgarciap/ttl/actions/workflows/ci.yml/badge.svg)](https://github.com/asgarciap/ttl/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/asgarciap/ttl)](https://goreportcard.com/report/github.com/asgarciap/ttl)
[![Coverage Status](https://coveralls.io/repos/github/asgarciap/ttl/badge.svg?branch=master)](https://coveralls.io/github/asgarciap/ttl?branch=master)
[![license](https://img.shields.io/github/license/asgarciap/ttl.svg?maxAge=2592000)](https://github.com/asgarciap/ttl/LICENSE)

## Usage

`go get github.com/asgarciap/ttl/v3`

You can copy it as a full standalone demo program. The first snippet is basic usage, where the second exploits more options in the cache.

### ttl.Cache
Basic:
```go
package main

import (
	"fmt"
	"time"

	"github.com/asgarciap/ttl/v3"
)

var notFound = ttl.ErrNotFound

func main() {
	var cache ttl.SimpleCache = ttl.NewCache()

	cache.SetTTL(time.Duration(10 * time.Second))
	cache.Set("MyKey", "MyValue")
	cache.Set("MyNumber", 1000)

	if val, err := cache.Get("MyKey"); err != notFound {
		fmt.Printf("Got it: %s\n", val)
	}

	cache.Remove("MyNumber")
	cache.Purge()
	cache.Close()
}
```

Advanced:
```go
package main

import (
	"fmt"
	"time"

	"github.com/asgarciap/ttl/v3"
)

var (
	notFound = ttl.ErrNotFound
	isClosed = ttl.ErrClosed
)

func main() {
	newItemCallback := func(key string, value interface{}) {
		fmt.Printf("New key(%s) added\n", key)
	}
	checkExpirationCallback := func(key string, value interface{}) bool {
		if key == "key1" {
			// if the key equals "key1", the value
			// will not be allowed to expire
			return false
		}
		// all other values are allowed to expire
		return true
	}

	expirationCallback := func(key string, reason ttlcache.EvictionReason, value interface{}) {
		fmt.Printf("This key(%s) has expired because of %s\n", key, reason)
	}

	loaderFunction := func(key string) (data interface{}, ttl time.Duration, err error) {
		ttl = time.Second * 300
		data, err = getFromNetwork(key)

		return data, ttl, err
	}

	cache := ttl.NewCache()
	cache.SetTTL(time.Duration(10 * time.Second))
	cache.SetExpirationReasonCallback(expirationCallback)
	cache.SetLoaderFunction(loaderFunction)
	cache.SetNewItemCallback(newItemCallback)
	cache.SetCheckExpirationCallback(checkExpirationCallback)
	cache.SetCacheSizeLimit(2)

	cache.Set("key", "value")
	cache.SetWithTTL("keyWithTTL", "value", 10*time.Second)

	if value, exists := cache.Get("key"); exists == nil {
		fmt.Printf("Got value: %v\n", value)
	}
	if v, ttl, e := cache.GetWithTTL("key"); e == nil {
		fmt.Printf("Got value: %v which still have a ttl of: %v\n", v, ttl)
	}
	count := cache.Count()
	if result := cache.Remove("keyNNN"); result == notFound {
		fmt.Printf("Not found, %d items left\n", count)
	}
	cache.Set("key6", "value")
	cache.Set("key7", "value")
	metrics := cache.GetMetrics()
	fmt.Printf("Total inserted: %d\n", metrics.Inserted)

	cache.Close()

}

func getFromNetwork(key string) (string, error) {
	time.Sleep(time.Millisecond * 30)
	return "value", nil
}
```
### ttl.ExpirationHeap
Any struct can be used as the heap entry as long the ExpirationHeapEntry interface is implemented.

```go
package main

import (
	"fmt"
	"time"

	"github.com/asgarciap/ttl/v3"
)

type struct MyStruct {
	data string
	index int
	validUntil time.Time
}

func (m *MyStruct) SetIndex(index int) {
	m.index = index
}

func (m *MyStruct) GetIndex() int {
	return m.index
}

func (m *MyStruct) ExpiresAt() {
	return m.validUntil
}

func main() {
	heap := ttl.NewExpirationHeap()
	//Just start a simple goroutine to check when the first position is updated
	go func() {
		for {
			<-heap.NotifyCh
			fmt.Printf("Heap first element was updated")
		}
	}()
	entry := &MyStruct{
		data: "MyValue",
		validUntil: time.Now().Add(10*time.Second),
	}
	heap.Add(entry)
	entry2 := &MyStruct{
		data: "MyValue_2",
		validUntil: time.Now().Add(5*time.Second),
	}
	//Get the first element without removing it from the heap
	v := heap.Peek()
	//This should print: Got it MyValue_2
	fmt.Printf("Got it: %v":,v.(*MyStruct).value)
	//after updating the TTL, the item should be moved to the first position
	entry.validUntil = time.Now().Add(1*time.Second)
	heap.Update(entry)
	//Get the first position and remove it from the heap
	v = heap.First()
	//This should print: Got it: MyValue
	fmt.Printf("Got it: %v":,v.(*MyStruct).value)
}
```


### TTL Cache - Some design considerations

1. The complexity of the current cache is already quite high. Therefore not all requests can be implemented in a straight-forward manner.
2. The locking should be done only in the exported functions and `startExpirationProcessing` of the Cache struct. Else data races can occur or recursive locks are needed, which are both unwanted.
3. I prefer correct functionality over fast tests. It's ok for new tests to take seconds to proof something.

### Original Project

TTLCache was forked from [ReneKroon/ttlcache](https://github.com/ReneKroon/ttlcache) which in turn is a fork from  [wunderlist/ttlcache](https://github.com/wunderlist/ttlcache)
to add extra functions not avaiable in the original scope.

The main differences that [ReneKroon/ttlcache](https://github.com/ReneKroon/ttlcache) has from the original project are:

1. A item can store any kind of object, previously, only strings could be saved
2. Optionally, you can add callbacks too: check if a value should expire, be notified if a value expires, and be notified when new values are added to the cache
3. The expiration can be either global or per item
4. Items can exist without expiration time (`time.Zero`)
5. Expirations and callbacks are realtime. Don't have a pooling time to check anymore, now it's done with a heap.
6. A cache count limiter

This fork differs in the following aspects:

1. We add a new `GetWithTTL` function to get the available TTL (as `time.Duration`) that an item has when recovering from the cache
2. We rename the `priority_queue.go` file/struct to `ExpirationHeap` and expose it so we can use it independently
3. Metrics for eviction are more detailed (`EvictedFull`, `EvictedClosed`, `EvictedExpired`)
3. 100% test coverage
4. Build checks are now done with github actions
