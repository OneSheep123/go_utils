package failover

import (
	"context"
	"go_utils/internal/sms"
	"sync/atomic"
)

type TimeoutFailoverSMSService struct {
	//lock sync.Mutex
	svcs []sms.Service
	idx  int32

	// 连续超时次数
	cnt int32

	// 连续超时次数阈值
	threshold int32
}

func NewTimeoutFailoverSMSService(svcs []sms.Service, threshold int32) *TimeoutFailoverSMSService {
	return &TimeoutFailoverSMSService{
		svcs:      svcs,
		threshold: threshold,
	}
}

func (t *TimeoutFailoverSMSService) Send(ctx context.Context, tplId string, args []string, numbers ...string) error {
	cnt := atomic.LoadInt32(&t.cnt)
	idx := atomic.LoadInt32(&t.idx)
	if cnt >= t.threshold {
		// 触发切换，计算新的下标
		newIdx := (idx + 1) % int32(len(t.svcs))
		// CAS 操作失败，说明有人切换了，所以你这里不需要检测返回值
		if atomic.CompareAndSwapInt32(&t.idx, idx, newIdx) {
			// 说明你切换了
			atomic.StoreInt32(&t.cnt, 0)
		}
		idx = newIdx
	}
	svc := t.svcs[idx]
	// 当前使用的 svc
	err := svc.Send(ctx, tplId, args, numbers...)
	switch err {
	case nil:
		// 没有任何错误，重置计数器
		atomic.StoreInt32(&t.cnt, 0)
	case context.DeadlineExceeded:
		atomic.AddInt32(&t.cnt, 1)
	default:
		// 如果是别的异常的话，我们保持不动
	}
	return err
}
