package ttlcache

// Metrics contains common cache metrics so you can calculate hit and miss rates
type Metrics struct {
	// successful inserts
	Inserted int64
	// retrieval attempts
	Retrievals int64
	// all get calls that were in the cache (excludes loader invocations)
	Hits int64
	// entries not in cache (includes loader invocations)
	Misses int64
	// items removed from the cache due to a full size
	EvictedFull int64
	// items removed from the cache due to expiration
	EvictedExpired int64
	// items removed from the cache due to a close call
	EvictedClosed int64
}
