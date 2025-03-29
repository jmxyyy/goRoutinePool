// created by jmxyyy

package main

type Worker interface {
	Process(interface{}) interface{} // 处理
	BlockUntilReady()                // 阻塞
	Interrupt()                      // 中断
	Terminate()                      // 终止
}

type closureWorker struct {
	processor func(interface{}) interface{}
}

func (w *closureWorker) Process(payload interface{}) interface{} {
	return w.processor(payload)
}

func (w *closureWorker) BlockUntilReady() {}
func (w *closureWorker) Interrupt()       {}
func (w *closureWorker) Terminate()       {}

type callbackWorker struct{}

func (w *callbackWorker) Process(payload interface{}) interface{} {
	f, ok := payload.(func())
	if !ok {
		return ErrorJobNotFunc
	}
	f()
	return nil
}

func (w *callbackWorker) BlockUntilReady() {}
func (w *callbackWorker) Interrupt()       {}
func (w *callbackWorker) Terminate()       {}

type workRequest struct {
	jobChan       chan<- interface{} // 发送任务
	retChan       <-chan interface{} // 接受worker返回结果
	interruptFunc func()             // 中断函数
}

type workerWrapper struct {
	worker        Worker
	interruptChan chan struct{}      // 中断信号通道
	reqChan       chan<- workRequest // 发送工作请求
	closeChan     chan struct{}      // 关闭信号
	closedChan    chan struct{}      // 协程已关闭信号
}

func newWorkerWrapper(reqChan chan<- workRequest, worker Worker) *workerWrapper {
	w := workerWrapper{
		worker:        worker,
		interruptChan: make(chan struct{}),
		reqChan:       reqChan,
		closeChan:     make(chan struct{}),
		closedChan:    make(chan struct{}),
	}
	go w.run()
	return &w
}

func (w *workerWrapper) run() {
	jobChan, retChan := make(chan interface{}), make(chan interface{})
	defer func() { // 销毁
		w.worker.Terminate()
		close(retChan)
		close(w.closedChan)
	}()

	for {
		w.worker.BlockUntilReady()
		select {
		case w.reqChan <- workRequest{ // 发送请求
			jobChan:       jobChan,
			retChan:       retChan,
			interruptFunc: w.interrupt,
		}:
			select {
			case payload := <-jobChan: // 接受任务
				res := w.worker.Process(payload)
				select {
				case retChan <- res:
				case <-w.interruptChan: // 中断
					w.interruptChan = make(chan struct{})
				}
			case <-w.interruptChan: // 外部中断
				w.interruptChan = make(chan struct{})
			}
		case <-w.closeChan: // 收到关闭信号
			return
		}
	}
}

func (w *workerWrapper) stop() {
	close(w.closeChan)
}

func (w *workerWrapper) join() {
	<-w.closedChan
}

func (w *workerWrapper) interrupt() {
	close(w.interruptChan)
	w.worker.Interrupt()
}
