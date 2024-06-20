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

// 用于数据迁移，对应一个hash区间和该区间的数据来源节点名称
type RangeNode struct {
	Start    int    `json:"start"`
	End      int    `json:"end"`
	RealNode string `json:"realNode"`
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

// 根据新加的节点的名称，获取新加的节点所有虚拟节点hash值和哈希环里前一个节点hash值的范围区间合并后的集合
func (m *Map) GetRange(key string, oldKeys []int) []RangeNode {
	var ranges []RangeNode

	// 检索与提供的key相关的所有虚拟节点的哈希值
	keyHashes := make([]int, 0)
	for i := 0; i < m.Replicas; i++ {
		virtualNodeKey := strconv.Itoa(i) + key
		hash := int(m.Hash([]byte(virtualNodeKey)))
		keyHashes = append(keyHashes, hash)
	}

	// 排序这些哈希值以保证正确的顺序处理
	sort.Ints(keyHashes)

	// 对于每个虚拟节点，找到它负责的数据范围
	for _, hash := range keyHashes {
		// 找到当前哈希值的位置
		idx := sort.SearchInts(m.Keys, hash)

		// 确定该虚拟节点负责的数据范围的起始和结束
		// 这里需要确定环形列表中的上一个节点
		prevIdx := (idx - 1 + len(m.Keys)) % len(m.Keys)
		start := m.Keys[prevIdx] + 1
		end := hash // 包括当前节点

		// 添加到结果中
		ranges = append(ranges, RangeNode{Start: start, End: end})
	}
	t1 := mergeRanges(ranges)

	// 对t1中的每一个区间，处理每个区间的结束节点信息
	var t2 []RangeNode
	for _, rangeNode := range t1 {
		end := rangeNode.End
		nextIdx := sort.SearchInts(oldKeys, end)
		if nextIdx == len(oldKeys) {
			nextIdx = 0
		}
		realNode := m.HashMap[oldKeys[nextIdx]]

		// 将区间起始、结束和对应真实节点加入最终结果
		t2 = append(t2, RangeNode{Start: rangeNode.Start, End: rangeNode.End, RealNode: realNode})
	}

	return t2
}

// 辅助函数，用于合并区间
func mergeRanges(ranges []RangeNode) []RangeNode {
	if len(ranges) == 0 {
		return ranges
	}

	// 按起始位置排序
	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].Start < ranges[j].Start
	})

	merged := []RangeNode{}
	// 初始区间
	current := ranges[0]
	for _, rangeNode := range ranges[1:] {
		if rangeNode.Start <= current.End {
			// 如果当前区间可以合并，更新结束位置
			if rangeNode.End > current.End {
				current.End = rangeNode.End
			}
		} else {
			// 如果不能合并，先将当前区间添加到结果中，然后更新当前区间
			merged = append(merged, current)
			current = rangeNode
		}
	}
	// 添加最后一个区间
	merged = append(merged, current)

	return merged
}
