package workerpool

import (
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

func (p *WorkerPool) dispatch() {

	timeout := time.NewTimer(idleTimeout)

Loop:
	for {
		log.Trace().Msg("dispatch: Looping")
		// Tasks are in the waiting queue
		if p.waitingQueue.Len() != 0 {
			log.Trace().Msg("dispatch: waiting queue not empty")
			select {
			// Received done from context, break
			case <-p.context.Done():
				log.Trace().Msg("dispatch: received done from context")
				break Loop
			// Received task from queue
			case task, ok := <-p.taskQueue:
				if !ok {
					log.Trace().Msg("dispatch: task queue closed")
					break Loop
				}
				log.Trace().Msg("dispatch: pushing task onto waiting queue")
				p.waitingQueue.PushBack(task)
			// A worker is ready
			case p.workerQueue <- p.waitingQueue.Front().(func() error):
				log.Trace().Msg("dispatch: Process waiting queue's task")
				p.waitingQueue.PopFront()
			}
			continue
		}

		log.Trace().Msg("dispatch: waiting queue empty")
		select {

		// Received done from context, break
		case <-p.context.Done():
			log.Trace().Msg("dispatch: received done from context")
			break Loop

		// Received task from queue
		case task, ok := <-p.taskQueue:
			if !ok {
				log.Trace().Msg("dispatch: task queue closed")
				break Loop
			}
			log.Trace().Msg("dispatch: got a task")
			// Got a task to do.
			select {
			// A worker is ready
			case p.workerQueue <- task:
				log.Trace().Msg("dispatch: push task into worker queue")
			default:
				// Create a new worker, if not at max.
				if p.workerCount < p.maxWorkers {
					log.Trace().Msg("dispatch: creating a new worker")
					p.errgroup.Go(func() error { return p.startWorker(p.errgroup, task, p.workerQueue) })
					p.workerCount++
				} else {
					log.Trace().Msg("dispatch: pushing task into waiting queue")
					// Enqueue task to be executed by next available worker.
					p.waitingQueue.PushBack(task)
				}
			}
			p.idle = false

		// Timed out waiting for work to arrive.  Kill a ready worker if
		// pool has been idle for a whole timeout.
		case <-timeout.C:
			log.Trace().Msg("dispatch: timed out")
			if p.idle && p.workerCount > 0 {
				if p.killIdleWorker() {
					p.workerCount--
				}
			}
			p.idle = true
			timeout.Reset(idleTimeout)
		}
	}

	log.Trace().Msg("dispatch: done")
	close(p.workerQueue)
	if err := p.errgroup.Wait(); err != nil {
		p.errChan <- err
		close(p.errChan)
	} else {
		close(p.doneChan)
	}

	timeout.Stop()
}

// startWorker runs initial task, then starts a worker waiting for more.
func (p *WorkerPool) startWorker(errGroup *errgroup.Group, task func() error, workerQueue chan func() error) error {

	defer p.waitgroup.Done()

	log.Trace().Msg("worker (startWorker): Performing task")
	if err := task(); err != nil {
		log.Trace().Msg("worker (startWorker): got error")
		return err
	}

	errGroup.Go(func() error {
		return p.worker(workerQueue)
	})

	return nil
}

// worker executes tasks and stops when it receives a nil task.
func (p *WorkerPool) worker(workerQueue chan func() error) error {
	for task := range workerQueue {
		if task == nil {
			log.Trace().Msg("worker: got kill order")
			return nil
		}

		log.Trace().Msg("worker: Got task, performing task")
		if err := task(); err != nil {
			log.Trace().Msg("worker: got error")
			return err
		}

		p.waitgroup.Done()
	}
	return nil
}

func (p *WorkerPool) killIdleWorker() bool {
	select {
	case p.workerQueue <- nil:
		// Sent kill signal to worker.
		return true
	default:
		// No ready workers.  All, if any, workers are busy.
		return false
	}
}
