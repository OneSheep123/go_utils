package list

import (
	"errors"
	"fmt"
	"github.com/stretchr/testify/assert"
	"go_utils/internal/errs"
	"testing"
)

func TestArrayList_Add(t *testing.T) {
	testCases := []struct {
		name      string
		list      *ArrayList[int]
		index     int
		newVal    int
		wantSlice []int
		wantErr   error
	}{
		{
			name:      "add num to index left",
			list:      NewArrayListOf[int]([]int{1, 2, 3}),
			newVal:    100,
			index:     0,
			wantSlice: []int{100, 1, 2, 3},
		},
		{
			name:      "add num to index right",
			list:      NewArrayListOf[int]([]int{1, 2, 3}),
			newVal:    100,
			index:     3,
			wantSlice: []int{1, 2, 3, 100},
		},
		{
			name:      "add num to index mid",
			list:      NewArrayListOf[int]([]int{1, 2, 3}),
			newVal:    100,
			index:     1,
			wantSlice: []int{1, 100, 2, 3},
		},
		{
			name:    "add num to index -1",
			list:    NewArrayListOf[int]([]int{1, 2, 3}),
			newVal:  100,
			index:   -1,
			wantErr: fmt.Errorf("go_utils: 下标超出范围，长度 %d, 下标 %d", 3, -1),
		},
		{
			name:    "add num to index OutOfRange",
			list:    NewArrayListOf[int]([]int{1, 2, 3}),
			newVal:  100,
			index:   4,
			wantErr: fmt.Errorf("go_utils: 下标超出范围，长度 %d, 下标 %d", 3, 4),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.list.Add(tc.index, tc.newVal)
			assert.Equal(t, tc.wantErr, err)
			// 因为返回了 error，所以我们不用继续往下比较了
			if err != nil {
				return
			}
			assert.Equal(t, tc.wantSlice, tc.list.data)
		})
	}
}

func TestArrayList_Cap(t *testing.T) {
	testCases := []struct {
		name      string
		expectCap int
		list      *ArrayList[int]
	}{
		{
			name:      "与实际容量相等",
			expectCap: 5,
			list: &ArrayList[int]{
				data: make([]int, 5),
			},
		},
		{
			name:      "用户传入nil",
			expectCap: 0,
			list: &ArrayList[int]{
				data: nil,
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actual := testCase.list.Cap()
			assert.Equal(t, testCase.expectCap, actual)
		})
	}
}

func TestArrayList_Append(t *testing.T) {
	testCases := []struct {
		name      string
		list      *ArrayList[int]
		newVal    []int
		wantSlice []int
	}{
		{
			name:      "append non-empty values to non-empty list",
			list:      NewArrayListOf[int]([]int{123}),
			newVal:    []int{234, 456},
			wantSlice: []int{123, 234, 456},
		},
		{
			name:      "append empty values to non-empty list",
			list:      NewArrayListOf[int]([]int{123}),
			newVal:    []int{},
			wantSlice: []int{123},
		},
		{
			name:      "append nil to non-empty list",
			list:      NewArrayListOf[int]([]int{123}),
			newVal:    nil,
			wantSlice: []int{123},
		},
		{
			name:      "append non-empty values to empty list",
			list:      NewArrayListOf[int]([]int{}),
			newVal:    []int{234, 456},
			wantSlice: []int{234, 456},
		},
		{
			name:      "append empty values to empty list",
			list:      NewArrayListOf[int]([]int{}),
			newVal:    []int{},
			wantSlice: []int{},
		},
		{
			name:      "append nil to empty list",
			list:      NewArrayListOf[int]([]int{}),
			newVal:    nil,
			wantSlice: []int{},
		},
		{
			name:      "append non-empty values to nil list",
			list:      NewArrayListOf[int](nil),
			newVal:    []int{234, 456},
			wantSlice: []int{234, 456},
		},
		{
			name:      "append empty values to nil list",
			list:      NewArrayListOf[int](nil),
			newVal:    []int{},
			wantSlice: []int{},
		},
		{
			name:      "append nil to nil list",
			list:      NewArrayListOf[int](nil),
			newVal:    nil,
			wantSlice: []int{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.list.Append(tc.newVal...)
			if err != nil {
				return
			}

			assert.Equal(t, tc.wantSlice, tc.list.AsSlice())
		})
	}
}

func TestArrayList_Delete(t *testing.T) {
	testCases := []struct {
		name      string
		list      *ArrayList[int]
		index     int
		wantSlice []int
		wantVal   int
		wantErr   error
	}{
		{
			name: "deleted",
			list: &ArrayList[int]{
				data: []int{123, 124, 125},
			},
			index:     1,
			wantSlice: []int{123, 125},
			wantVal:   124,
		},
		{
			name: "index out of range",
			list: &ArrayList[int]{
				data: []int{123, 100},
			},
			index:   12,
			wantErr: errs.NewErrIndexOutOfRange(2, 12),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			val, err := tc.list.Delete(tc.index)
			assert.Equal(t, tc.wantErr, err)
			// 因为返回了 error，所以我们不用继续往下比较了
			if err != nil {
				return
			}
			assert.Equal(t, tc.wantSlice, tc.list.data)
			assert.Equal(t, tc.wantVal, val)
		})
	}
}

