package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache Redis 缓存封装
type Cache struct {
	client *redis.Client
	prefix string // key 前缀，避免与其他服务冲突
}

// Config Redis 配置
type Config struct {
	Host     string
	Port     string
	Password string
	DB       int
	Prefix   string
}

// New 创建 Redis 缓存实例
// 如果 Redis 连接失败，返回 nil（降级为无缓存模式）
func New(cfg *Config) *Cache {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("[Cache] Redis 连接失败，将以无缓存模式运行: %v", err)
		return nil
	}

	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "oj"
	}

	log.Printf("[Cache] Redis 连接成功: %s:%s", cfg.Host, cfg.Port)
	return &Cache{
		client: client,
		prefix: prefix,
	}
}

// Close 关闭 Redis 连接
func (c *Cache) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

// key 生成带前缀的 key
func (c *Cache) key(parts ...string) string {
	k := c.prefix
	for _, p := range parts {
		k += ":" + p
	}
	return k
}

// Get 获取缓存，反序列化到 dest 中
// 返回 true 表示命中缓存
func (c *Cache) Get(ctx context.Context, dest interface{}, keyParts ...string) bool {
	if c == nil {
		return false
	}

	data, err := c.client.Get(ctx, c.key(keyParts...)).Bytes()
	if err != nil {
		return false // 未命中或错误，都当作 miss
	}

	if err := json.Unmarshal(data, dest); err != nil {
		log.Printf("[Cache] 反序列化失败 key=%s: %v", c.key(keyParts...), err)
		return false
	}

	return true
}

// Set 写入缓存
func (c *Cache) Set(ctx context.Context, value interface{}, ttl time.Duration, keyParts ...string) {
	if c == nil {
		return
	}

	data, err := json.Marshal(value)
	if err != nil {
		log.Printf("[Cache] 序列化失败 key=%s: %v", c.key(keyParts...), err)
		return
	}

	if err := c.client.Set(ctx, c.key(keyParts...), data, ttl).Err(); err != nil {
		log.Printf("[Cache] 写入失败 key=%s: %v", c.key(keyParts...), err)
	}
}

// Delete 删除缓存
func (c *Cache) Delete(ctx context.Context, keyParts ...string) {
	if c == nil {
		return
	}

	if err := c.client.Del(ctx, c.key(keyParts...)).Err(); err != nil {
		log.Printf("[Cache] 删除失败 key=%s: %v", c.key(keyParts...), err)
	}
}

// DeletePattern 按模式删除（如 "oj:problems:*"）
func (c *Cache) DeletePattern(ctx context.Context, pattern string) {
	if c == nil {
		return
	}

	fullPattern := c.key(pattern)
	iter := c.client.Scan(ctx, 0, fullPattern, 100).Iterator()
	for iter.Next(ctx) {
		c.client.Del(ctx, iter.Val())
	}
	if err := iter.Err(); err != nil {
		log.Printf("[Cache] 模式删除失败 pattern=%s: %v", fullPattern, err)
	}
}
