package cache

import (
	"context"
	"go_utils/internal/errs"
	"sync"
	"time"
)

var _ Cache = &LRUCache{}

type node struct {
	// 这里多一个key用于删除使用
	key  string
	val  any
	pre  *node
	next *node
}

type LRUCache struct {
	mu   sync.RWMutex
	m    map[string]*node
	head *node
	tail *node
	cap  int
}

func (lru *LRUCache) Get(ctx context.Context, key string) (any, error) {
	lru.mu.RLock()
	if len(lru.m) == 0 {
		lru.mu.RUnlock()
		return nil, errs.ErrKeyNotFound
	}
	n, ok := lru.m[key]
	lru.mu.RUnlock()
	if !ok {
		return nil, errs.ErrKeyNotFound
	}
	lru.mu.Lock()
	defer lru.mu.Unlock()
	val := n.val
	n1 := lru.m[key]
	// 双重校验(这里有可能当前节点已经被其他goroutine移除)
	if n1 != n {
		return n.val, nil
	}

	lru.removeFromList(n)
	// 这里面会新建一个节点
	lru.insertToListHead(key, val)

	return n.val, nil
}

func (lru *LRUCache) Set(ctx context.Context, key string, value any, expireTime time.Duration) error {
	lru.mu.RLock()
	oldNode := lru.m[key]
	lru.mu.RUnlock()

	lru.mu.Lock()
	defer lru.mu.Unlock()
	tempNode := lru.m[key]

	if oldNode != tempNode {
		// 说明对应key已经是被操作的
		return nil
	}

	if oldNode != nil {
		oldNode.val = value
		lru.removeFromList(oldNode)
		lru.insertToListHead(key, value)
		return nil
	}
	newNode := lru.insertToListHead(key, value)
	lru.m[key] = newNode
	// 将当前元素移到头部
	if len(lru.m) > lru.cap {
		// 需要将最少使用的元素进行移除
		lru.removeFromList(lru.tail.pre)
	}
	return nil
}

func (lru *LRUCache) Delete(ctx context.Context, key string) error {
	lru.delete(key)
	return nil
}

func (lru *LRUCache) delete(key string) any {
	lru.mu.RLock()
	n, ok := lru.m[key]
	// 说明当前节点已经被移除
	if !ok {
		return nil
	}
	lru.mu.RUnlock()
	lru.mu.Lock()
	// 双重锁校验()
	_, ok = lru.m[key]
	if !ok {
		return n.val
	}
	delete(lru.m, key)
	n.next.pre = n.pre
	n.pre.next = n.next
	lru.mu.Unlock()
	return n.val
}

func (lru *LRUCache) LoadAndDelete(ctx context.Context, key string) (any, error) {
	return lru.delete(key), nil
}

func NewBuildLRUCache(capacity int) *LRUCache {
	lru := LRUCache{
		m:   make(map[string]*node),
		cap: capacity,
	}
	lru.head = &node{}
	lru.tail = &node{}
	lru.head.next = lru.tail
	lru.tail.pre = lru.head
	return &lru
}

func (lru *LRUCache) removeFromList(node *node) {
	// 1. 从当前链表中移除
	pre := node.pre
	next := node.next
	if pre != nil {
		pre.next = next
	}
	if next != nil {
		next.pre = pre
	}
	// 2. 从当前map中移除节点
	delete(lru.m, node.key)
}

func (lru *LRUCache) insertToListHead(key string, value any) *node {
	n := &node{key: key, val: value}

	// 获取head节点的下一个节点
	head := lru.head

	n.next = head.next
	head.next.pre = n

	n.pre = head
	head.next = n

	lru.m[key] = n
	return n
}
