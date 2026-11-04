package cache

import (
	"container/list"
	"runtime/debug"
	"sync"
	"time"
)

type Config[V any] struct {
	TTL           time.Duration
	MaxBytes      int64
	MaxEntryBytes int64
	SizeOf        func(V) int64
	Now           func() time.Time
	SweepInterval time.Duration
	ReleaseOS     bool
}

const (
	minReleaseBytes = 8 << 20
	maxReleaseBytes = 32 << 20
)

type cacheItem[V any] struct {
	key    string
	value  V
	size   int64
	expire time.Time
}

type Cache[V any] struct {
	mu     sync.Mutex
	cfg    Config[V]
	ll     *list.List
	items  map[string]*list.Element
	bytes  int64
	hits   int64
	misses int64

	stop     chan struct{}
	stopOnce sync.Once
}

type Stats struct {
	Hits    int64
	Misses  int64
	Entries int
	Bytes   int64
}

func New[V any](cfg Config[V]) *Cache[V] {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.SizeOf == nil {
		cfg.SizeOf = func(V) int64 { return 0 }
	}
	c := &Cache[V]{
		cfg:   cfg,
		ll:    list.New(),
		items: make(map[string]*list.Element),
	}
	if cfg.SweepInterval > 0 && cfg.TTL > 0 {
		c.stop = make(chan struct{})
		go c.janitor(cfg.SweepInterval)
	}
	return c
}

func (c *Cache[V]) janitor(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			freed := c.PurgeExpired()
			if freed > 0 && c.cfg.ReleaseOS && freed >= c.releaseThreshold() {
				debug.FreeOSMemory()
			}
		case <-c.stop:
			return
		}
	}
}

func (c *Cache[V]) releaseThreshold() int64 {
	threshold := c.cfg.MaxBytes / 8
	if threshold < minReleaseBytes {
		return minReleaseBytes
	}
	if threshold > maxReleaseBytes {
		return maxReleaseBytes
	}
	return threshold
}

func (c *Cache[V]) Close() {
	c.stopOnce.Do(func() {
		if c.stop != nil {
			close(c.stop)
		}
	})
}

func (c *Cache[V]) Get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		c.misses++
		var zero V
		return zero, false
	}

	it := el.Value.(*cacheItem[V])
	if c.expired(it) {
		c.removeLocked(el)
		c.misses++
		var zero V
		return zero, false
	}

	c.hits++
	c.ll.MoveToFront(el)
	return it.value, true
}

func (c *Cache[V]) Set(key string, value V) bool {
	size := c.cfg.SizeOf(value)
	if size < 0 {
		return false
	}
	if c.cfg.MaxEntryBytes > 0 && size > c.cfg.MaxEntryBytes {
		return false
	}
	if c.cfg.MaxBytes > 0 && size > c.cfg.MaxBytes {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		c.removeLocked(el)
	}
	c.evictLocked(size)

	it := &cacheItem[V]{
		key:    key,
		value:  value,
		size:   size,
		expire: c.expireAt(),
	}
	c.items[key] = c.ll.PushFront(it)
	c.bytes += size
	return true
}

func (c *Cache[V]) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.removeLocked(el)
	}
}

func (c *Cache[V]) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ll.Init()
	c.items = make(map[string]*list.Element)
	c.bytes = 0
}

func (c *Cache[V]) PurgeExpired() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	var freed int64
	for el := c.ll.Back(); el != nil; {
		prev := el.Prev()
		if it := el.Value.(*cacheItem[V]); c.expired(it) {
			freed += it.size
			c.removeLocked(el)
		}
		el = prev
	}
	return freed
}

func (c *Cache[V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

func (c *Cache[V]) Bytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bytes
}

func (c *Cache[V]) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Stats{
		Hits:    c.hits,
		Misses:  c.misses,
		Entries: len(c.items),
		Bytes:   c.bytes,
	}
}

func (c *Cache[V]) evictLocked(need int64) {
	if c.cfg.MaxBytes <= 0 {
		return
	}
	for c.bytes+need > c.cfg.MaxBytes {
		el := c.ll.Back()
		if el == nil {
			return
		}
		c.removeLocked(el)
	}
}

func (c *Cache[V]) removeLocked(el *list.Element) {
	it := el.Value.(*cacheItem[V])
	c.ll.Remove(el)
	delete(c.items, it.key)
	c.bytes -= it.size
	if c.bytes < 0 {
		c.bytes = 0
	}
}

func (c *Cache[V]) expired(it *cacheItem[V]) bool {
	return !it.expire.IsZero() && c.cfg.Now().After(it.expire)
}

func (c *Cache[V]) expireAt() time.Time {
	if c.cfg.TTL <= 0 {
		return time.Time{}
	}
	return c.cfg.Now().Add(c.cfg.TTL)
}
