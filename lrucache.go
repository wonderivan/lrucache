package lrucache

import (
	"container/list"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/wonderivan/logger"
)

var (
	// The default minimum and maximum cache value
	defaultLowSize       int = (1 << 20) * 800               // 800 MB
	defaultHighSize      int = 1 << 30                       // 1 GB
	defaultCheckInterval int = 24 * 3600                     // 1 day
	defaultExpire            = 24 * 3600 * 365 * time.Second // 秒
)

type LruCache struct {
	sync.Mutex
	waitGroup     sync.WaitGroup
	name          string                   // 缓存名称
	low           int                      // 缓存空间大小最小阈值，清理数据时到该阈值
	high          int                      // 缓存空间大小最大阈值，超过该阈值时开始清理
	interval      int                      // 缓存空间检查间隔
	curCacheSize  int                      // 当前缓存使用大小
	chCacheExit   chan bool                // 清理go程序退出通知
	chShrinkCache chan bool                // 清理go程序执行通知
	valueList     *list.List               // lru逻辑列表
	keyIndex      map[string]*list.Element // 缓存数据
}

func NewLruCache(name string, config ...string) *LruCache {
	cache := new(LruCache)
	cache.name = name
	cache.low = defaultLowSize
	cache.high = defaultHighSize
	cache.interval = defaultCheckInterval
	cache.valueList = list.New()
	cache.keyIndex = make(map[string]*list.Element)
	cache.setConfig(append(config, "")[0])
	cache.chShrinkCache = make(chan bool, 1)
	cache.chCacheExit = make(chan bool)
	cache.waitGroup.Add(1)
	go cache.shrinkCache()
	logger.Info(`LruCache create with configure:
    --------------------------
    | name:"%s"          
    | low:%dbyte=%.3fMb
    | high:%dbyte=%.3fMb
    | interval:%d(s)
    --------------------------`,
		cache.name, cache.low, float64(cache.low)/1024/1024,
		cache.high, float64(cache.high)/1024/1024, cache.interval)
	return cache
}

// 销毁缓存
func (this *LruCache) Destroy() {
	if this != nil {
		logger.Info("LruCache(%s) Destroy", this.name)
		this.Lock()
		if this.chCacheExit != nil {
			close(this.chCacheExit)
			this.waitGroup.Wait()
			this.chCacheExit = nil
		}
		this.Unlock()
	}
	this = nil
}

// 从缓存中获取指定的有效数据，不存在或已过期返回nil
func (this *LruCache) Get(key string) interface{} {
	this.Lock()
	defer this.Unlock()
	if e, exist := this.keyIndex[key]; exist {
		itm := e.Value.(*MemoryItem)
		if (time.Now().Unix() - itm.lastAccess) > int64(itm.expired) {
			return nil
		}
		this.valueList.MoveToBack(e)
		return itm.val
	}
	return nil
}

// 从缓存中获取指定的多个有效数据
func (this *LruCache) GetMulti(keys []string) []interface{} {
	this.Lock()
	defer this.Unlock()
	rtl := make([]interface{}, len(keys))
	for i, key := range keys {
		if e, exist := this.keyIndex[key]; exist {
			itm := e.Value.(*MemoryItem)
			if (time.Now().Unix() - itm.lastAccess) > int64(itm.expired) {
				rtl[i] = itm.val
				this.valueList.MoveToBack(e)
				continue
			}
		}
		rtl[i] = nil
	}
	return rtl
}

// 从缓存中获取指定的有效数据，不存在或已过期返回(nil,false)
func (this *LruCache) GetEx(key string) (interface{}, bool) {
	this.Lock()
	defer this.Unlock()
	if e, exist := this.keyIndex[key]; exist {
		itm := e.Value.(*MemoryItem)
		if (time.Now().Unix() - itm.lastAccess) > int64(itm.expired) {
			return nil, false
		}
		this.valueList.MoveToBack(e)
		return itm.val, true
	}
	return nil, false
}

