package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/ristretto"
	"github.com/golang/mock/gomock"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"go_utils/cache/mocks"
)

// TestUser 测试用的用户结构体
type TestUser struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

// MockDBService 模拟数据库服务
type MockDBService struct {
	users []TestUser
	delay time.Duration
	err   error
	mu    sync.RWMutex
}

func NewMockDBService(users []TestUser) *MockDBService {
	return &MockDBService{
		users: users,
		delay: 0,
	}
}

func (m *MockDBService) SetDelay(delay time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.delay = delay
}

func (m *MockDBService) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

func (m *MockDBService) FetchSegment(ctx context.Context, page, size int) ([]TestUser, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	if m.err != nil {
		return nil, m.err
	}

	total := len(m.users)
	if total == 0 {
		return []TestUser{}, nil
	}

	start := (page - 1) * size
	if start >= total {
		return []TestUser{}, nil
	}

	end := start + size
	if end > total {
		end = total
	}

	return m.users[start:end], nil
}

func (m *MockDBService) FetchCount(ctx context.Context) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.delay > 0 {
		time.Sleep(m.delay)
	}

	if m.err != nil {
		return 0, m.err
	}

	return len(m.users), nil
}

// 测试辅助函数
func createTestUsers(count int) []TestUser {
	users := make([]TestUser, count)
	for i := 0; i < count; i++ {
		users[i] = TestUser{
			ID:   i + 1,
			Name: fmt.Sprintf("User%d", i+1),
			Age:  20 + (i % 50),
		}
	}
	return users
}

func createLocalCache() *ristretto.Cache {
	cache, _ := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,
		MaxCost:     1 << 30,
		BufferItems: 64,
	})
	return cache
}

func TestNewPager(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()

	t.Run("with default codec", func(t *testing.T) {
		pager := NewPager[TestUser](Config{}, localCache, mockRedis, nil)
		assert.NotNil(t, pager)
		assert.IsType(t, JSONCodec{}, pager.codec)
	})

	t.Run("with custom codec", func(t *testing.T) {
		customCodec := JSONCodec{}
		pager := NewPager[TestUser](Config{}, localCache, mockRedis, customCodec)
		assert.NotNil(t, pager)
		assert.Equal(t, customCodec, pager.codec)
	})
}

func TestPager_GetPageOrAll_LocalCacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()
	defer localCache.Close()
	mockDB := NewMockDBService(createTestUsers(10))

	pager := NewPager[TestUser](Config{
		LocalTTL: time.Minute,
		RedisTTL: time.Hour,
	}, localCache, mockRedis, nil)

	// 预先设置本地缓存
	users := createTestUsers(10)
	data, _ := pager.codec.Marshal(users)
	localCache.SetWithTTL("test_key", data, 1, time.Minute)
	localCache.Wait()

	result, err := pager.GetPageOrAll(context.Background(), "test_key", PageRequest{Page: 1, Size: 5}, mockDB.FetchSegment, mockDB.FetchCount)

	assert.NoError(t, err)
	assert.Len(t, result.Items, 5)
	assert.Equal(t, 10, result.Total)
	assert.True(t, result.CacheStats.LocalHit)
	assert.False(t, result.CacheStats.RedisHit)
	assert.False(t, result.CacheStats.DBQueried)
}

func TestPager_GetPageOrAll_RedisCacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()
	defer localCache.Close()
	mockDB := NewMockDBService(createTestUsers(10))

	pager := NewPager[TestUser](Config{
		LocalTTL: time.Minute,
		RedisTTL: time.Hour,
	}, localCache, mockRedis, nil)

	users := createTestUsers(10)
	data, _ := pager.codec.Marshal(users)

	// 模拟Redis缓存命中
	mockRedis.EXPECT().Get(context.Background(), "test_key").Return(redis.NewStringResult(data, nil))

	result, err := pager.GetPageOrAll(context.Background(), "test_key", PageRequest{Page: 1, Size: 5}, mockDB.FetchSegment, mockDB.FetchCount)

	assert.NoError(t, err)
	assert.Len(t, result.Items, 5)
	assert.Equal(t, 10, result.Total)
	assert.False(t, result.CacheStats.LocalHit)
	assert.True(t, result.CacheStats.RedisHit)
	assert.False(t, result.CacheStats.DBQueried)
}

