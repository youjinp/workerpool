package workerpool

import (
	"context"
	"sync"
	"time"

	"github.com/gammazero/deque"
	"golang.org/x/sync/errgroup"
)

const (
	// If workes idle for at least this period of time, then stop a worker.
	idleTimeout = 2 * time.Second
)

// WorkerPool is a collection of goroutines, where the number of concurrent
// goroutines processing requests does not exceed the specified maximum.
type WorkerPool struct {
	waitgroup    *sync.WaitGroup
	errgroup     *errgroup.Group
	maxWorkers   int
	taskQueue    chan func() error
	workerQueue  chan func() error
	errChan      chan error
	doneChan     chan struct{}
	waitingQueue deque.Deque
	context      context.Context
}

// New creates and starts a pool of worker goroutines.
//
// The maxWorkers parameter specifies the maximum number of workers that can
// execute tasks concurrently.  When there are no incoming tasks, workers are
// gradually stopped until there are no remaining workers.
func New(ctx context.Context, maxWorkers int) *WorkerPool {

	// There must be at least one worker.
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	// error group
	g, ctx := errgroup.WithContext(ctx)

	pool := &WorkerPool{
		waitgroup:   &sync.WaitGroup{},
		errgroup:    g,
		maxWorkers:  maxWorkers,
		taskQueue:   make(chan func() error),
		workerQueue: make(chan func() error),
		errChan:     make(chan error),
		doneChan:    make(chan struct{}),
		context:     ctx,
	}

	// Start the task dispatcher.
	go pool.dispatch()

	return pool
}

// Submit enqueues a function for a worker to execute.
//
// Any external values needed by the task function must be captured in a
// closure.  Any return values should be returned over a channel that is
// captured in the task function closure.
//
// Submit will not block regardless of the number of tasks submitted.  Each
// task is immediately given to an available worker or to a newly started
// worker.  If there are no available workers, and the maximum number of
// workers are already created, then the task is put onto a waiting queue.
//
// When there are tasks on the waiting queue, any additional new tasks are put
// on the waiting queue.  Tasks are removed from the waiting queue as workers
// become available.
//
// As long as no new tasks arrive, one available worker is shutdown each time
// period until there are no more idle workers.  Since the time to start new
// goroutines is not significant, there is no need to retain idle workers
// indefinitely.
func (p *WorkerPool) Submit(task func() error) {
	p.waitgroup.Add(1)
	if task != nil {
		select {
		case <-p.context.Done():
		case p.taskQueue <- task:
		}
	}
}

// Stop stops the worker pool and waits for only currently running tasks to
// complete.  Pending tasks that are not currently running are abandoned.
// Tasks must not be submitted to the worker pool after calling stop.
//
// Since creating the worker pool starts at least one goroutine, for the
// dispatcher, Stop() or Wait() should be called when the worker pool is no
// longer needed.
func (p *WorkerPool) Stop() error {
	// close task queue
	close(p.taskQueue)

	// receive error (if any) from dispatch
	select {
	case err, ok := <-p.errChan:
		// return received error
		if ok {
			return err
		}
		// no error
	case <-p.doneChan:
	}

	return nil
}

// Wait stops the worker pool and waits for a signal from either the done
// channel or the error channel.
func (p *WorkerPool) Wait() error {

	waitChan := make(chan bool)
	go func() {
		p.waitgroup.Wait()
		close(waitChan)
	}()

	// wait until all tasks finished, or received done from context
	select {
	case <-waitChan:
	case <-p.context.Done():
	}

	// cloase task queue
	close(p.taskQueue)

	// receive error (if any) from dispatch
	select {
	case err, ok := <-p.errChan:
		// return received error
		if ok {
			return err
		}
		// no error
	case <-p.doneChan:
	}

	return nil
}