// 将数据写入到缓存中，并指定其数据的有效期expired
// 当expired=0时缓存数据立即失效，当expired<0时缓存数据有效期设置为默认的（24*3600*365)秒
// 缓存的key只允许是字符串，value支持任意格式
func (this *LruCache) Put(key string, val interface{}, expired time.Duration) {
	if expired < 0 {
		expired = defaultExpire
	}
	this.Lock()
	if e, exist := this.keyIndex[key]; exist {
		this.valueList.MoveToBack(e)
		itm := e.Value.(*MemoryItem)
		this.curCacheSize -= itm.Size()
		itm.val = val
		itm.expired = expired
		itm.lastAccess = time.Now().Unix()
		this.curCacheSize += itm.Size()
	} else {
		itm := &MemoryItem{key, val, time.Now().Unix(), expired}
		this.curCacheSize += itm.Size()
		this.keyIndex[key] = this.valueList.PushBack(itm)
	}
	this.Unlock()
	// 缓存检查
	if this.curCacheSize >= this.high {
		this.chShrinkCache <- true
	}
}

// 删除缓存中指定的key及其value，如果key不存在则返回err
func (this *LruCache) Delete(key string) error {
	this.Lock()
	defer this.Unlock()
	if e, exist := this.keyIndex[key]; exist {
		this.curCacheSize -= e.Value.(*MemoryItem).Size()
		delete(this.keyIndex, key)
		this.valueList.Remove(e)
	} else {
		return errors.New("key not exist")
	}
	return nil
}

// 延迟删除缓存中指定的key及其value，如果key不存在则返回err
func (this *LruCache) DelayDelete(key string, delay time.Duration) error {
	this.Lock()
	defer this.Unlock()
	if e, exist := this.keyIndex[key]; exist {
		itm := e.Value.(*MemoryItem)
		if (time.Now().Unix() - itm.lastAccess) <= int64(itm.expired) {
			itm.expired = delay
			itm.lastAccess = time.Now().Unix()
		}
	} else {
		return errors.New("key not exist")
	}
	return nil
}

// 判断缓存中指定的key值是否存在（有效），存在（有效）则返回true
func (this *LruCache) IsExist(key string) bool {
	this.Lock()
	defer this.Unlock()
	e, exist := this.keyIndex[key]
	if exist {
		itm := e.Value.(*MemoryItem)
		exist = (time.Now().Unix() - itm.lastAccess) <= int64(itm.expired)
	}
	return exist
}

// 清除所有缓存数据
func (this *LruCache) ClearAll() {
	this.Lock()
	defer this.Unlock()
	this.curCacheSize = 0
	this.valueList = list.New()
	this.keyIndex = make(map[string]*list.Element)
}

// 清除缓存中所有的指定的前缀key数据
func (this *LruCache) ClearPrefixKeys(prefix string) {
	logger.Info("LruCache(%s) delete all containing the prefix key(%s) Start;", this.name, prefix)
	curTime := time.Now()
	this.Lock()
	curCacheSize := this.curCacheSize
	keyArr := make([]string, 0, len(this.keyIndex))
	for key := range this.keyIndex {
		keyArr = append(keyArr, key)
	}
	this.Unlock()
	logger.Info("LruCache(%s) delete all containing the prefix key(%s) Proccing; ProccTotalNum:%d,CurCacheSize:%d(byte),usetime(%v)",
		this.name, prefix, len(keyArr), curCacheSize, time.Since(curTime))
	curTime = time.Now()
	var clearNum, clearSize, size int
	for _, key := range keyArr {
		if strings.HasPrefix(key, prefix) {
			this.Lock()
			if e, exist := this.keyIndex[key]; exist {
				itm := e.Value.(*MemoryItem)
				size = itm.Size()
				this.curCacheSize -= size
				delete(this.keyIndex, key)
				this.valueList.Remove(e)
				clearSize += size
				clearNum++
			}
			this.Unlock()
		}
	}
	logger.Info("LruCache(%s) delete all containing the prefix key(%s) End; TotalNum:%d, ClearNum:%d,ClearSize:%d(byte),usetime(%v)",
		this.name, prefix, len(keyArr), clearNum, clearSize, time.Since(curTime))
}