func TestPager_GetPageOrAll_DatabaseQuery(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()
	mockDB := NewMockDBService(createTestUsers(10))

	pager := NewPager[TestUser](Config{
		LocalTTL:      time.Minute,
		RedisTTL:      time.Hour,
		DBConcurrency: 1,
	}, localCache, mockRedis, nil)

	// 模拟缓存未命中
	mockRedis.EXPECT().Get(context.Background(), "test_key").Return(redis.NewStringResult("", redis.Nil))
	mockRedis.EXPECT().Set(context.Background(), "test_key", gomock.Any(), time.Hour).Return(redis.NewStatusResult("OK", nil))

	result, err := pager.GetPageOrAll(context.Background(), "test_key", PageRequest{Page: 1, Size: 5}, mockDB.FetchSegment, mockDB.FetchCount)

	assert.NoError(t, err)
	assert.Len(t, result.Items, 5)
	assert.Equal(t, 10, result.Total)
	assert.False(t, result.CacheStats.LocalHit)
	assert.False(t, result.CacheStats.RedisHit)
	assert.True(t, result.CacheStats.DBQueried)
}

func TestPager_GetPageOrAll_ReturnOnLocalMiss(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()
	mockDB := NewMockDBService(createTestUsers(10))

	pager := NewPager[TestUser](Config{
		LocalTTL: time.Minute,
		RedisTTL: time.Hour,
		Miss: MissBehavior{
			ReturnOnLocalMiss: true,
		},
	}, localCache, mockRedis, nil)

	result, err := pager.GetPageOrAll(context.Background(), "test_key", PageRequest{Page: 1, Size: 5}, mockDB.FetchSegment, mockDB.FetchCount)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stop on local cache miss")
	assert.Nil(t, result.Items)
	assert.Equal(t, 0, result.Total)
	assert.False(t, result.CacheStats.LocalHit)
	assert.False(t, result.CacheStats.RedisHit)
	assert.False(t, result.CacheStats.DBQueried)
}

func TestPager_GetPageOrAll_ReturnOnRedisMiss(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()
	mockDB := NewMockDBService(createTestUsers(10))

	pager := NewPager[TestUser](Config{
		LocalTTL: time.Minute,
		RedisTTL: time.Hour,
		Miss: MissBehavior{
			ReturnOnRedisMiss: true,
		},
	}, localCache, mockRedis, nil)

	// 模拟Redis缓存未命中
	mockRedis.EXPECT().Get(context.Background(), "test_key").Return(redis.NewStringResult("", redis.Nil))

	result, err := pager.GetPageOrAll(context.Background(), "test_key", PageRequest{Page: 1, Size: 5}, mockDB.FetchSegment, mockDB.FetchCount)

	assert.NoError(t, err)
	assert.Nil(t, result.Items)
	assert.Equal(t, 0, result.Total)
	assert.False(t, result.CacheStats.LocalHit)
	assert.False(t, result.CacheStats.RedisHit)
	assert.False(t, result.CacheStats.DBQueried)
}

func TestPager_GetPageOrAll_AllData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()
	mockDB := NewMockDBService(createTestUsers(10))

	pager := NewPager[TestUser](Config{
		LocalTTL:      time.Minute,
		RedisTTL:      time.Hour,
		DBConcurrency: 1,
	}, localCache, mockRedis, nil)

	// 模拟缓存未命中
	mockRedis.EXPECT().Get(context.Background(), "test_key").Return(redis.NewStringResult("", redis.Nil))
	mockRedis.EXPECT().Set(context.Background(), "test_key", gomock.Any(), time.Hour).Return(redis.NewStatusResult("OK", nil))

	// 请求所有数据（Page=0或Size=0）
	result, err := pager.GetPageOrAll(context.Background(), "test_key", PageRequest{Page: 0, Size: 0}, mockDB.FetchSegment, mockDB.FetchCount)

	assert.NoError(t, err)
	assert.Len(t, result.Items, 10)
	assert.Equal(t, 10, result.Total)
	assert.Equal(t, 1, result.Page)
	assert.Equal(t, 10, result.Size)
	assert.Equal(t, 1, result.Pages)
}

