// created by jmxyyy

package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPoolSizeAdjustment(t *testing.T) {
	pool := NewFunc(10, func(interface{}) interface{} { return "foo" })
	if exp, act := 10, len(pool.workers); exp != act {
		t.Errorf("Wrong size of pool: %v != %v", act, exp)
	}

	pool.SetSize(10)
	if exp, act := 10, pool.GetSize(); exp != act {
		t.Errorf("Wrong size of pool: %v != %v", act, exp)
	}

	pool.SetSize(9)
	if exp, act := 9, pool.GetSize(); exp != act {
		t.Errorf("Wrong size of pool: %v != %v", act, exp)
	}

	pool.SetSize(10)
	if exp, act := 10, pool.GetSize(); exp != act {
		t.Errorf("Wrong size of pool: %v != %v", act, exp)
	}

	pool.SetSize(0)
	if exp, act := 0, pool.GetSize(); exp != act {
		t.Errorf("Wrong size of pool: %v != %v", act, exp)
	}

	pool.SetSize(10)
	if exp, act := 10, pool.GetSize(); exp != act {
		t.Errorf("Wrong size of pool: %v != %v", act, exp)
	}

	// Finally, make sure we still have actual active workers.
	if exp, act := "foo", pool.Process(0).(string); exp != act {
		t.Errorf("Wrong result: %v != %v", act, exp)
	}

	pool.Close()
	if exp, act := 0, pool.GetSize(); exp != act {
		t.Errorf("Wrong size of pool: %v != %v", act, exp)
	}
}

func TestFuncJob(t *testing.T) {
	pool := NewFunc(10, func(in interface{}) interface{} {
		intVal := in.(int)
		return intVal * 2
	})
	defer pool.Close()

	for i := 0; i < 10; i++ {
		ret := pool.Process(10)
		if exp, act := 20, ret.(int); exp != act {
			t.Errorf("Wrong result: %v != %v", act, exp)
		}
	}
}

func TestFuncJobTimed(t *testing.T) {
	pool := NewFunc(10, func(in interface{}) interface{} {
		intVal := in.(int)
		return intVal * 2
	})
	defer pool.Close()

	for i := 0; i < 10; i++ {
		ret, err := pool.ProcessTimed(10, time.Millisecond)
		if err != nil {
			t.Fatalf("Failed to process: %v", err)
		}
		if exp, act := 20, ret.(int); exp != act {
			t.Errorf("Wrong result: %v != %v", act, exp)
		}
	}
}

func TestFuncJobCtx(t *testing.T) {
	t.Run("Completes when ctx not canceled", func(t *testing.T) {
		pool := NewFunc(10, func(in interface{}) interface{} {
			intVal := in.(int)
			return intVal * 2
		})
		defer pool.Close()

		for i := 0; i < 10; i++ {
			ret, err := pool.ProcessCtx(10, context.Background())
			if err != nil {
				t.Fatalf("Failed to process: %v", err)
			}
			if exp, act := 20, ret.(int); exp != act {
				t.Errorf("Wrong result: %v != %v", act, exp)
			}
		}
	})

	t.Run("Returns err when ctx canceled", func(t *testing.T) {
		pool := NewFunc(1, func(in interface{}) interface{} {
			intVal := in.(int)
			<-time.After(time.Millisecond)
			return intVal * 2
		})
		defer pool.Close()

		ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
		defer cancel()
		_, act := pool.ProcessCtx(10, ctx)
		if exp := context.DeadlineExceeded; !errors.Is(exp, act) {
			t.Errorf("Wrong error returned: %v != %v", act, exp)
		}
	})
}

func TestCallbackJob(t *testing.T) {
	pool := NewCallback(10)
	defer pool.Close()

	var counter int64 = 0
	for i := 0; i < 10; i++ {
		ret := pool.Process(func() {
			atomic.AddInt64(&counter, 1)
		})
		if ret != nil {
			t.Errorf("Non-nil callback response: %v", ret)
		}
	}

	ret := pool.Process("foo")
	if exp, act := ErrorJobNotFunc, ret; exp != act {
		t.Errorf("Wrong result from non-func: %v != %v", act, exp)
	}

	if exp, act := int64(10), counter; exp != act {
		t.Errorf("Wrong result: %v != %v", act, exp)
	}
}

