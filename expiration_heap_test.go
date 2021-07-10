package ttlcache

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type testExpirationItem struct {
	validUntil time.Time
	index      int
	data       string
}

func (item *testExpirationItem) ExpiresAt() time.Time {
	return item.validUntil
}

func (item *testExpirationItem) SetIndex(index int) {
	item.index = index
}

func (item *testExpirationItem) GetIndex() int {
	return item.index
}

func (item *testExpirationItem) String() string {
	return fmt.Sprintf("Item Idx: %d ExpiresAt: %v", item.index, item.validUntil)
}

func newTestItem(data string, ttl time.Duration) *testExpirationItem {
	return &testExpirationItem{
		validUntil: time.Now().Add(ttl),
		data:       data,
		index:      EntryNotIndexed,
	}
}

func TestExpirationHeapAdd(t *testing.T) {
	heap := NewExpirationHeap()
	for i := 0; i < 10; i++ {
		heap.Add(newTestItem(fmt.Sprintf("key_%d", i), time.Second))
	}
	assert.Equal(t, heap.Len(), 10, "Expected heap to have 10 elements")
}

func TestExpirationHeapFirst(t *testing.T) {
	heap := NewExpirationHeap()
	for i := 0; i < 10; i++ {
		heap.Add(newTestItem(fmt.Sprintf("key_%d", i), time.Second))
	}
	for i := 0; i < 5; i++ {
		item := heap.First()
		assert.Equal(t, fmt.Sprintf("%T", item), "*ttlcache.testExpirationItem", "Expected 'item' to be a '*ttlcache.testExpirationItem'")
	}
	assert.Equal(t, heap.Len(), 5, "Expected heap to have 5 elements")
	for i := 0; i < 5; i++ {
		item := heap.First()
		assert.Equal(t, fmt.Sprintf("%T", item), "*ttlcache.testExpirationItem", "Expected 'item' to be a '*ttlcache.testExpirationItem'")
	}
	assert.Equal(t, heap.Len(), 0, "Expected heap to have 0 elements")

	item := heap.First()
	assert.Nil(t, item, "ttlcache.testExpirationItem", "Expected 'item' to be nil")
}

func TestExpirationHeapCheckOrder(t *testing.T) {
	heap := NewExpirationHeap()
	for i := 10; i > 0; i-- {
		heap.Add(newTestItem(fmt.Sprintf("key_%d", i), time.Duration(i)*time.Second))
	}
	for i := 1; i <= 10; i++ {
		item := heap.First()
		assert.Equal(t, item.(*testExpirationItem).data, fmt.Sprintf("key_%d", i), "error")
	}
}

func TestExpirationHeapRemove(t *testing.T) {
	heap := NewExpirationHeap()
	var itemRemove *testExpirationItem
	for i := 0; i < 5; i++ {
		data := fmt.Sprintf("key_%d", i)
		newItem := newTestItem(data, time.Duration(i)*time.Second)
		heap.Add(newItem)
		assert.NotEqual(t, newItem.GetIndex(), EntryNotIndexed, "Entry should be indexed now")
		if i == 2 {
			itemRemove = newItem
		}
	}
	assert.Equal(t, heap.Len(), 5, "Expected heap to have 5 elements")
	heap.Remove(itemRemove)
	assert.Equal(t, heap.Len(), 4, "Expected heap to have 4 elements")

	for {
		item := heap.First()
		if item == nil {
			break
		}
		assert.NotEqual(t, itemRemove.data, item.(*testExpirationItem).data, "This element was not supposed to be in the heap")
	}

	assert.Equal(t, heap.Len(), 0, "The heap is supposed to be with 0 items")
}

func TestExpirationHeapUpdate(t *testing.T) {
	heap := NewExpirationHeap()
	item := newTestItem("data", 1*time.Second)
	heap.Add(item)
	assert.Equal(t, heap.Len(), 1, "The heap is supposed to be with 1 item")

	item.data = "newData"
	heap.Update(item)
	newItem := heap.First()
	assert.Equal(t, newItem.(*testExpirationItem).data, "newData", "The item data didn't change")
	assert.Equal(t, heap.Len(), 0, "The heap is supposed to be with 0 items")
}

func TestNotifyExpirationHeapChannel(t *testing.T) {
	NumEntriesInsert := 10
	heap := NewExpirationHeap()
	assert.True(t, heap.NextExpiration().IsZero(), "Next Expiration time should be 0 for an empty heap")
	var wg sync.WaitGroup
	var lock sync.RWMutex
	wg.Add(1)
	go func() {
		defer wg.Done()
		lock.RLock()
		currenExpiration := heap.NextExpiration()
		if currenExpiration.IsZero() {
			currenExpiration = time.Now().Add(5 * time.Hour)
		}
		lock.RUnlock()

		for {
			<-heap.NotifyCh
			lock.RLock()
			newExpiration := heap.NextExpiration()
			len := heap.Len()
			lock.RUnlock()
			assert.True(t, newExpiration.Before(currenExpiration))
			currenExpiration = newExpiration
			if len == NumEntriesInsert {
				return
			}
		}
	}()
	ttl := 1000
	for i := 0; i < NumEntriesInsert; i++ {
		lock.Lock()
		heap.Add(newTestItem(fmt.Sprintf("data_%d", i), time.Second*time.Duration(ttl-(i*10))))
		lock.Unlock()
		time.Sleep(time.Millisecond)
	}
	wg.Wait()
}
