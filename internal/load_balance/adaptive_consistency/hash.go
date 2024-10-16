package adaptive_consistency

import (
	"fmt"
	"go_utils/internal/slice"
	"math"
	"sync"
)

const DefaultHashRingSlotNum = 1024

type HashCodeFunc func(req any) int

type HashRing struct {
	nodes            []*Node
	slotOfNodes      []*Node
	requestNumOfSlot []int
	slotNum          int
	nodeNum          int
	lock             sync.RWMutex
	hashCodeFunc     HashCodeFunc
}

func NewHashRing(nodes []*Node, slotNum int, hashCodeFunc HashCodeFunc) *HashRing {
	cnt := len(nodes)
	avg := slotNum / cnt
	ns := make([]*Node, 0, slotNum)
	k := 0
	total := 0
	// cnt = 3
	for i := 0; i < cnt; i++ {
		if i == cnt-1 {
			total = slotNum
		} else {
			total += avg
		}
		for ; k <= total; k++ {
			ns = append(ns, nodes[i])
		}

	}
	return &HashRing{
		nodes:            nodes,
		slotOfNodes:      ns,
		requestNumOfSlot: make([]int, slotNum),
		slotNum:          slotNum,
		nodeNum:          len(nodes),
		hashCodeFunc:     hashCodeFunc,
	}
}

func (h *HashRing) GetNode(uid int) *Node {
	sKey := h.hashCodeFunc(uid)
	h.countSlotRequest(sKey)
	return h.slotOfNodes[sKey]
}

func (h *HashRing) countSlotRequest(sKey int) {
	h.requestNumOfSlot[sKey]++
}

func (h *HashRing) Balance() {
	h.lock.Lock()

	// 计算前缀和，方便快速计算子数组的和
	prefixSum := h.prefixSums()

	totalRequest := slice.Sum[int](h.requestNumOfSlot)
	avgRequest := totalRequest / h.nodeNum

	//fmt.Printf("总请求数为：%d，平均请求数为：%d\n", totalRequest, avgRequest)

	// dp[i][j] 表示前 i 个元素分成 j 段，最小的偏差值
	dp := make([][]int, h.slotNum+1)   // 用于存储动态规划过程中的最小偏差值
	cuts := make([][]int, h.slotNum+1) // 用于记录到达最小偏差的前一个状态
	for i := range dp {
		dp[i] = make([]int, h.nodeNum+1)
		cuts[i] = make([]int, h.nodeNum+1)
		for j := range dp[i] {
			if j == 0 {
				dp[i][j] = 0 // 初始状态：无切分，偏差为0
			} else {
				dp[i][j] = math.MaxInt64 // 其他状态初始化为最大值，表示尚未找到更优解
			}
		}
	}

	// 动态规划过程，寻找最佳分割点
	for j := 2; j <= h.nodeNum; j++ { // 从2个分割点开始，直到分割为nodeNum个节点
		for i := j; i <= h.slotNum; i++ { // 从第j个位置开始考虑分割
			for k := j - 1; k < i; k++ { // 从j-1到i-1中寻找最佳的前一个分割点
				// 计算从k+1到i的区间和，即当前区间的请求量
				sum := prefixSum[i] - prefixSum[k]
				// 计算平均请求量与当前区间请求量的偏差值
				diff := int(math.Abs(float64(avgRequest - sum)))
				// 如果通过前一个状态k到达当前状态i的偏差更小，则更新dp和cuts
				if dp[k][j-1]+diff < dp[i][j] {
					dp[i][j] = dp[k][j-1] + diff
					cuts[i][j] = k
				}
			}
		}
	}

	// 回溯找到切割点
	bestSplitKey := make([]int, h.nodeNum)
	bestSplit := make([][]int, h.nodeNum)
	idx := h.slotNum
	for j := h.nodeNum; j > 0; j-- {
		prevIdx := cuts[idx][j]
		bestSplitKey[j-1] = idx
		bestSplit[j-1] = h.requestNumOfSlot[prevIdx:idx]
		//fmt.Printf("回溯切割点: %d, 子数组: %v\n", prevIdx, bestSplit[j-1]) // 打印切割点
		idx = prevIdx
	}

	// 打印最终结果
	for i, subarray := range bestSplit {
		fmt.Printf("子数组 %d: %v, 和: %d\n", i+1, subarray, slice.Sum[int](subarray))
	}
	//fmt.Printf("最小偏差总和: %d\n", dp[h.slotNum][h.nodeNum])

	// 初始化hash槽
	ns := make([]*Node, 0, h.slotNum)
	key := 0
	for k, v := range h.nodes {
		for ; key < bestSplitKey[k]; key++ {
			ns = append(ns, v)
		}
	}

	h.slotOfNodes = ns
	h.requestNumOfSlot = make([]int, h.slotNum)
	h.lock.Unlock()
}

// 计算数组的部分和
func (h *HashRing) prefixSums() []int {
	prefixSum := make([]int, h.slotNum+1)
	for i := 1; i <= h.slotNum; i++ {
		prefixSum[i] = prefixSum[i-1] + h.requestNumOfSlot[i-1]
	}
	return prefixSum
}

// 方便测试
func (h *HashRing) SetRequestNumOfSlot(requestNums []int) {
	h.requestNumOfSlot = requestNums
}
