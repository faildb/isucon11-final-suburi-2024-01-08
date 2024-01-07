package gotemplates

import (
	"sync"
	"time"
)

type expiredValue[V any] struct {
	value  V
	expire time.Time
}

type cacheExpired[K comparable, V any] struct {
	sync.RWMutex
	items map[K]expiredValue[V]
}

func NewCacheExpired[K comparable, V any]() *cacheExpired[K, V] {
	c := &cacheExpired[K, V]{
		items: make(map[K]expiredValue[V]),
	}
	return c
}

func (c *cacheExpired[K, V]) Set(key K, value V, expire time.Time) {
	val := expiredValue[V]{
		value:  value,
		expire: expire,
	}
	c.Lock()
	defer c.Unlock()
	c.items[key] = val
}

func (c *cacheExpired[K, V]) Get(key K) (V, bool) {
	c.RLock()
	defer c.RUnlock()
	v, found := c.items[key]
	if !found {
		var zero V
		return zero, false
	}
	if time.Now().After(v.expire) {
		var zero V
		return zero, false
	}
	return v.value, found
}

func (c *cacheExpired[K, V]) GetAll() map[K]V {
	c.RLock()
	vs := c.items
	c.RUnlock()

	ret := make(map[K]V, len(vs))
	for k, v := range vs {
		if time.Now().After(v.expire) {
			continue
		}
		ret[k] = v.value
	}
	return ret
}

func (c *cacheExpired[K, V]) Del(key K) {
	c.Lock()
	delete(c.items, key)
	c.Unlock()
}

func (c *cacheExpired[K, V]) Keys() []K {
	c.RLock()
	res := make([]K, len(c.items))
	i := 0
	for k, _ := range c.items {
		res[i] = k
		i++
	}
	c.RUnlock()
	return res
}