func TestPager_GetPageOrAll_EmptyData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()
	mockDB := NewMockDBService([]TestUser{}) // 空数据

	pager := NewPager[TestUser](Config{
		LocalTTL:      time.Minute,
		RedisTTL:      time.Hour,
		DBConcurrency: 1,
	}, localCache, mockRedis, nil)

	// 模拟缓存未命中
	mockRedis.EXPECT().Get(context.Background(), "test_key").Return(redis.NewStringResult("", redis.Nil))
	mockRedis.EXPECT().Set(context.Background(), "test_key", gomock.Any(), time.Hour).Return(redis.NewStatusResult("OK", nil))

	result, err := pager.GetPageOrAll(context.Background(), "test_key", PageRequest{Page: 1, Size: 5}, mockDB.FetchSegment, mockDB.FetchCount)

	assert.NoError(t, err)
	assert.Len(t, result.Items, 0)
	assert.Equal(t, 0, result.Total)
}

func TestPager_GetPageOrAll_PageOutOfRange(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()
	mockDB := NewMockDBService(createTestUsers(10))

	pager := NewPager[TestUser](Config{
		LocalTTL:      time.Minute,
		RedisTTL:      time.Hour,
		DBConcurrency: 1,
	}, localCache, mockRedis, nil)

	// 模拟缓存未命中
	mockRedis.EXPECT().Get(context.Background(), "test_key").Return(redis.NewStringResult("", redis.Nil))
	mockRedis.EXPECT().Set(context.Background(), "test_key", gomock.Any(), time.Hour).Return(redis.NewStatusResult("OK", nil))

	// 请求超出范围的页面
	result, err := pager.GetPageOrAll(context.Background(), "test_key", PageRequest{Page: 10, Size: 5}, mockDB.FetchSegment, mockDB.FetchCount)

	assert.NoError(t, err)
	assert.Len(t, result.Items, 0)
	assert.Equal(t, 10, result.Total)
	assert.Equal(t, 10, result.Page)
	assert.Equal(t, 5, result.Size)
	assert.Equal(t, 2, result.Pages)
	assert.False(t, result.HasPrev)
	assert.False(t, result.HasNext)
}

func TestPager_GetPageOrAll_DatabaseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()
	mockDB := NewMockDBService(createTestUsers(10))

	// 设置数据库错误
	dbError := errors.New("database connection failed")
	mockDB.SetError(dbError)

	pager := NewPager[TestUser](Config{
		LocalTTL:      time.Minute,
		RedisTTL:      time.Hour,
		DBConcurrency: 1,
	}, localCache, mockRedis, nil)

	// 模拟缓存未命中
	mockRedis.EXPECT().Get(context.Background(), "test_key").Return(redis.NewStringResult("", redis.Nil))

	result, err := pager.GetPageOrAll(context.Background(), "test_key", PageRequest{Page: 1, Size: 5}, mockDB.FetchSegment, mockDB.FetchCount)

	assert.Error(t, err)
	assert.Equal(t, dbError, err)
	assert.Empty(t, result.Items)
}

func TestPager_GetPageOrAll_RedisError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()
	mockDB := NewMockDBService(createTestUsers(10))

	pager := NewPager[TestUser](Config{
		LocalTTL:      time.Minute,
		RedisTTL:      time.Hour,
		DBConcurrency: 1,
	}, localCache, mockRedis, nil)

	// 模拟Redis错误，但应该继续查询数据库
	mockRedis.EXPECT().Get(context.Background(), "test_key").Return(redis.NewStringResult("", errors.New("redis error")))
	mockRedis.EXPECT().Set(context.Background(), "test_key", gomock.Any(), time.Hour).Return(redis.NewStatusResult("OK", nil))

	result, err := pager.GetPageOrAll(context.Background(), "test_key", PageRequest{Page: 1, Size: 5}, mockDB.FetchSegment, mockDB.FetchCount)

	assert.NoError(t, err)
	assert.Len(t, result.Items, 5)
	assert.Equal(t, 10, result.Total)
	assert.True(t, result.CacheStats.DBQueried)
}

