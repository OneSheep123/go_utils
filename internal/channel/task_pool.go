package channel

import "context"

type Task func()

// TaskPool 任务池 / 协程池
type TaskPool struct {
	tasks chan Task
	close chan struct{}
}

func NewTaskPool(numG int, cap int) *TaskPool {
	t := &TaskPool{
		tasks: make(chan Task, cap),
		close: make(chan struct{}),
	}

	for i := 0; i < numG; i++ {
		go func() {
			for {
				select {
				case <-t.close:
					return
				case f := <-t.tasks:
					f()
				}
			}
		}()
	}
	return t
}

func (tp *TaskPool) Submit(ctx context.Context, t Task) error {
	select {
	case tp.tasks <- t:
	// 超时控制，防止任务队列满了情况
	case <-ctx.Done():
		return ctx.Err()
	}
	return nil
}

// Close 要暴露出来
func (tp *TaskPool) Close() error {
	close(tp.close)
	return nil
}
