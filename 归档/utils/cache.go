package utils

import (
	"sync"
	"time"
)

// CacheItem 缓存项
type CacheItem struct {
	Value      interface{}
	ExpireTime time.Time
}

// CacheManager 缓存管理器
type CacheManager struct {
	items sync.Map
	mutex sync.RWMutex
}

// NewCacheManager 创建缓存管理器
func NewCacheManager() *CacheManager {
	return &CacheManager{}
}

// Set 设置缓存
func (cm *CacheManager) Set(key string, value interface{}, duration time.Duration) {
	cm.items.Store(key, CacheItem{
		Value:      value,
		ExpireTime: time.Now().Add(duration),
	})
}

// Get 获取缓存
func (cm *CacheManager) Get(key string) (interface{}, bool) {
	item, ok := cm.items.Load(key)
	if !ok {
		return nil, false
	}

	cacheItem := item.(CacheItem)
	if time.Now().After(cacheItem.ExpireTime) {
		// 缓存已过期
		cm.items.Delete(key)
		return nil, false
	}

	return cacheItem.Value, true
}

// Delete 删除缓存
func (cm *CacheManager) Delete(key string) {
	cm.items.Delete(key)
}

// Clear 清空缓存
func (cm *CacheManager) Clear() {
	cm.items.Range(func(key, _ interface{}) bool {
		cm.items.Delete(key)
		return true
	})
}

// GetSize 获取缓存大小
func (cm *CacheManager) GetSize() int {
	count := 0
	cm.items.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// StartCleanup 启动缓存清理
func (cm *CacheManager) StartCleanup(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			<-ticker.C
			cm.cleanupExpired()
		}
	}()
}

// cleanupExpired 清理过期缓存
func (cm *CacheManager) cleanupExpired() {
	cm.items.Range(func(key, value interface{}) bool {
		cacheItem := value.(CacheItem)
		if time.Now().After(cacheItem.ExpireTime) {
			cm.items.Delete(key)
		}
		return true
	})
}

// 全局缓存管理器
var Cache = NewCacheManager()

// 启动缓存清理
func InitCache() {
	// 每5分钟清理一次过期缓存
	Cache.StartCleanup(5 * time.Minute)
}
