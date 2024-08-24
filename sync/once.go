package sync

import (
	"sync"
	"sync/atomic"
)

type Once struct {
	m    sync.Mutex
	done atomic.Int32
}

func (o *Once) Do(f func() error) error {
	if o.done.Load() == 1 {
		return nil
	}
	return o.slowDo(f)
}

func (o *Once) slowDo(f func() error) error {
	o.m.Lock()
	defer o.m.Unlock()
	var err error
	if o.done.Load() == 0 {
		err = f()
		if err == nil {
			o.done.Store(1)
		}
	}
	return err
}