// 设置缓存参数
func (this *LruCache) setConfig(config string) {
	if config != "" {
		logger.Info("LruCache config: %s", config)
		var cf map[string]interface{}
		err := json.Unmarshal([]byte(config), &cf)
		if err != nil {
			logger.Info("LruCache config formate err:%s, use default", err)
			return
		}

		if value, exist := cf["name"]; exist {
			if name, ok := value.(string); ok {
				this.name = name
			}
		}

		if value, exist := cf["low"]; exist {
			if low, ok := value.(float64); ok {
				this.low = int(low)
			}
		}

		if value, exist := cf["high"]; exist {
			if high, ok := value.(float64); ok {
				this.high = int(high)
			}
		}
		if this.high < this.low {
			panic("the low must be less than high...")
		}

		if value, exist := cf["interval"]; exist {
			if interval, ok := value.(float64); ok {
				this.interval = int(interval)
			}
		}
	}
}

// 缩减缓存和垃圾回收
func (this *LruCache) shrinkCache() {
	defer this.waitGroup.Done()
	logger.Debug("LruCache(%s) Shrink Proccess goroutine Start;", this.name)
	defer logger.Debug("LruCache(%s) Shrink Proccess goroutine End;", this.name)
	timer := time.NewTicker(time.Second * time.Duration(this.interval))
	defer timer.Stop()
	for {
		select {
		case <-this.chShrinkCache: // 缩减缓存
			curTime := time.Now()
			this.Lock()
			curCacheSize := this.curCacheSize
			this.Unlock()
			var shrinkNum, shrinkSize, size int
			for {
				this.Lock()
				if this.curCacheSize > this.low {
					Value := this.valueList.Remove(this.valueList.Front())
					itm := Value.(*MemoryItem)
					size = itm.Size()
					this.curCacheSize -= size
					delete(this.keyIndex, itm.key)
					this.Unlock()
					shrinkSize += size
					shrinkNum++
				} else {
					this.Unlock()
					break
				}
			}
			if shrinkSize > 0 {
				logger.Info("LruCache(%s) ShrinkCache curSize:%d(byte),shrinkSize:%d(byte), shrinkObj:%d, usetime(%v)",
					this.name, curCacheSize, shrinkSize, shrinkNum, time.Since(curTime))
			}
		case <-timer.C: // 清理无效缓存数据
			logger.Info("LruCache(%s) CycleCheck Start;", this.name)

			this.Lock()
			curCacheSize := this.curCacheSize
			keyArr := make([]string, 0, len(this.keyIndex))
			for key := range this.keyIndex {
				keyArr = append(keyArr, key)
			}
			this.Unlock()
			logger.Info("LruCache(%s) CycleCheck totalObj:%d, totalSize:%d(byte)",
				this.name, len(keyArr), curCacheSize)
			curTime := time.Now()
			var shrinkNum, shrinkSize, size int
			for _, key := range keyArr {
				this.Lock()
				if e, exist := this.keyIndex[key]; exist {
					itm := e.Value.(*MemoryItem)
					if (time.Now().Unix() - itm.lastAccess) > int64(itm.expired) {
						size = itm.Size()
						this.curCacheSize -= size
						delete(this.keyIndex, key)
						this.valueList.Remove(e)
						shrinkSize += size
						shrinkNum++
					}
				}
				this.Unlock()
			}
			runtime.GC()
			logger.Info("LruCache(%s) CycleCheck End; clearObj:%d, clearSize:%d(byte), usetime(%v)",
				this.name, shrinkNum, shrinkSize, time.Since(curTime))
		case <-this.chCacheExit:
			return
		}
	}
}

// 获取缓存名称
func (this *LruCache) GetCacheName() string {
	return this.name
}
