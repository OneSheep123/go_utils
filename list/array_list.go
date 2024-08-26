package list

import (
	"go_utils/internal/errs"
	"go_utils/internal/slice"
)

var _ List[any] = &ArrayList[any]{}

type ArrayList[T any] struct {
	data []T
}

// NewArrayList 初始化一个len为0，cap为cap的ArrayList
func NewArrayList[T any](cap int) *ArrayList[T] {
	return &ArrayList[T]{data: make([]T, 0, cap)}
}

// NewArrayListOf 直接使用 ts，而不会执行复制
func NewArrayListOf[T any](ts []T) *ArrayList[T] {
	return &ArrayList[T]{
		data: ts,
	}
}

func (a *ArrayList[T]) Get(index int) (t T, err error) {
	l := a.Len()
	if index < 0 || index >= l {
		return t, errs.NewErrIndexOutOfRange(l, index)
	}
	return a.data[index], nil
}

func (a *ArrayList[T]) Append(ts ...T) (err error) {
	a.data = append(a.data, ts...)
	return
}

func (a *ArrayList[T]) Add(index int, t T) (err error) {
	a.data, err = slice.Add[T](a.data, t, index)
	return
}

func (a *ArrayList[T]) Set(index int, t T) (err error) {
	length := len(a.data)
	if index >= length || index < 0 {
		return errs.NewErrIndexOutOfRange(length, index)
	}
	a.data[index] = t
	return
}

func (a *ArrayList[T]) Delete(index int) (t T, err error) {
	res, t, err := slice.Delete[T](a.data, index)
	if err != nil {
		return
	}
	a.data = res
	// todo: 后续可以考虑增加缩容
	return
}

func (a *ArrayList[T]) Len() int {
	return len(a.data)
}

func (a *ArrayList[T]) Cap() int {
	return len(a.data)
}

func (a *ArrayList[T]) Range(fn func(index int, t T) error) error {
	for key, value := range a.data {
		e := fn(key, value)
		if e != nil {
			return e
		}
	}
	return nil
}

func (a *ArrayList[T]) AsSlice() []T {
	res := make([]T, len(a.data))
	copy(res, a.data)
	return res
}
