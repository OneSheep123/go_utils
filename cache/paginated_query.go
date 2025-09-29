package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/dgraph-io/ristretto"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

// ----------------------------- 公共类型 -----------------------------

// PageRequest 分页查询请求参数
// Page等于0, 获取Size等于0, 返回所有数据
type PageRequest struct {
	Page int
	Size int
}

type PageResult[T any] struct {
	Items      []T
	Total      int
	Page       int
	Size       int
	Pages      int
	HasPrev    bool
	HasNext    bool
	CacheStats CacheStats // 缓存命中统计信息
}

// CacheStats 缓存统计信息
type CacheStats struct {
	LocalHit  bool `json:"local_hit"`  // 本地缓存是否命中
	RedisHit  bool `json:"redis_hit"`  // Redis缓存是否命中
	DBQueried bool `json:"db_queried"` // 是否查询了数据库
}

// cacheResult 内部使用的缓存结果，包含数据和统计信息
type cacheResult struct {
	Data  string
	Stats CacheStats
}

type MissBehavior struct {
	ReturnOnLocalMiss bool
	ReturnOnRedisMiss bool
}

type Config struct {
	LocalTTL      time.Duration
	RedisTTL      time.Duration
	Miss          MissBehavior
	DBConcurrency int // 数据库并发查询数
}

// FetchSegmentFunc 定义了分页查询数据的函数
type FetchSegmentFunc[T any] func(ctx context.Context, page, size int) ([]T, error)

// FetchCountFunc 定义了查询数据总数的函数
type FetchCountFunc func(ctx context.Context) (int, error)

// Codec 定义了编码解码接口
type Codec interface {
	Marshal(v any) (string, error)
	Unmarshal(data string, v any) error
}

type JSONCodec struct{}

func (JSONCodec) Marshal(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}
func (JSONCodec) Unmarshal(data string, v any) error {
	return json.Unmarshal([]byte(data), v)
}

// ----------------------------- 主组件 -----------------------------

// Pager 分页查询组件
type Pager[T any] struct {
	cfg   Config
	local *ristretto.Cache
	redis redis.Cmdable
	codec Codec
	group singleflight.Group
}

var (
	errStopOnLocalMiss = errors.New("stop on local cache miss")
	errStopOnRedisMiss = errors.New("stop on redis miss")
)

func NewPager[T any](cfg Config, localCache *ristretto.Cache, redis redis.Cmdable, codec Codec) *Pager[T] {
	if codec == nil {
		// 默认使用 JSON 编码
		codec = JSONCodec{}
	}
	return &Pager[T]{
		cfg:   cfg,
		local: localCache,
		redis: redis,
		codec: codec,
	}
}

