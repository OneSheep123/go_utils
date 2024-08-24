package sync

import (
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

type one int

func (o *one) Increment() {
	*o++
}

func run(t *testing.T, once *Once, o *one, c chan bool) {
	err := once.Do(func() error {
		o.Increment()
		return nil
	})
	require.NoError(t, err)
	if v := *o; v != 1 {
		t.Errorf("once failed inside run: %d is not 1", v)
	}
	c <- true
}

func TestOnce(t *testing.T) {
	o := new(one)
	once := new(Once)
	c := make(chan bool)
	const N = 10
	for i := 0; i < N; i++ {
		go run(t, once, o, c)
	}
	for i := 0; i < N; i++ {
		<-c
	}
	if *o != 1 {
		t.Errorf("once failed outside run: %d is not 1", *o)
	}
}

type other int

func (o *other) Increment() {
	*o++
}

var OtherIncrementError = errors.New("Other 自增失败")

func TestOnce_FuncError(t *testing.T) {
	o := new(other)
	once := new(Once)
	c := make(chan bool)
	const N = 10
	for i := 0; i < N; i++ {
		go func() {
			err := once.Do(func() error {
				o.Increment()
				return OtherIncrementError
			})
			require.EqualError(t, err, OtherIncrementError.Error())
			c <- true
		}()
	}
	for i := 0; i < N; i++ {
		<-c
	}
	if *o != N {
		t.Errorf("once failed outside run: %d is not %d", *o, N)
	}
}
