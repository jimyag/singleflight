# singleflight

This repo is a hard fork of [go-zero/singleflight.go at master · zeromicro/go-zero (github.com)](https://github.com/zeromicro/go-zero/blob/master/core/syncx/singleflight.go) that adds generics to the Group type so that there is no need for type assertion when using the library.

## Install

```shell
go get github.com/jimyag/singleflight@latest
```

## Usage

```go
package main

import (
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jimyag/singleflight"
)

func main() {
	g := singleflight.NewSingleFlight[string]()
	c := make(chan string)
	var calls int32
	// 给 calls 加1
	fn := func() (string, error) {
		atomic.AddInt32(&calls, 1)
		return <-c, nil
	}

	const n = 10
	var wg sync.WaitGroup
	// 同时加1 最终的结果只能是 1
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			v, err := g.Do("key", fn)
			if err != nil {
				log.Fatalf("Do error: %v", err)
			}
			if v != "bar" {
				log.Fatalf("got %q; want %q", v, "bar")
			}
			wg.Done()
		}()
	}
	time.Sleep(100 * time.Millisecond) // let goroutines above block
	c <- "bar"
	wg.Wait()
	if got := atomic.LoadInt32(&calls); got != 1 {
		log.Fatalf("number of calls = %d; want 1", got)
	}
	log.Printf("done %v", calls)
}
```
