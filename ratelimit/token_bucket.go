package ratelimit

import "time"

type TokenBucket struct {
	cap       int
	bucket    chan struct{}
	limitRate int
	close     chan struct{}
}

func NewTokenBucket(limitRate, cap int) *TokenBucket {
	res := &TokenBucket{
		cap:    cap,
		bucket: make(chan struct{}, cap),
		close:  make(chan struct{}),
	}

	t := time.NewTicker(time.Duration(limitRate) * time.Second)
	go func() {
		for {
			select {
			case <-t.C:
				if len(res.bucket) < cap {
					res.bucket <- struct{}{}
				}
			case <-res.close:
				t.Stop()
				break
			}
		}
	}()
	return res
}

func (t *TokenBucket) Close() {
	close(t.close)
	close(t.bucket)
}

func (t *TokenBucket) Consume() {
	<-t.bucket
}
