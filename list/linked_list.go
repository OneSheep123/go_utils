package list

import "go_utils/internal/errs"

var _ List[any] = &LinkedList[any]{}

type node[T any] struct {
	val  T
	prev *node[T]
	next *node[T]
}

type LinkedList[T any] struct {
	head   *node[T]
	tail   *node[T]
	length int
}

// NewLinkedList 创建一个双向循环链表
func NewLinkedList[T any]() *LinkedList[T] {
	head := &node[T]{}
	tail := &node[T]{next: head, prev: head}
	head.next, head.prev = tail, tail
	return &LinkedList[T]{
		head: head,
		tail: tail,
	}
}

// NewLinkedListOf 将切片转换为双向循环链表, 直接使用了切片元素的值，而没有进行复制
func NewLinkedListOf[T any](ts []T) *LinkedList[T] {
	list := NewLinkedList[T]()
	if err := list.Append(ts...); err != nil {
		panic(err)
	}
	return list
}

func (l *LinkedList[T]) checkIndex(index int) bool {
	return index >= 0 && index < l.length
}

func (l *LinkedList[T]) findNode(index int) *node[T] {
	var cur *node[T]
	if index <= l.Len()/2 {
		cur = l.head
		for i := -1; i < index; i++ {
			cur = cur.next
		}
	} else {
		cur = l.tail
		for i := l.Len(); i > index; i-- {
			cur = cur.prev
		}
	}

	return cur
}

func (l *LinkedList[T]) Get(index int) (T, error) {
	if !l.checkIndex(index) {
		var zeroValue T
		return zeroValue, errs.NewErrIndexOutOfRange(l.Len(), index)
	}
	return l.findNode(index).val, nil
}

func (l *LinkedList[T]) Append(ts ...T) error {
	for _, t := range ts {
		n := &node[T]{prev: l.tail.prev, next: l.tail, val: t}
		n.prev.next, n.next.prev = n, n
		l.length++
	}
	return nil
}

func (l *LinkedList[T]) Add(index int, t T) error {
	if index < 0 || index > l.length {
		return errs.NewErrIndexOutOfRange(l.length, index)
	}
	if index == l.length {
		return l.Append(t)
	}
	next := l.findNode(index)
	n := &node[T]{prev: next.prev, next: next, val: t}
	n.prev.next, n.next.prev = n, n
	l.length++
	return nil
}

func (l *LinkedList[T]) Set(index int, t T) error {
	if !l.checkIndex(index) {
		return errs.NewErrIndexOutOfRange(l.Len(), index)
	}
	n := l.findNode(index)
	n.val = t
	return nil
}

func (l *LinkedList[T]) Delete(index int) (T, error) {
	if !l.checkIndex(index) {
		var zeroValue T
		return zeroValue, errs.NewErrIndexOutOfRange(l.Len(), index)
	}
	n := l.findNode(index)
	n.prev.next = n.next
	n.next.prev = n.prev
	n.prev, n.next = nil, nil
	l.length--
	return n.val, nil
}

func (l *LinkedList[T]) Len() int {
	return l.length
}

func (l *LinkedList[T]) Cap() int {
	return l.Len()
}

func (l *LinkedList[T]) Range(fn func(index int, t T) error) error {
	for cur, i := l.head.next, 0; i < l.length; i++ {
		err := fn(i, cur.val)
		if err != nil {
			return err
		}
		cur = cur.next
	}
	return nil
}

func (l *LinkedList[T]) AsSlice() []T {
	slice := make([]T, l.length)
	for cur, i := l.head.next, 0; i < l.length; i++ {
		slice[i] = cur.val
		cur = cur.next
	}
	return slice
}
