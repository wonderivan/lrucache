package lrucache

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"runtime"
	"testing"
	"time"
)

type TestType struct {
	token string
}

func (this *TestType) GetToken() string { return this.token }

func TestCache1(t *testing.T) {
	runtime.GOMAXPROCS(4)
	cache := NewLruCache("test1", `{"low": 200,"high": 400}`)
	defer cache.Destroy()

	cache.Put("test", &TestType{"test"}, 2)
	testval := cache.Get("test")
	if testval == nil {
		t.Fatal("testval should not nil!")
	}
	if !cache.IsExist("test") {
		t.Fatal("key->test should exist!")
	}
	time.Sleep(3 * time.Second)
	testval = cache.Get("test")
	if testval != nil {
		t.Fatal("testval should nil!")
	}
	if cache.IsExist("test") {
		t.Fatal("key->test should not exist!")
	}

	testval = cache.Get("testtttt")

	cache.Put("test", &TestType{"testtt"}, 1)

	cache.Put("test1", "", 0)

	err := cache.Delete("test2")
	if err == nil {
		t.Fatal("delete should return error!")
	}
	cache.Put("test3", &TestType{"test3"}, 0)
	err = cache.Delete("test3")
	if err != nil {
		t.Fatal("delete should return right!")
	}
	cache.ClearAll()
	if cache.curCacheSize != 0 {
		t.Fatal("cache.size() should zero")
	}
	cache.Put("test4", &TestType{"test4"}, 0)
	cache.Put("test5", &TestType{"test5"}, 0)
	cache.Put("test6", &TestType{"test6"}, 0)

	for e := cache.valueList.Back(); e != nil; e = e.Prev() {
		fmt.Println("First = ", e.Value.(*MemoryItem).key)
	}
	cache.Get("test5")

	for e := cache.valueList.Back(); e != nil; e = e.Prev() {
		fmt.Println("Second = ", e.Value.(*MemoryItem).key)
	}

	cache.ClearAll()

	for i := 1; i < 15; i++ {
		key := fmt.Sprintf("kkkkkkkkkkkkkkkk->%d", i)
		value := &TestType{key}
		cache.Put(key, value, 1)
		info := cache.Get(key)
		fmt.Printf("Get %s' value = %v\n", key, info.(*TestType).GetToken())
		fmt.Println("Current CacheNum = ", cache.curCacheSize)
	}
	time.Sleep(time.Second)
	for k := range cache.keyIndex {
		fmt.Println("LastKey = ", k)
	}
}

func GenerateTopic(fund_account int32) []byte {
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.BigEndian, fund_account)
	return buf.Bytes()
}

func TestClearAllPrefixKeys(t *testing.T) {
	cache := NewLruCache("test4", `{"name":"testCache","low": 140000,"high": 200000}`)
	defer func() {
		cache.Destroy()
		time.Sleep(time.Second)
	}()
	cache.Put("test6", &TestType{"test6"}, 0)
	cache.Put("111test4", &TestType{"test4"}, 0)

	for i := 1; i < 5; i++ {
		key := fmt.Sprintf("test%d", i)
		value := &TestType{key}
		cache.Put(key, value, 0)
		info := cache.Get(key)
		fmt.Println("Gut Key = ", info.(*TestType).GetToken())
		fmt.Println("Current CacheSize = ", cache.curCacheSize)
		fmt.Printf("\n*******************\n\n")
		time.Sleep(time.Second)
	}

	cache.ClearPrefixKeys("test")
	for k := range cache.keyIndex {
		fmt.Println("LastKey = ", k)
	}

}

func TestDelayDelete(t *testing.T) {
	cache := NewLruCache("test5", `{"low": 140,"high": 200}`)
	defer cache.Destroy()
	cache.Put("test", &TestType{"test"}, 1)
	testval := cache.Get("test")
	if testval == nil {
		t.Fatal("testval should not nil!")
	}
	cache.DelayDelete("test", 3)
	time.Sleep(2 * time.Second)
	if !cache.IsExist("test") {
		t.Fatal("key->test should exist!")
	}
}

// go test -benchmem -run=^$ -bench ^BenchmarkPut$
func BenchmarkPut(b *testing.B) {
	cache := NewLruCache("test2", `{"low": 140000000,"high": 200000000}`)

	b.N = 10000000
	keys := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		keys[i] = string(GenerateTopic(int32(i)))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Put(keys[i], &TestType{keys[i]}, -1)
	}
}

// go test -benchmem -run=^$ -bench ^BenchmarkGet$
func BenchmarkGet(b *testing.B) {
	cache := NewLruCache("test3", `{"low": 140000000,"high": 200000000}`)
	b.N = 10000000
	keys := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		keys[i] = string(GenerateTopic(int32(i)))
	}
	for i := 0; i < b.N; i++ {
		cache.Put(keys[i], &TestType{keys[i]}, -1)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(keys[i])
	}
}
