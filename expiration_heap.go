package ttl

import (
	"container/heap"
	"time"
)

//An ExpirationHeap is basically a heap priority queue implementation
//but using a expiration time as the priority, this means that
//the entries in the queue are ordered using the time when they
//are going to expire.
//A NotifyCh es used to know when the first element in the queue is
//updated.

//This is a PriorityQueue implementation based on the example code
//from here https://golang.org/pkg/container/heap/
//Main difference is that it uses a time.Time to order the items
//in the queue, so that we will always have the items than are closer
//to expire in the top of the queue
//Any kind of structs can be used as the items to be stored as long
//as they implement the priorityItem interface, this is having a ValidUntil()
//method to get the time when the item will expire and also methods to
//set and get the item index in the queue

//ExpirationHeapEntry is the interface that any entry we want to insert
//in the heap should meet. The ExpirationHeap will need to set and get
//the index used and to know when the entry will expire.
//Since we are not keeping the index ourself you must be sure than
//you dont modify the index value in your own entry implementation!!
//Use this with caution. We are relying in the Set/Get Index methods
//to keep the objects in the underlying heap. Do not change those ones
//in your entry object.
type ExpirationHeapEntry interface {
	ExpiresAt() time.Time
	GetIndex() int
	SetIndex(int)
}

//ExpirationHeap is the struct used in container/heap
type ExpirationHeap struct {
	entries []ExpirationHeapEntry
	//A channel used to notify when the first element (index=0)
	//in the heap has been modified
	NotifyCh chan struct{}
}

//EntryNotIndexed is the index value assigned to an entry that was removed from the heap
const EntryNotIndexed = -1

//NewExpirationHeap creates a new ExpirationHeap
func NewExpirationHeap() *ExpirationHeap {
	h := &ExpirationHeap{
		NotifyCh: make(chan struct{}),
	}
	heap.Init(h)
	return h
}

func (h *ExpirationHeap) notify() {
	select {
	case h.NotifyCh <- struct{}{}:
	default:
	}
}

//Len meets the container/heap interface
//and returns the number of items in the queue
func (h ExpirationHeap) Len() int {
	return len(h.entries)
}

//Less meets the container/heap interface
//and compare to item within the queue
//Dont call this directly!
func (h ExpirationHeap) Less(i, j int) bool {
	if h.entries[i].ExpiresAt().IsZero() {
		return false
	}
	if h.entries[j].ExpiresAt().IsZero() {
		return true
	}
	return h.entries[i].ExpiresAt().Before(h.entries[j].ExpiresAt())
}

//Swap meets the container/heap interface
//and change the position of 2 elements in the queue
//Dont call this directly!
func (h ExpirationHeap) Swap(i, j int) {
	var firstEntry ExpirationHeapEntry
	isFirstEntry := h.Len() == 0
	if !isFirstEntry {
		firstEntry = h.entries[0]
	}
	h.entries[i], h.entries[j] = h.entries[j], h.entries[i]
	h.entries[i].SetIndex(i)
	h.entries[j].SetIndex(j)
	if isFirstEntry || firstEntry.GetIndex() != 0 {
		h.notify()
	}
}

//Push meets the container/heap interface
//and inserts an item in the heap
//Dont call this directly!
func (h *ExpirationHeap) Push(x interface{}) {
	l := len(h.entries)
	entry := x.(ExpirationHeapEntry)
	entry.SetIndex(l)
	h.entries = append(h.entries, entry)
	if entry.GetIndex() == 0 {
		h.notify()
	}
}

//Pop meets the container/heap interface
//and removes the first item in the heap
//Don not call this directly!
func (h *ExpirationHeap) Pop() interface{} {
	heapEntries := h.entries
	l := len(heapEntries)
	entry := heapEntries[l-1]
	heapEntries[l-1] = nil
	entry.SetIndex(EntryNotIndexed)
	h.entries = heapEntries[0 : l-1]
	return entry
}

//Update uptade an entry in the heap
func (h *ExpirationHeap) Update(entry ExpirationHeapEntry) {
	heap.Fix(h, entry.GetIndex())
}

//Add a new entry in the heap
func (h *ExpirationHeap) Add(entry ExpirationHeapEntry) {
	heap.Push(h, entry)
}

//First get the first element in the heap (ie: the one with the lowest ttl)
func (h *ExpirationHeap) First() ExpirationHeapEntry {
	if h.Len() == 0 {
		return nil
	}
	return heap.Pop(h).(ExpirationHeapEntry)
}

//Peek get the first element in the heap without removing it
func (h *ExpirationHeap) Peek() ExpirationHeapEntry {
	if h.Len() == 0 {
		return nil
	}
	return h.entries[0]
}

//Remove removes an entry from the heap. Note that this just try to remove
//the entry acording to the index it has. It wont check if the object is really
//the same we are sending.
func (h *ExpirationHeap) Remove(entry ExpirationHeapEntry) {
	if entry.GetIndex() != EntryNotIndexed {
		heap.Remove(h, entry.GetIndex())
	}
}

//NextExpiration gets the lower ttl in the heap. The ttl from the element with index 0
func (h *ExpirationHeap) NextExpiration() time.Time {
	if h.Len() == 0 {
		return time.Time{}
	}
	entry := h.entries[0]
	return entry.ExpiresAt()
}