func TestArrayList_Len(t *testing.T) {
	testCases := []struct {
		name      string
		expectLen int
		list      *ArrayList[int]
	}{
		{
			name:      "与实际元素数相等",
			expectLen: 5,
			list: &ArrayList[int]{
				data: make([]int, 5),
			},
		},
		{
			name:      "用户传入nil",
			expectLen: 0,
			list: &ArrayList[int]{
				data: nil,
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actual := testCase.list.Cap()
			assert.Equal(t, testCase.expectLen, actual)
		})
	}
}

func TestArrayList_Get(t *testing.T) {
	testCases := []struct {
		name    string
		list    *ArrayList[int]
		index   int
		wantVal int
		wantErr error
	}{
		{
			name:    "index 0",
			list:    NewArrayListOf[int]([]int{123, 100}),
			index:   0,
			wantVal: 123,
		},
		{
			name:    "index 2",
			list:    NewArrayListOf[int]([]int{123, 100}),
			index:   2,
			wantVal: 0,
			wantErr: fmt.Errorf("go_utils: 下标超出范围，长度 %d, 下标 %d", 2, 2),
		},
		{
			name:    "index -1",
			list:    NewArrayListOf[int]([]int{123, 100}),
			index:   -1,
			wantVal: 0,
			wantErr: fmt.Errorf("go_utils: 下标超出范围，长度 %d, 下标 %d", 2, -1),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			val, err := tc.list.Get(tc.index)
			assert.Equal(t, tc.wantErr, err)
			// 因为返回了 error，所以我们不用继续往下比较了
			if err != nil {
				return
			}
			assert.Equal(t, tc.wantVal, val)
		})
	}
}
func TestArrayList_Range(t *testing.T) {
	testCases := []struct {
		name    string
		list    *ArrayList[int]
		index   int
		wantVal int
		wantErr error
	}{
		{
			name: "计算全部元素的和",
			list: &ArrayList[int]{
				data: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
			},
			wantVal: 55,
			wantErr: nil,
		},
		{
			name: "测试中断",
			list: &ArrayList[int]{
				data: []int{1, 2, 3, 4, -5, 6, 7, 8, -9, 10},
			},
			wantVal: 41,
			wantErr: errors.New("index 4 is error"),
		},
		{
			name: "测试数组为nil",
			list: &ArrayList[int]{
				data: nil,
			},
			wantVal: 0,
			wantErr: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := 0
			err := tc.list.Range(func(index int, num int) error {
				if num < 0 {
					return fmt.Errorf("index %d is error", index)
				}
				result += num
				return nil
			})

			assert.Equal(t, tc.wantErr, err)
			if err != nil {
				return
			}
			assert.Equal(t, tc.wantVal, result)
		})
	}
}

func TestArrayList_AsSlice(t *testing.T) {
	data := []int{1, 2, 3}
	a := NewArrayListOf[int](data)
	slice := a.AsSlice()
	// 内容相同
	assert.Equal(t, slice, data)
	aAddr := fmt.Sprintf("%p", data)
	sliceAddr := fmt.Sprintf("%p", slice)
	// 但是地址不同，也就是意味着 slice 必须是一个新创建的
	assert.NotEqual(t, aAddr, sliceAddr)
}

func TestArrayList_Set(t *testing.T) {
	testCases := []struct {
		name      string
		list      *ArrayList[int]
		index     int
		newVal    int
		wantSlice []int
		wantErr   error
	}{
		{
			name:      "set 5 by index  1",
			list:      NewArrayListOf[int]([]int{0, 1, 2, 3, 4}),
			index:     1,
			newVal:    5,
			wantSlice: []int{0, 5, 2, 3, 4},
			wantErr:   nil,
		},
		{
			name:      "index  -1",
			list:      NewArrayListOf[int]([]int{0, 1, 2, 3, 4}),
			index:     -1,
			newVal:    5,
			wantSlice: []int{},
			wantErr:   fmt.Errorf("go_utils: 下标超出范围，长度 %d, 下标 %d", 5, -1),
		},
		{
			name:      "index  100",
			list:      NewArrayListOf[int]([]int{0, 1, 2, 3, 4}),
			index:     100,
			newVal:    5,
			wantSlice: []int{},
			wantErr:   fmt.Errorf("go_utils: 下标超出范围，长度 %d, 下标 %d", 5, 100),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.list.Set(tc.index, tc.newVal)
			if err != nil {
				assert.Equal(t, tc.wantErr, err)
				return
			}
			assert.Equal(t, tc.wantSlice, tc.list.data)
		})
	}
}
