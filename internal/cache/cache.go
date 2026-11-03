package cache

import (
	"container/list"
	"sync"
	"time"
)

// Config 有界缓存的运行参数
type Config[V any] struct {
	// TTL 条目存活时间，<= 0 表示永不过期
	TTL time.Duration
	// MaxBytes 所有条目占用字节总和的上限，<= 0 表示不限制
	MaxBytes int64
	// MaxEntryBytes 单条上限，超过上限的条目直接拒绝写入（避免一条巨物挤掉整个池）
	MaxEntryBytes int64
	// SizeOf 计算单个条目占用字节，用于预算统计
	SizeOf func(V) int64
	// Now 时钟，仅测试注入用
	Now func() time.Time
}

type cacheItem[V any] struct {
	key    string
	value  V
	size   int64
	expire time.Time
}

// Cache 有界 TTL + LRU 缓存
//
// 设计要点（相对 bigcache）：
//   - 内存由 MaxBytes 精确控制，不预分配、不保留 per-shard 复用缓冲
//   - 淘汰按条目真实字节数进行，存得下就存、存不下就淘汰最久未使用
//   - 过期条目在访问或预算不足时回收，不额外起后台 goroutine
//
// 并发安全；Get 返回的值必须按只读方式使用（多个请求可能共享同一份）
type Cache[V any] struct {
	mu    sync.Mutex
	cfg   Config[V]
	ll    *list.List // 头部为最近使用
	items map[string]*list.Element
	bytes int64
}

// New 创建缓存实例
func New[V any](cfg Config[V]) *Cache[V] {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.SizeOf == nil {
		cfg.SizeOf = func(V) int64 { return 0 }
	}
	return &Cache[V]{
		cfg:   cfg,
		ll:    list.New(),
		items: make(map[string]*list.Element),
	}
}

// Get 读取条目，命中时刷新 LRU 位置
func (c *Cache[V]) Get(key string) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}

	it := el.Value.(*cacheItem[V])
	if c.expired(it) {
		c.removeLocked(el)
		var zero V
		return zero, false
	}

	c.ll.MoveToFront(el)
	return it.value, true
}

// Set 写入条目，返回是否被缓存（超过单条上限时返回 false）
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

// Delete 删除条目
func (c *Cache[V]) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.removeLocked(el)
	}
}

// Purge 清空缓存并释放内存
func (c *Cache[V]) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ll.Init()
	c.items = make(map[string]*list.Element)
	c.bytes = 0
}

// Len 当前条目数
func (c *Cache[V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

// Bytes 当前占用字节数
func (c *Cache[V]) Bytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bytes
}

// evictLocked 按 LRU 淘汰直到能容纳 need 字节
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
