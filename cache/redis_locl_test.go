package cache

import (
	"context"
	"github.com/golang/mock/gomock"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"go_utils/cache/mocks"
	"testing"
	"time"
)

// 其中mock生成，执行
// 1. 安装: https://github.com/golang/mock
// 2. mockgen.exe -package=mocks -destination="cache/mocks/redis_cmdable.mock.go" "github.com/redis/go-redis/v9" Cmdable

func TestClient_TryLock(t *testing.T) {
	testCases := []struct {
		name string
		mock func(ctrl *gomock.Controller) redis.Cmdable
		key  string

		wantErr  error
		wantLock *Lock
	}{
		{
			name: "set nx error",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				res := redis.NewBoolResult(false, context.DeadlineExceeded)
				cmd.EXPECT().SetNX(context.Background(), "key1", gomock.Any(), time.Minute).
					Return(res)
				return cmd
			},
			key:     "key1",
			wantErr: context.DeadlineExceeded,
		},
		{
			name: "FailedToPreemptLock",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				res := redis.NewBoolResult(false, nil)
				cmd.EXPECT().SetNX(context.Background(), "key1", gomock.Any(), time.Minute).
					Return(res)
				return cmd
			},
			key:     "key1",
			wantErr: ErrFailedToPreemptLock,
		},
		{
			name: "lock",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				res := redis.NewBoolResult(true, nil)
				cmd.EXPECT().SetNX(context.Background(), "key1", gomock.Any(), time.Minute).
					Return(res)
				return cmd
			},
			key: "key1",
			wantLock: &Lock{
				key: "key1",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			controller := gomock.NewController(t)
			defer controller.Finish()
			client := NewRedisClient(tc.mock(controller))
			lock, err := client.TryLock(context.Background(), tc.key, time.Minute)
			assert.Equal(t, tc.wantErr, err)
			if err != nil {
				return
			}
			assert.Equal(t, tc.wantLock.key, tc.key)
			assert.NotEmpty(t, lock.value)
		})
	}
}

func TestLock_Unlock(t *testing.T) {
	//ctrl := gomock.NewController(t)
	//defer ctrl.Finish()
	testCases := []struct {
		name string

		mock  func(ctrl *gomock.Controller) redis.Cmdable
		key   string
		value string

		//lock *Lock

		wantErr error
	}{
		{
			name: "eval error",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				res := redis.NewCmd(context.Background())
				res.SetErr(context.DeadlineExceeded)
				cmd.EXPECT().Eval(context.Background(), unLockLua, []string{"key1"}, []any{"value1"}).
					Return(res)
				return cmd
			},
			key:     "key1",
			value:   "value1",
			wantErr: context.DeadlineExceeded,
			//lock: &Lock{
			//	key: "key1",
			//	value: "value1",
			//	client: func() redis.Cmdable {
			//		cmd := mocks.NewMockCmdable(ctrl)
			//		return cmd
			//	}(),
			//}
		},
		{
			name: "lock not hold",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				res := redis.NewCmd(context.Background())
				res.SetVal(int64(0))
				cmd.EXPECT().Eval(context.Background(), unLockLua, []string{"key1"}, []any{"value1"}).
					Return(res)
				return cmd
			},
			key:     "key1",
			value:   "value1",
			wantErr: ErrLockNotHold,
		},
		{
			name: "unlocked",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				res := redis.NewCmd(context.Background())
				res.SetVal(int64(1))
				cmd.EXPECT().Eval(context.Background(), unLockLua, []string{"key1"}, []any{"value1"}).
					Return(res)
				return cmd
			},
			key:   "key1",
			value: "value1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			lock := &Lock{
				key:   tc.key,
				value: tc.value,
				c:     tc.mock(ctrl),
			}
			err := lock.UnLock(context.Background())
			assert.Equal(t, tc.wantErr, err)
		})
	}
}

func TestLock_Refresh(t *testing.T) {
	//ctrl := gomock.NewController(t)
	//defer ctrl.Finish()
	testCases := []struct {
		name string

		mock       func(ctrl *gomock.Controller) redis.Cmdable
		key        string
		value      string
		expiration time.Duration

		//lock *Lock

		wantErr error
	}{
		{
			name: "eval error",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				res := redis.NewCmd(context.Background())
				res.SetErr(context.DeadlineExceeded)
				cmd.EXPECT().Eval(context.Background(), refreshLua, []string{"key1"}, "value1", float64(60)).
					Return(res)
				return cmd
			},
			key:        "key1",
			value:      "value1",
			expiration: time.Minute,
			wantErr:    context.DeadlineExceeded,
		},
		{
			name: "lock not hold",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				res := redis.NewCmd(context.Background())
				res.SetVal(int64(0))
				cmd.EXPECT().Eval(context.Background(), refreshLua, []string{"key1"}, "value1", float64(60)).
					Return(res)
				return cmd
			},
			key:        "key1",
			value:      "value1",
			expiration: time.Minute,
			wantErr:    ErrLockNotHold,
		},
		{
			name: "unlocked",
			mock: func(ctrl *gomock.Controller) redis.Cmdable {
				cmd := mocks.NewMockCmdable(ctrl)
				res := redis.NewCmd(context.Background())
				res.SetVal(int64(1))
				cmd.EXPECT().Eval(context.Background(), refreshLua, []string{"key1"}, "value1", float64(60)).
					Return(res)
				return cmd
			},
			key:        "key1",
			value:      "value1",
			expiration: time.Minute,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			lock := &Lock{
				key:        tc.key,
				value:      tc.value,
				c:          tc.mock(ctrl),
				expiration: tc.expiration,
			}
			err := lock.Refresh(context.Background())
			assert.Equal(t, tc.wantErr, err)
		})
	}
}