func (p *Pager[T]) GetPageOrAll(ctx context.Context, key string, pageReq PageRequest, fetchAll FetchSegmentFunc[T], countFunc FetchCountFunc) (PageResult[T], error) {
	// 本地缓存
	if p.local != nil {
		if v, ok := p.local.Get(key); ok {
			if s, sok := v.(string); sok {
				var items []T
				if err := p.codec.Unmarshal(s, &items); err == nil {
					result := paginate(items, pageReq)
					result.CacheStats = CacheStats{
						LocalHit:  true,
						RedisHit:  false,
						DBQueried: false,
					}
					return result, nil
				}
			}
		}
	}
	// 本地缓存未命中
	if p.cfg.Miss.ReturnOnLocalMiss {
		result := PageResult[T]{
			Items: nil,
			Total: 0,
			Page:  max(1, pageReq.Page),
			Size:  pageReq.Size,
			CacheStats: CacheStats{
				LocalHit:  false,
				RedisHit:  false,
				DBQueried: false,
			},
		}
		return result, errStopOnLocalMiss
	}

	v, err, _ := p.group.Do(key, func() (any, error) {
		// 检查本地缓存（在 singleflight 内部再次检查）
		if p.local != nil {
			if vv, ok := p.local.Get(key); ok {
				if s, sok := vv.(string); sok {
					return cacheResult{
						Data: s,
						Stats: CacheStats{
							LocalHit:  true,
							RedisHit:  false,
							DBQueried: false,
						},
					}, nil
				}
			}
		}

		// Redis 缓存
		if p.redis != nil {
			if rs, rerr := p.redis.Get(ctx, key).Result(); rerr == nil && rs != "" {
				if p.local != nil {
					p.local.SetWithTTL(key, rs, 1, p.cfg.LocalTTL)
					p.local.Wait()
				}
				return cacheResult{
					Data: rs,
					Stats: CacheStats{
						LocalHit:  false,
						RedisHit:  true,
						DBQueried: false,
					},
				}, nil
			}
		}
		// Redis 未命中
		if p.cfg.Miss.ReturnOnRedisMiss {
			return cacheResult{
				Data: "",
				Stats: CacheStats{
					LocalHit:  false,
					RedisHit:  false,
					DBQueried: false,
				},
			}, errStopOnRedisMiss
		}

		// ----------------- 并发 DB 分页查询 -----------------
		items, derr := p.concurrentFetchAll(ctx, fetchAll, countFunc)
		if derr != nil {
			return cacheResult{}, derr
		}

		enc, merr := p.codec.Marshal(items)
		if merr != nil {
			return cacheResult{}, merr
		}

		if p.redis != nil {
			_ = p.redis.Set(ctx, key, enc, p.cfg.RedisTTL)
		}

		if p.local != nil {
			p.local.SetWithTTL(key, enc, 1, p.cfg.LocalTTL)
			p.local.Wait()
		}

		return cacheResult{
			Data: enc,
			Stats: CacheStats{
				LocalHit:  false,
				RedisHit:  false,
				DBQueried: true,
			},
		}, nil
	})

	if err != nil {
		if errors.Is(err, errStopOnLocalMiss) || errors.Is(err, errStopOnRedisMiss) {
			// 从 v 中获取统计信息
			if result, ok := v.(cacheResult); ok {
				return PageResult[T]{
					Items:      nil,
					Total:      0,
					Page:       max(1, pageReq.Page),
					Size:       pageReq.Size,
					CacheStats: result.Stats,
				}, nil
			}
			// 兜底情况
			return PageResult[T]{
				Items: nil,
				Total: 0,
				Page:  max(1, pageReq.Page),
				Size:  pageReq.Size,
				CacheStats: CacheStats{
					LocalHit:  false,
					RedisHit:  false,
					DBQueried: false,
				},
			}, nil
		}
		return PageResult[T]{}, err
	}

	result, ok := v.(cacheResult)
	if !ok {
		return PageResult[T]{}, errors.New("unexpected result type from singleflight")
	}

	var items []T
	if uerr := p.codec.Unmarshal(result.Data, &items); uerr != nil {
		return PageResult[T]{}, uerr
	}

	pageResult := paginate(items, pageReq)
	pageResult.CacheStats = result.Stats
	return pageResult, nil
}

