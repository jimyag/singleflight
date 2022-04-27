package singleflight

import "sync"

type (

	// SingleFlight
	// 可以将对同一个 Key 的并发请求进行合并，只让其中一个请求到数据库进行查询，其他请求共享同一个结果，可以很大程度提升并发能力
	// 定义 call 的结构
	call[T any] struct {
		wg  sync.WaitGroup // 用于实现通过1个 call，其他 call 阻塞
		val T              // 表示 call 操作的返回结果
		err error          // 表示 call 操作发生的错误
	}

	// 总控结构，实现 SingleFlight 接口
	flightGroup[T any] struct {
		calls map[string]*call[T] // 不同的 call 对应不同的 key
		lock  sync.Mutex          // 利用锁控制请求
	}
)

// NewSingleFlight returns a SingleFlight.
func NewSingleFlight[T any]() *flightGroup[T] {
	return &flightGroup[T]{
		calls: make(map[string]*call[T]),
	}
}

func (g *flightGroup[T]) Do(key string, fn func() (T, error)) (T, error) {
	// 对 key 发起 call 请求（其实就是做一件事情），
	// 如果此时已经有其他协程已经在发起 call 请求就阻塞住（done 为 true 的情况），
	// 等待拿到结果后直接返回
	c, done := g.createCall(key)
	if done {
		return c.val, c.err
	}
	// 如果 done 是 false，说明当前协程是第一个发起 call 的协程，
	// 那么就执行 g.makeCall(c, key, fn)
	// 真正地发起 call 请求（此后的其他协程就阻塞在了 g.createCall(key))
	g.makeCall(c, key, fn)
	return c.val, c.err
}

func (g *flightGroup[T]) DoEx(key string, fn func() (T, error)) (val T, fresh bool, err error) {
	c, done := g.createCall(key)
	if done {
		return c.val, false, c.err
	}

	g.makeCall(c, key, fn)
	return c.val, true, c.err
}

func (g *flightGroup[T]) createCall(key string) (c *call[T], done bool) {
	g.lock.Lock()
	// 先看第一步：判断是第一个请求的协程（利用 map）此处判断 map 中的 key 是否存在，
	// 如果已经存在，说明已经有其他协程在请求了，
	// 当前这个协程只需要等待，等待是利用了 sync.WaitGroup 的 Wait() 方法实现的，此处还是很巧妙的
	if c, ok := g.calls[key]; ok {
		g.lock.Unlock()
		c.wg.Wait()
		return c, true
	}

	// 如果是第一个发起 call 的协程，所以需要 new 这个 call，然后将 wg.Add(1)，
	// 这样就对应了上面的 wg.Wait()，阻塞剩下的协程。
	// 随后将 new 的 call 放入 map 中。
	// 注意此时只是完成了初始化，并没有真正去执行 call 请求，
	// 真正的处理逻辑在 g.makeCall(c, key, fn) 中。
	c = new(call[T])
	c.wg.Add(1)
	g.calls[key] = c
	g.lock.Unlock()

	return c, false
}

func (g *flightGroup[T]) makeCall(c *call[T], key string, fn func() (T, error)) {
	// 这个方法中做的事情很简单，就是执行了传递的匿名函数 fn()（也就是真正 call 请求要做的事情）。最后处理收尾的事情（通过 defer），也是分成两步：
	//
	//删除 map 中的 key，使得下次发起请求可以获取新的值。
	//调用 wg.Done()，让之前阻塞的协程全部获得结果并返回。
	defer func() {
		g.lock.Lock()
		delete(g.calls, key)
		g.lock.Unlock()
		c.wg.Done()
	}()

	c.val, c.err = fn()
}