func TestPager_ConcurrentFetchAll_SingleThread(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()
	mockDB := NewMockDBService(createTestUsers(1000)) // 大量数据

	pager := NewPager[TestUser](Config{
		LocalTTL:      time.Minute,
		RedisTTL:      time.Hour,
		DBConcurrency: 1, // 单线程
	}, localCache, mockRedis, nil)

	result, err := pager.concurrentFetchAll(context.Background(), mockDB.FetchSegment, mockDB.FetchCount)

	assert.NoError(t, err)
	assert.Len(t, result, 1000)
	// 验证数据顺序
	for i, user := range result {
		assert.Equal(t, i+1, user.ID)
	}
}

func TestPager_ConcurrentFetchAll_MultiThread(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()
	mockDB := NewMockDBService(createTestUsers(1000)) // 大量数据

	pager := NewPager[TestUser](Config{
		LocalTTL:      time.Minute,
		RedisTTL:      time.Hour,
		DBConcurrency: 4, // 多线程
	}, localCache, mockRedis, nil)

	result, err := pager.concurrentFetchAll(context.Background(), mockDB.FetchSegment, mockDB.FetchCount)

	assert.NoError(t, err)
	assert.Len(t, result, 1000)
	// 验证数据顺序（并发查询后应该保持正确顺序）
	for i, user := range result {
		assert.Equal(t, i+1, user.ID)
	}
}

func TestPager_ConcurrentFetchAll_SmallDataset(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()
	mockDB := NewMockDBService(createTestUsers(10)) // 小数据集

	pager := NewPager[TestUser](Config{
		LocalTTL:      time.Minute,
		RedisTTL:      time.Hour,
		DBConcurrency: 4,
	}, localCache, mockRedis, nil)

	result, err := pager.concurrentFetchAll(context.Background(), mockDB.FetchSegment, mockDB.FetchCount)

	assert.NoError(t, err)
	assert.Len(t, result, 10)
}

func TestPager_ConcurrentFetchAll_EmptyDataset(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()
	mockDB := NewMockDBService([]TestUser{}) // 空数据集

	pager := NewPager[TestUser](Config{
		LocalTTL:      time.Minute,
		RedisTTL:      time.Hour,
		DBConcurrency: 4,
	}, localCache, mockRedis, nil)

	result, err := pager.concurrentFetchAll(context.Background(), mockDB.FetchSegment, mockDB.FetchCount)

	assert.NoError(t, err)
	assert.Len(t, result, 0)
}

func TestPager_ConcurrentFetchAll_CountError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()
	mockDB := NewMockDBService(createTestUsers(10))

	// 设置计数错误
	countError := errors.New("count query failed")
	mockDB.SetError(countError)

	pager := NewPager[TestUser](Config{
		LocalTTL:      time.Minute,
		RedisTTL:      time.Hour,
		DBConcurrency: 4,
	}, localCache, mockRedis, nil)

	result, err := pager.concurrentFetchAll(context.Background(), mockDB.FetchSegment, mockDB.FetchCount)

	assert.Error(t, err)
	assert.Equal(t, countError, err)
	assert.Nil(t, result)
}