func (p *Pager[T]) concurrentFetchAll(ctx context.Context, fetchSegment FetchSegmentFunc[T], fetchCount FetchCountFunc) ([]T, error) {
	// 先获取数据总数
	total, err := fetchCount(ctx)
	if err != nil {
		return nil, err
	}
	if total == 0 {
		return []T{}, nil
	}

	// 固定每批最多处理500条记录
	const maxBatchSize = 500

	// 如果并发数小于等于1，使用单线程模式
	if p.cfg.DBConcurrency <= 1 {
		// 单线程模式下，如果数据量超过500条，分批处理
		if total <= maxBatchSize {
			return fetchSegment(ctx, 1, total)
		}

		// 分批处理大量数据
		var finalResult []T
		for offset := 0; offset < total; offset += maxBatchSize {
			batchSize := maxBatchSize
			if offset+batchSize > total {
				batchSize = total - offset
			}

			// 计算页码（从1开始）
			page := (offset / maxBatchSize) + 1
			data, err := fetchSegment(ctx, page, batchSize)
			if err != nil {
				return nil, err
			}
			finalResult = append(finalResult, data...)
		}
		return finalResult, nil
	}

	// 计算需要的批次数量（每批最多500条）
	totalBatches := (total + maxBatchSize - 1) / maxBatchSize

	// 实际并发数不能超过配置的并发数和总批次数
	actualConcurrency := p.cfg.DBConcurrency
	if actualConcurrency > totalBatches {
		actualConcurrency = totalBatches
	}

	// 使用有序结果收集器确保结果顺序
	type segmentResult struct {
		index int // 分段索引
		data  []T // 分段数据
	}

	// 创建结果通道，容量等于总批次数
	resultChan := make(chan segmentResult, totalBatches)

	var g errgroup.Group

	// 控制并发数量
	g.SetLimit(actualConcurrency)

	// 启动所有批次的查询任务
	for batchIndex := 0; batchIndex < totalBatches; batchIndex++ {
		// 计算当前批次的数据范围
		offset := batchIndex * maxBatchSize
		batchSize := maxBatchSize
		if offset+batchSize > total {
			batchSize = total - offset
		}

		// 捕获循环变量
		currentBatchIndex := batchIndex
		currentPage := batchIndex + 1 // 页码从1开始
		currentSize := batchSize

		g.Go(func() error {
			// 执行分页查询
			data, er := fetchSegment(ctx, currentPage, currentSize)

			// 将结果发送到通道
			resultChan <- segmentResult{
				index: currentBatchIndex,
				data:  data,
			}

			return er
		})
	}

	// 等待所有协程完成并关闭结果通道
	go func() {
		g.Wait()
		close(resultChan)
	}()

	// 检查是否有错误发生
	if err = g.Wait(); err != nil {
		return nil, err
	}

	// 收集结果并按索引排序
	segments := make(map[int][]T)

	for result := range resultChan {
		segments[result.index] = result.data
	}

	// 按顺序合并结果
	var finalResult []T
	for i := 0; i < totalBatches; i++ {
		if segment, exists := segments[i]; exists {
			finalResult = append(finalResult, segment...)
		}
	}

	return finalResult, nil
}

func (p *Pager[T]) Invalidate(key string) {
	if p.local == nil {
		return
	}
	p.local.Del(key)
}

// paginate 分页函数
func paginate[T any](items []T, req PageRequest) PageResult[T] {
	total := len(items)
	if req.Page <= 0 || req.Size <= 0 {
		// 分页参数错误，返回所有数据
		return PageResult[T]{
			Items:      items,
			Total:      total,
			Page:       1,
			Size:       total,
			Pages:      1,
			HasPrev:    false,
			HasNext:    false,
			CacheStats: CacheStats{}, // 初始化为零值，调用方会设置正确的值
		}
	}
	page := req.Page
	size := req.Size
	start := (page - 1) * size
	if start >= total {
		// 分页超出范围，返回空数据
		return PageResult[T]{
			Items:      []T{},
			Total:      total,
			Page:       page,
			Size:       size,
			Pages:      max(1, (total+size-1)/size),
			HasPrev:    false,
			HasNext:    false,
			CacheStats: CacheStats{}, // 初始化为零值，调用方会设置正确的值
		}
	}
	end := start + size
	if end > total {
		end = total
	}
	pages := (total + size - 1) / size
	// 分页数据
	return PageResult[T]{
		Items:      items[start:end],
		Total:      total,
		Page:       page,
		Size:       size,
		Pages:      max(1, pages),
		HasPrev:    page > 1,
		HasNext:    page < pages,
		CacheStats: CacheStats{}, // 初始化为零值，调用方会设置正确的值
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
