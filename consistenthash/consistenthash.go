package consistenthash

import (
	"hash/crc32"
	"sort"
	"strconv"
)

type Hash func(data []byte) uint32

type Map struct {
	Hash     Hash           `json:"-"`        // hash函数
	Replicas int            `json:"replicas"` // 虚拟节点倍数
	Keys     []int          `json:"keys"`     // 哈希环
	HashMap  map[int]string `json:"hashMap"`  // 虚拟节点和真实节点的映射表，键是虚拟节点的哈希值，值是真实节点的名称
}

func New(replicas int, fn Hash) *Map {
	m := &Map{
		Replicas: replicas,
		Hash:     fn,
		HashMap:  make(map[int]string),
	}
	if m.Hash == nil {
		m.Hash = crc32.ChecksumIEEE
	}
	return m
}

func (m *Map) Add(keys ...string) {
	for _, key := range keys {
		for i := 0; i < m.Replicas; i++ {
			hash := int(m.Hash([]byte(strconv.Itoa(i) + key))) //虚拟节点的id用i+key表示，就相当于peer2的三个虚拟节点分别为：0peer2、1peer2、2peer2
			m.Keys = append(m.Keys, hash)
			m.HashMap[hash] = key
		}
	}
	sort.Ints(m.Keys)
}

func (m *Map) Get(key string) string {
	if len(m.Keys) == 0 {
		return ""
	}

	hash := int(m.Hash([]byte(key)))
	// 二分查找在len(m.keys)范围内，第一个keys中值大于等于hash的索引，也就是找到顺时针第一个虚拟节点下标
	idx := sort.Search(len(m.Keys), func(i int) bool {
		return m.Keys[i] >= hash
	})
	// 如果 idx == len(m.keys)，说明应选择 m.keys[0]，因为 m.keys 是一个环状结构，所以用取余数的方式来处理这种情况。
	return m.HashMap[m.Keys[idx%len(m.Keys)]]
}

func (m *Map) GetPeers() []string {
	uniqueValues := make(map[string]bool)
	result := make([]string, 0)

	for _, value := range m.HashMap {
		// 如果 value 不存在于 set 中,则添加到 set 和结果 map 中
		if !uniqueValues[value] {
			uniqueValues[value] = true
			result = append(result, value)
		}
	}

	return result
}
