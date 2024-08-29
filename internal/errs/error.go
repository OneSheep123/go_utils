package errs

import (
	"errors"
	"fmt"
)

var (
	ErrKeyNotFound      = errors.New("go_utils：键不存在")
	ErrOverCapacity     = errors.New("go_utils：超过容量限制")
	ErrFailedToSetCache = errors.New("go_utils: 写入 redis 失败")
)

// NewErrIndexOutOfRange 创建一个代表下标超出范围的错误
func NewErrIndexOutOfRange(length int, index int) error {
	return fmt.Errorf("go_utils: 下标超出范围，长度 %d, 下标 %d", length, index)
}