// 并发安全性测试
func TestPager_ConcurrentAccess(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()
	mockDB := NewMockDBService(createTestUsers(100))

	pager := NewPager[TestUser](Config{
		LocalTTL:      time.Minute,
		RedisTTL:      time.Hour,
		DBConcurrency: 2,
	}, localCache, mockRedis, nil)

	// 模拟多次Redis未命中，会触发数据库查询
	mockRedis.EXPECT().Get(gomock.Any(), gomock.Any()).Return(redis.NewStringResult("", redis.Nil)).AnyTimes()
	mockRedis.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(redis.NewStatusResult("OK", nil)).AnyTimes()

	const numGoroutines = 10
	const numRequests = 5

	var wg sync.WaitGroup
	results := make([]PageResult[TestUser], numGoroutines*numRequests)
	errors := make([]error, numGoroutines*numRequests)

	// 启动多个协程并发访问
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < numRequests; j++ {
				index := goroutineID*numRequests + j
				key := fmt.Sprintf("test_key_%d_%d", goroutineID, j)
				result, err := pager.GetPageOrAll(context.Background(), key, PageRequest{Page: 1, Size: 10}, mockDB.FetchSegment, mockDB.FetchCount)
				results[index] = result
				errors[index] = err
			}
		}(i)
	}

	wg.Wait()

	// 验证所有请求都成功
	for i, err := range errors {
		assert.NoError(t, err, "Request %d failed", i)
	}

	// 验证所有结果都正确
	for i, result := range results {
		assert.Len(t, result.Items, 10, "Result %d has wrong length", i)
		assert.Equal(t, 100, result.Total, "Result %d has wrong total", i)
	}
}

// Singleflight测试
func TestPager_Singleflight(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()
	mockDB := NewMockDBService(createTestUsers(10))

	// 添加延迟以确保并发请求
	mockDB.SetDelay(100 * time.Millisecond)

	pager := NewPager[TestUser](Config{
		LocalTTL:      time.Minute,
		RedisTTL:      time.Hour,
		DBConcurrency: 1,
	}, localCache, mockRedis, nil)

	// 模拟Redis未命中，只期望一次数据库查询（由于singleflight）
	mockRedis.EXPECT().Get(context.Background(), "test_key").Return(redis.NewStringResult("", redis.Nil)).Times(1)
	mockRedis.EXPECT().Set(context.Background(), "test_key", gomock.Any(), time.Hour).Return(redis.NewStatusResult("OK", nil)).Times(1)

	const numGoroutines = 5
	var wg sync.WaitGroup
	results := make([]PageResult[TestUser], numGoroutines)
	errors := make([]error, numGoroutines)

	// 同时启动多个协程请求相同的key
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			result, err := pager.GetPageOrAll(context.Background(), "test_key", PageRequest{Page: 1, Size: 5}, mockDB.FetchSegment, mockDB.FetchCount)
			results[index] = result
			errors[index] = err
		}(i)
	}

	wg.Wait()

	// 验证所有请求都成功且结果一致
	for i, err := range errors {
		assert.NoError(t, err, "Request %d failed", i)
	}

	for i, result := range results {
		assert.Len(t, result.Items, 5, "Result %d has wrong length", i)
		assert.Equal(t, 10, result.Total, "Result %d has wrong total", i)
		assert.True(t, result.CacheStats.DBQueried, "Result %d should indicate DB was queried", i)
	}
}

func TestPager_Invalidate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()

	pager := NewPager[TestUser](Config{}, localCache, mockRedis, nil)

	// 设置缓存
	users := createTestUsers(5)
	data, _ := pager.codec.Marshal(users)
	localCache.SetWithTTL("test_key", data, 1, time.Minute)
	localCache.Wait()

	// 验证缓存存在
	_, found := localCache.Get("test_key")
	assert.True(t, found)

	// 清除缓存
	pager.Invalidate("test_key")

	// 验证缓存已清除
	_, found = localCache.Get("test_key")
	assert.False(t, found)
}

func TestPager_Invalidate_NilLocalCache(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)

	pager := NewPager[TestUser](Config{}, nil, mockRedis, nil)

	// 应该不会panic
	assert.NotPanics(t, func() {
		pager.Invalidate("test_key")
	})
}

