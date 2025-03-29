// created by jmxyyy

package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrorJobNotFunc     = errors.New("worker has no func")
	ErrorJobTimeOut     = errors.New("job time out")
	ErrorWorkerClosed   = errors.New("worker was closed")
	ErrorPoolNotRunning = errors.New("")
)

type Pool struct {
	queuedJobs  int64            // 计数器
	constructor func() Worker    // 构造
	workers     []*workerWrapper // 协程包装器集合
	reqChan     chan workRequest // worker注册通道
	workerMutex sync.Mutex
}

func NewPool(n int, constructor func() Worker) *Pool {
	p := &Pool{
		constructor: constructor,
		reqChan:     make(chan workRequest),
	}
	p.SetSize(n)
	return p
}

func (p *Pool) SetSize(n int) {
	p.workerMutex.Lock()
	defer p.workerMutex.Unlock()

	// 扩容
	for i := len(p.workers); i < n; i++ {
		p.workers = append(p.workers, newWorkerWrapper(p.reqChan, p.constructor()))
	}

	// 缩容
	for i := n; i < len(p.workers); i++ {
		p.workers[i].stop()
		p.workers[i].join()
	}

	p.workers = p.workers[:n]
}

func (p *Pool) GetSize() int {
	p.workerMutex.Lock()
	defer p.workerMutex.Unlock()
	return len(p.workers)
}

func (p *Pool) Close() {
	p.SetSize(0)
	close(p.reqChan)
}

func (p *Pool) GetQueueLength() int64 {
	return atomic.LoadInt64(&p.queuedJobs)
}

func NewFunc(n int, f func(interface{}) interface{}) *Pool {
	return NewPool(n, func() Worker {
		return &closureWorker{
			processor: f,
		}
	})
}

func NewCallback(n int) *Pool {
	return NewPool(n, func() Worker {
		return &callbackWorker{}
	})
}

func (p *Pool) Process(payload interface{}) interface{} {
	atomic.AddInt64(&p.queuedJobs, 1)
	defer atomic.AddInt64(&p.queuedJobs, -1)
	req, open := <-p.reqChan
	if !open {
		panic(ErrorPoolNotRunning)
	}
	req.jobChan <- payload
	payload, open = <-req.retChan
	if !open {
		panic(ErrorWorkerClosed)
	}
	return payload
}

func (p *Pool) ProcessTimed(
	payload interface{},
	timeout time.Duration) (interface{}, error) {
	atomic.AddInt64(&p.queuedJobs, 1)
	defer atomic.AddInt64(&p.queuedJobs, -1)

	tout := time.NewTimer(timeout)
	var req workRequest
	var open bool
	select {
	case req, open = <-p.reqChan:
		if !open {
			return nil, ErrorPoolNotRunning
		}
	case <-tout.C:
		return nil, ErrorJobTimeOut
	}

	select {
	case req.jobChan <- payload:
	case <-tout.C:
		req.interruptFunc()
		return nil, ErrorJobTimeOut
	}

	select {
	case payload, open = <-req.retChan:
		if !open {
			return nil, ErrorWorkerClosed
		}
	case <-tout.C:
		req.interruptFunc()
		return nil, ErrorJobTimeOut
	}

	tout.Stop()
	return payload, nil
}

func (p *Pool) ProcessCtx(
	payload interface{},
	ctx context.Context) (interface{}, error) {
	atomic.AddInt64(&p.queuedJobs, 1)
	defer atomic.AddInt64(&p.queuedJobs, -1)

	var req workRequest
	var open bool
	select {
	case req, open = <-p.reqChan:
		if !open {
			return nil, ErrorPoolNotRunning
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case req.jobChan <- payload:
	case <-ctx.Done():
		req.interruptFunc()
		return nil, ctx.Err()
	}

	select {
	case payload, open = <-req.retChan:
		if !open {
			return nil, ErrorWorkerClosed
		}
	case <-ctx.Done():
		req.interruptFunc()
		return nil, ctx.Err()
	}

	return payload, nil
}
