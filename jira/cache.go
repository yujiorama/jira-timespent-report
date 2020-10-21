package jira

import "sync"

type Cache struct {
	mutex *sync.Mutex
	memo  map[string]interface{}
}

var (
	cache = &Cache{
		mutex: &sync.Mutex{},
		memo:  map[string]interface{}{},
	}
)

func (c *Cache) put(k string, v interface{}) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.memo[k] = v
}

func (c *Cache) get(k string) (interface{}, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	v, ok := c.memo[k]
	return v, ok
}
