package cache

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
	"sync"
)

type MetaInfo struct {
	Data     []byte   `json:"data"`
	IsBigKey bool     `json:"is_big_key"`
	Keys     []string `json:"keys"`
}

// StoreValueInRedis 分快存储大key, chunkSize > 0 代表需要进行分块
func StoreValueInRedis(ctx context.Context, client redis.Cmdable, key string, value []byte, chunkSize int) error {
	totalChunks := len(value) / chunkSize
	// 如果除不尽就多加一块
	if len(value)%chunkSize != 0 {
		totalChunks++
	}
	group := errgroup.Group{}
	group.SetLimit(3)
	meta := MetaInfo{IsBigKey: false, Data: value}
	if totalChunks > 0 {
		version := md5LastSixBytes(value)
		keys := make([]string, 0, totalChunks)
		for i := 0; i < totalChunks; i++ {
			start := i * chunkSize
			end := (i + 1) * chunkSize
			if end > len(value) {
				end = len(value)
			}
			chunk := value[start:end]
			// ⼦Key拼接上版本号，避免直接覆盖之前的数据，导致脏读
			chunkKey := fmt.Sprintf("%s:%s:%d", key, version, i)
			keys = append(keys, chunkKey)

			group.Go(func() error {
				return client.Set(context.Background(), chunkKey, chunk, 0).Err()
			})
		}
		err := group.Wait()
		if err != nil {
			return err
		}
		meta = MetaInfo{IsBigKey: true, Keys: keys}
	}
	marshal, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	_, err = client.Set(ctx, key, marshal, 0).Result()
	return err
}

// md5LastSixBytes md5最后6位作为版本号
func md5LastSixBytes(value []byte) string {
	sum := md5.Sum(value)                // 16字节
	hexStr := hex.EncodeToString(sum[:]) // 32个十六进制字符
	return hexStr[len(hexStr)-6:]        // 取最后6个字符
}

func GetDataFromRedis(ctx context.Context, client redis.Cmdable, key string) ([]byte, error) {
	metaByte, err := client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, err
	}
	var metaInfo MetaInfo
	err = json.Unmarshal(metaByte, &metaInfo)
	if err != nil {
		return nil, err
	}
	if !metaInfo.IsBigKey {
		return metaInfo.Data, nil
	}
	group := errgroup.Group{}
	group.SetLimit(10)
	ch := make(chan int, 1) // 缓冲为1的通道
	ch <- 1
	var data []byte
	l := sync.Mutex{}
	for i := 1; i <= len(metaInfo.Keys); i++ {
		i := i
		group.Go(func() error {
			for {
				// 保证顺序执行
				v := <-ch
				if i == v {
					result, err2 := client.Get(ctx, metaInfo.Keys[i-1]).Result()
					if err2 != nil {
						return err2
					}
					chunkData := []byte(result)
					l.Lock()
					data = append(data, chunkData...)
					l.Unlock()
					ch <- v + 1
					break
				}
				ch <- v
			}
			return nil
		})
	}
	err = group.Wait()
	if err != nil {
		return nil, err
	}
	return data, err
}
