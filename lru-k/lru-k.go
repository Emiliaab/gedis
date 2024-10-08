package lru_k

import (
	"container/list"
	"encoding/json"
	"fmt"
	"github.com/spaolacci/murmur3"
)

type Hash func(data []byte) uint32

type Cache interface {
	Get(k string) (v gValue, ok bool)
	Set(k string, v gValue)
	Len() int
	Remove(k string) (ok bool)
	RemoveOldest()
	Clear()
	BytesUsed() int64
	GetData() map[string]*list.Element
	SetData(map[string]*list.Element)
	GetRangeData(start, end int) ([]byte, error)
	GetAll() map[string]gValue
}

type cache struct {
	maxBytes int64 // 最大允许的字节大小
	nbytes   int64 // 当前缓存使用的字节大小

	k int // 使用超过k次就移入缓存列表

	inactiveList *list.List
	inactiveMap  map[string]*list.Element

	activeList *list.List
	activeMap  map[string]*list.Element

	onEliminate func(k string, v any)
}

type gValue interface {
	Len() int
	GetBytes() []byte
}

type Entry struct {
	k   string
	v   gValue
	cnt int
}

func NewCache(k int, maxBytes int64, opts ...Option) Cache {
	if k < 2 {
		panic("[cb-cache]: k must be at least 2")
	}

	c := &cache{
		k:            k,
		maxBytes:     maxBytes,
		inactiveList: list.New(),
		inactiveMap:  make(map[string]*list.Element),
		activeList:   list.New(),
		activeMap:    make(map[string]*list.Element),
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

func (c *cache) fill() {
	c.inactiveList = list.New()
	c.inactiveMap = make(map[string]*list.Element)

	c.activeList = list.New()
	c.activeMap = make(map[string]*list.Element)
}

func (c *cache) Get(k string) (v gValue, ok bool) {
	if c.isNil() {
		return
	}

	if e, ok_ := c.activeMap[k]; ok_ {
		c.activeList.MoveToFront(e)
		v, ok = e.Value.(*Entry).v, true
		return
	}

	if e, ok_ := c.inactiveMap[k]; ok_ {
		entry := e.Value.(*Entry)
		entry.cnt++
		if entry.cnt >= c.k {
			c.moveToRealCache(entry, e)
		} else {
			c.inactiveList.MoveToFront(e)
		}
		v, ok = entry.v, true
		return
	}

	v, ok = nil, false
	return
}

func (c *cache) moveToRealCache(entry_ *Entry, e *list.Element) {
	c.activeList.PushFront(entry_)
	c.activeMap[entry_.k] = e
	fmt.Println(e.Value)

	c.inactiveList.Remove(e)
	delete(c.inactiveMap, entry_.k)
}

func (c *cache) Set(k string, v gValue) {
	if c.isNil() {
		c.fill()
	}

	if e, ok_ := c.inactiveMap[k]; ok_ {
		entry := e.Value.(*Entry)
		c.nbytes -= int64(entry.v.Len()) // 减去旧的字节大小
		entry.v = v
		c.nbytes += int64(v.Len()) // 加上新的字节大小
		entry.cnt++
		if entry.cnt >= c.k {
			c.moveToRealCache(entry, e)
		} else {
			c.inactiveList.MoveToFront(e)
		}
		return
	}

	if e, ok_ := c.activeMap[k]; ok_ {
		oldSize := int64(e.Value.(*Entry).v.Len())
		c.nbytes -= oldSize
		e.Value.(*Entry).v = v
		c.nbytes += int64(v.Len())
		c.activeList.MoveToFront(e)
	} else {
		e := c.inactiveList.PushFront(&Entry{k: k, v: v})
		c.inactiveMap[k] = e
		c.nbytes += int64(v.Len()) + int64(len(k))

	}
	if c.maxBytes != 0 && c.maxBytes < c.nbytes {
		c.RemoveOldest()
	}
}

func (c *cache) Remove(k string) (ok bool) {
	if c.isNil() {
		return
	}

	// 尝试从非活跃列表中移除
	if elem, found := c.inactiveMap[k]; found {
		entry := c.inactiveList.Remove(elem).(*Entry)
		delete(c.inactiveMap, entry.k)
		c.nbytes -= int64(entry.v.Len()) + int64(len(entry.k))
		if c.onEliminate != nil {
			c.onEliminate(entry.k, entry.v)
		}
		return true
	}

	// 尝试从活跃列表中移除
	if elem, found := c.activeMap[k]; found {
		entry := c.activeList.Remove(elem).(*Entry)
		delete(c.activeMap, entry.k)
		c.nbytes -= int64(entry.v.Len()) + int64(len(entry.k))
		if c.onEliminate != nil {
			c.onEliminate(entry.k, entry.v)
		}
		return true
	}

	return false
}

func (c *cache) RemoveOldest() {
	if c.isNil() {
		return
	}

	if c.inactiveList.Len() > 0 {
		e := c.inactiveList.Back()
		if e != nil {
			entry := c.inactiveList.Remove(e).(*Entry)
			delete(c.inactiveMap, entry.k)
			c.nbytes -= int64(entry.v.Len()) + int64(len(e.Value.(*Entry).k))
			if c.onEliminate != nil {
				c.onEliminate(entry.k, entry.v)
			}
		}
		return
	}

	if c.activeList.Len() > 0 {
		e := c.activeList.Back()
		if e != nil {
			entry := c.activeList.Remove(e).(*Entry)
			delete(c.activeMap, entry.k)
			c.nbytes -= int64(entry.v.Len()) + int64(len(e.Value.(*Entry).k))
			if c.onEliminate != nil {
				c.onEliminate(entry.k, entry.v)
			}
		}
		return
	}
}

func (c *cache) Clear() {
	if c.onEliminate != nil {
		for _, e := range c.activeMap {
			kv := e.Value.(*Entry)
			c.onEliminate(kv.k, kv.v)
		}
	}
	c.activeMap = nil
	c.inactiveMap = nil
	c.activeList = nil
	c.inactiveList = nil
	c.nbytes = 0
}

func (c *cache) Len() int {
	if c.isNil() {
		return 0
	}
	return len(c.inactiveMap) + len(c.activeMap)
}

func (c *cache) isNil() bool {
	return c.inactiveMap == nil || c.activeMap == nil
}

func (c *cache) BytesUsed() int64 {
	return c.nbytes
}

func (c *cache) GetData() map[string]*list.Element {
	return c.activeMap
}

func (c *cache) SetData(data map[string]*list.Element) {
	c.activeMap = data
}

func (c *cache) GetRangeData(start, end int) ([]byte, error) {
	if c.isNil() {
		return nil, fmt.Errorf("Cache not initialized")
	}

	var hash Hash = func(key []byte) uint32 {
		return uint32(murmur3.Sum64(key))
	}

	result := make(map[string]string)

	// Helper function to process lists
	processList := func(list *list.List) {
		for e := list.Front(); e != nil; e = e.Next() {
			entry := e.Value.(*Entry)
			keyInt := int(hash([]byte(entry.k)))
			if keyInt >= start && keyInt <= end {
				data := entry.v.(gValue).GetBytes()
				result[entry.k] = string(data)
				c.Remove(entry.k)
			}
		}
	}

	// Process both active and inactive lists
	processList(c.activeList)
	processList(c.inactiveList)

	// Serialize the result into JSON
	jsonData, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("Error marshaling data: %v", err)
	}

	return jsonData, nil
}

func (c *cache) GetAll() map[string]gValue {
	if c.isNil() {
		return nil
	}

	result := make(map[string]gValue)

	// Helper function to process lists
	processList := func(list *list.List) {
		for e := list.Front(); e != nil; e = e.Next() {
			entry := e.Value.(*Entry)
			result[entry.k] = entry.v
		}
	}

	// Process both active and inactive lists
	processList(c.activeList)
	processList(c.inactiveList)

	return result
}
