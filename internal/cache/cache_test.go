package cache

import (
	"strings"
	"testing"
	"time"
)

func newTestCache(maxBytes, maxEntryBytes int64, ttl time.Duration, now *time.Time) *Cache[string] {
	return New[string](Config[string]{
		TTL:           ttl,
		MaxBytes:      maxBytes,
		MaxEntryBytes: maxEntryBytes,
		SizeOf:        func(v string) int64 { return int64(len(v)) },
		Now:           func() time.Time { return *now },
	})
}

// 预算不足时按 LRU 淘汰，且总量不超预算
func TestSetEvictsLRU(t *testing.T) {
	now := time.Now()
	c := newTestCache(100, 60, time.Minute, &now)

	if !c.Set("a", strings.Repeat("a", 40)) || !c.Set("b", strings.Repeat("b", 40)) {
		t.Fatal("写入应成功")
	}
	if _, ok := c.Get("a"); !ok { // a 变成最近使用，下一个被淘汰的应是 b
		t.Fatal("a 应命中")
	}
	if !c.Set("c", strings.Repeat("c", 40)) {
		t.Fatal("写入应成功")
	}

	if _, ok := c.Get("b"); ok {
		t.Error("b 应被 LRU 淘汰")
	}
	if _, ok := c.Get("a"); !ok {
		t.Error("a 不应被淘汰")
	}
	if got := c.Bytes(); got > 100 {
		t.Errorf("占用 %d 字节，超过预算 100", got)
	}
	if got := c.Len(); got != 2 {
		t.Errorf("条目数 = %d，期望 2", got)
	}
}

// 超过单条上限的条目直接拒绝，不影响已有条目
func TestSetRejectsOversizeEntry(t *testing.T) {
	now := time.Now()
	c := newTestCache(100, 32, time.Minute, &now)

	if !c.Set("ok", strings.Repeat("x", 32)) {
		t.Fatal("等于上限的条目应写入成功")
	}
	if c.Set("big", strings.Repeat("x", 33)) {
		t.Error("超过单条上限的条目应被拒绝")
	}
	if got := c.Bytes(); got != 32 {
		t.Errorf("占用 = %d，期望 32", got)
	}
}

// 单条大于总预算时拒绝写入（避免一条巨物清空整个池）
func TestSetRejectsEntryLargerThanBudget(t *testing.T) {
	now := time.Now()
	c := newTestCache(16, 0, time.Minute, &now)
	if c.Set("k", strings.Repeat("x", 17)) {
		t.Error("超过总预算的条目应被拒绝")
	}
}

// 过期条目在读取时回收
func TestGetExpired(t *testing.T) {
	now := time.Now()
	c := newTestCache(100, 0, time.Minute, &now)

	c.Set("k", "v")
	if _, ok := c.Get("k"); !ok {
		t.Fatal("未过期时应命中")
	}

	now = now.Add(2 * time.Minute)
	if _, ok := c.Get("k"); ok {
		t.Error("过期后不应命中")
	}
	if c.Len() != 0 {
		t.Errorf("过期条目应被回收，当前条目数 = %d", c.Len())
	}
	if c.Bytes() != 0 {
		t.Errorf("过期条目应释放预算，当前占用 = %d", c.Bytes())
	}
}

// TTL <= 0 表示不过期
func TestNoTTL(t *testing.T) {
	now := time.Now()
	c := newTestCache(100, 0, 0, &now)
	c.Set("k", "v")

	now = now.Add(24 * time.Hour)
	if _, ok := c.Get("k"); !ok {
		t.Error("TTL 为 0 时不应过期")
	}
}

// 覆盖写入按新值重新计费
func TestOverwriteAccounting(t *testing.T) {
	now := time.Now()
	c := newTestCache(1000, 0, time.Minute, &now)

	c.Set("k", strings.Repeat("x", 100))
	c.Set("k", strings.Repeat("x", 10))

	if got := c.Bytes(); got != 10 {
		t.Errorf("覆盖后占用 = %d，期望 10", got)
	}
	if got := c.Len(); got != 1 {
		t.Errorf("条目数 = %d，期望 1", got)
	}
}

func TestDeleteAndPurge(t *testing.T) {
	now := time.Now()
	c := newTestCache(1000, 0, time.Minute, &now)

	c.Set("a", strings.Repeat("x", 10))
	c.Set("b", strings.Repeat("x", 20))
	c.Delete("a")
	if c.Bytes() != 20 || c.Len() != 1 {
		t.Errorf("Delete 后 bytes=%d len=%d，期望 20/1", c.Bytes(), c.Len())
	}

	c.Purge()
	if c.Bytes() != 0 || c.Len() != 0 {
		t.Errorf("Purge 后 bytes=%d len=%d，期望 0/0", c.Bytes(), c.Len())
	}
}
