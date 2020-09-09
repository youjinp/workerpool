package workerpool

import "time"

func anyReady(p *WorkerPool) bool {
	select {
	case p.workerQueue <- nil:
		go p.worker(p.workerQueue)
		return true
	default:
	}
	return false
}

func countReady(p *WorkerPool) int {
	// Try to stop max workers.
	timeout := time.After(100 * time.Millisecond)
	var readyCount int
	for i := 0; i < max; i++ {
		select {
		case p.workerQueue <- nil:
			readyCount++
		case <-timeout:
			i = max
		}
	}

	// Restore workers.
	for i := 0; i < readyCount; i++ {
		go p.worker(p.workerQueue)
	}
	return readyCount
}