func TestTimeout(t *testing.T) {
	pool := NewFunc(1, func(in interface{}) interface{} {
		intVal := in.(int)
		<-time.After(time.Millisecond)
		return intVal * 2
	})
	defer pool.Close()

	_, act := pool.ProcessTimed(10, time.Duration(1))
	if exp := ErrorJobTimeOut; !errors.Is(exp, act) {
		t.Errorf("Wrong error returned: %v != %v", act, exp)
	}
}

func TestTimedJobsAfterClose(t *testing.T) {
	pool := NewFunc(1, func(in interface{}) interface{} {
		return 1
	})
	pool.Close()

	_, act := pool.ProcessTimed(10, time.Duration(10*time.Millisecond))
	if exp := ErrorPoolNotRunning; !errors.Is(exp, act) {
		t.Errorf("Wrong error returned: %v != %v", act, exp)
	}
}

func TestJobsAfterClose(t *testing.T) {
	pool := NewFunc(1, func(in interface{}) interface{} {
		return 1
	})
	pool.Close()

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Process after Stop() did not panic")
		}
	}()

	pool.Process(10)
}

func TestParallelJobs(t *testing.T) {
	nWorkers := 10

	jobGroup := sync.WaitGroup{}
	testGroup := sync.WaitGroup{}

	pool := NewFunc(nWorkers, func(in interface{}) interface{} {
		jobGroup.Done()
		jobGroup.Wait()

		intVal := in.(int)
		return intVal * 2
	})
	defer pool.Close()

	for j := 0; j < 1; j++ {
		jobGroup.Add(nWorkers)
		testGroup.Add(nWorkers)

		for i := 0; i < nWorkers; i++ {
			go func() {
				ret := pool.Process(10)
				if exp, act := 20, ret.(int); exp != act {
					t.Errorf("Wrong result: %v != %v", act, exp)
				}
				testGroup.Done()
			}()
		}

		testGroup.Wait()
	}
}

type mockWorker struct {
	blockProcChan  chan struct{}
	blockReadyChan chan struct{}
	interruptChan  chan struct{}
	terminated     bool
}

func (m *mockWorker) Process(in interface{}) interface{} {
	select {
	case <-m.blockProcChan:
	case <-m.interruptChan:
	}
	return in
}

func (m *mockWorker) BlockUntilReady() {
	<-m.blockReadyChan
}

func (m *mockWorker) Interrupt() {
	m.interruptChan <- struct{}{}
}

func (m *mockWorker) Terminate() {
	m.terminated = true
}

func TestCustomWorker(t *testing.T) {
	pool := NewPool(1, func() Worker {
		return &mockWorker{
			blockProcChan:  make(chan struct{}),
			blockReadyChan: make(chan struct{}),
			interruptChan:  make(chan struct{}),
		}
	})

	worker1, ok := pool.workers[0].worker.(*mockWorker)
	if !ok {
		t.Fatal("Wrong type of worker in pool")
	}

	if worker1.terminated {
		t.Fatal("Worker started off terminated")
	}

	_, err := pool.ProcessTimed(10, time.Millisecond)
	if exp, act := ErrorJobTimeOut, err; !errors.Is(exp, act) {
		t.Errorf("Wrong error: %v != %v", act, exp)
	}

	close(worker1.blockReadyChan)
	_, err = pool.ProcessTimed(10, time.Millisecond)
	if exp, act := ErrorJobTimeOut, err; !errors.Is(exp, act) {
		t.Errorf("Wrong error: %v != %v", act, exp)
	}

	close(worker1.blockProcChan)
	if exp, act := 10, pool.Process(10).(int); exp != act {
		t.Errorf("Wrong result: %v != %v", act, exp)
	}

	pool.Close()
	if !worker1.terminated {
		t.Fatal("Worker was not terminated")
	}
}

func BenchmarkFuncJob(b *testing.B) {
	pool := NewFunc(10, func(in interface{}) interface{} {
		intVal := in.(int)
		return intVal * 2
	})
	defer pool.Close()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ret := pool.Process(10)
		if exp, act := 20, ret.(int); exp != act {
			b.Errorf("Wrong result: %v != %v", act, exp)
		}
	}
}

func BenchmarkFuncTimedJob(b *testing.B) {
	pool := NewFunc(10, func(in interface{}) interface{} {
		intVal := in.(int)
		return intVal * 2
	})
	defer pool.Close()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		ret, err := pool.ProcessTimed(10, time.Second)
		if err != nil {
			b.Error(err)
		}
		if exp, act := 20, ret.(int); exp != act {
			b.Errorf("Wrong result: %v != %v", act, exp)
		}
	}
}