// 测试分页函数
func TestPaginate(t *testing.T) {
	users := createTestUsers(25)

	t.Run("normal pagination", func(t *testing.T) {
		result := paginate(users, PageRequest{Page: 2, Size: 10})
		assert.Len(t, result.Items, 10)
		assert.Equal(t, 25, result.Total)
		assert.Equal(t, 2, result.Page)
		assert.Equal(t, 10, result.Size)
		assert.Equal(t, 3, result.Pages)
		assert.True(t, result.HasPrev)
		assert.True(t, result.HasNext)
		assert.Equal(t, 11, result.Items[0].ID) // 第二页第一个元素
	})

	t.Run("last page", func(t *testing.T) {
		result := paginate(users, PageRequest{Page: 3, Size: 10})
		assert.Len(t, result.Items, 5) // 最后一页只有5个元素
		assert.Equal(t, 25, result.Total)
		assert.Equal(t, 3, result.Page)
		assert.Equal(t, 10, result.Size)
		assert.Equal(t, 3, result.Pages)
		assert.True(t, result.HasPrev)
		assert.False(t, result.HasNext)
	})

	t.Run("invalid page parameters", func(t *testing.T) {
		result := paginate(users, PageRequest{Page: 0, Size: 0})
		assert.Len(t, result.Items, 25) // 返回所有数据
		assert.Equal(t, 25, result.Total)
		assert.Equal(t, 1, result.Page)
		assert.Equal(t, 25, result.Size)
		assert.Equal(t, 1, result.Pages)
		assert.False(t, result.HasPrev)
		assert.False(t, result.HasNext)
	})

	t.Run("page out of range", func(t *testing.T) {
		result := paginate(users, PageRequest{Page: 10, Size: 10})
		assert.Len(t, result.Items, 0)
		assert.Equal(t, 25, result.Total)
		assert.Equal(t, 10, result.Page)
		assert.Equal(t, 10, result.Size)
		assert.Equal(t, 3, result.Pages)
		assert.False(t, result.HasPrev)
		assert.False(t, result.HasNext)
	})

	t.Run("empty data", func(t *testing.T) {
		result := paginate([]TestUser{}, PageRequest{Page: 1, Size: 10})
		assert.Len(t, result.Items, 0)
		assert.Equal(t, 0, result.Total)
		assert.Equal(t, 1, result.Page)
		assert.Equal(t, 10, result.Size)
		assert.Equal(t, 1, result.Pages)
		assert.False(t, result.HasPrev)
		assert.False(t, result.HasNext)
	})
}

// 测试JSON编解码器
func TestJSONCodec(t *testing.T) {
	codec := JSONCodec{}
	users := createTestUsers(3)

	t.Run("marshal and unmarshal", func(t *testing.T) {
		data, err := codec.Marshal(users)
		assert.NoError(t, err)
		assert.NotEmpty(t, data)

		var decoded []TestUser
		err = codec.Unmarshal(data, &decoded)
		assert.NoError(t, err)
		assert.Equal(t, users, decoded)
	})

	t.Run("unmarshal invalid data", func(t *testing.T) {
		var decoded []TestUser
		err := codec.Unmarshal("invalid json", &decoded)
		assert.Error(t, err)
	})
}

// 基准测试
func BenchmarkPager_GetPageOrAll_LocalHit(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()
	mockDB := NewMockDBService(createTestUsers(1000))

	pager := NewPager[TestUser](Config{
		LocalTTL: time.Minute,
		RedisTTL: time.Hour,
	}, localCache, mockRedis, nil)

	// 预热缓存
	users := createTestUsers(1000)
	data, _ := pager.codec.Marshal(users)
	localCache.SetWithTTL("bench_key", data, 1, time.Minute)
	localCache.Wait()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := pager.GetPageOrAll(context.Background(), "bench_key", PageRequest{Page: 1, Size: 10}, mockDB.FetchSegment, mockDB.FetchCount)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkPager_ConcurrentFetchAll(b *testing.B) {
	ctrl := gomock.NewController(b)
	defer ctrl.Finish()

	mockRedis := mocks.NewMockCmdable(ctrl)
	localCache := createLocalCache()
	mockDB := NewMockDBService(createTestUsers(1000))

	pager := NewPager[TestUser](Config{
		LocalTTL:      time.Minute,
		RedisTTL:      time.Hour,
		DBConcurrency: 4,
	}, localCache, mockRedis, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := pager.concurrentFetchAll(context.Background(), mockDB.FetchSegment, mockDB.FetchCount)
		if err != nil {
			b.Fatal(err)
		}
	}
}
