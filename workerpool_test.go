package workerpool

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

const max = 20

func init() {
	// configure logging
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
}

func TestExample(t *testing.T) {
	// t.Parallel()

	wp := New(context.TODO(), 2)
	requests := []string{"1", "2", "3", "4", "5"}

	rspChan := make(chan string, len(requests))
	for _, r := range requests {
		r := r
		wp.Submit(func() error {
			time.Sleep(time.Millisecond)
			rspChan <- r
			return nil
		})
	}

	err := wp.Wait()
	assert.Nil(t, err)
	close(rspChan)

	rspMap := map[string]bool{}
	for rsp := range rspChan {
		rspMap[rsp] = true
	}

	assert.Equal(t, len(requests), len(rspMap), "did not handle all requests")
	for _, req := range requests {
		if _, ok := rspMap[req]; !ok {
			t.Fatal("Missing expected value: ", req)
		}
	}
}

func TestErrorPropagated(t *testing.T) {
	// t.Parallel()

	wp := New(context.TODO(), 2)
	requests := []string{"1", "2", "3", "4", "5"}

	rspChan := make(chan string, len(requests))
	for _, r := range requests {
		r := r
		wp.Submit(func() error {
			time.Sleep(time.Millisecond)
			rspChan <- r
			return errors.New("someerror")
		})
	}

	err := wp.Wait()
	assert.NotNil(t, err)
}

func TestContextCancel(t *testing.T) {
	// t.Parallel()

	// context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	wp := New(ctx, max)
	requests := []string{"1", "2", "3", "4", "5"}

	// Create tasks, cancel in the middle
	rspChan := make(chan string, len(requests))
	for i, r := range requests {
		if i == 3 {
			cancel()
		}
		r := r
		wp.Submit(func() error {
			time.Sleep(time.Millisecond)
			rspChan <- r
			return nil
		})
	}

	// wait for queue to finish
	err := wp.Wait()
	assert.Nil(t, err)
	close(rspChan)

	rspMap := map[string]bool{}
	for rsp := range rspChan {
		rspMap[rsp] = true
	}

	assert.Equal(t, 3, len(rspMap), "should only have completed 3")

}

func TestWorkerTimeout(t *testing.T) {
	// t.Parallel()

	wp := New(context.TODO(), max)
	defer wp.Stop()

	var started sync.WaitGroup
	started.Add(max)
	release := make(chan struct{})

	// Cause workers to be created.  Workers wait on channel, keeping them busy
	// and causing the worker pool to create more.
	for i := 0; i < max; i++ {
		wp.Submit(func() error {
			started.Done()
			<-release
			return nil
		})
	}

	// Wait for tasks to start.
	started.Wait()

	if anyReady(wp) {
		t.Fatal("number of ready workers should be zero")
	}

	if wp.killIdleWorker() {
		t.Fatal("should have been no idle workers to kill")
	}

	// Release workers.
	close(release)

	if countReady(wp) != max {
		t.Fatal("Expected", max, "ready workers")
	}

	// Check that a worker timed out.
	time.Sleep(idleTimeout*2 + idleTimeout/2)
	if countReady(wp) != max-1 {
		t.Fatal("First worker did not timeout")
	}

	// Check that another worker timed out.
	time.Sleep(idleTimeout)
	if countReady(wp) != max-2 {
		t.Fatal("Second worker did not timeout")
	}
}

func TestResubmit(t *testing.T) {

	// create pool
	wp := New(context.TODO(), 2)
	requests := []string{"1", "2", "3", "4", "5"}

	// create response channel
	rspChan := make(chan string)

	// create sync group
	// waits for all tasks to finish before closing task channel
	var wg sync.WaitGroup

	// submit tasks
	for _, r := range requests {
		// increment wait group
		wg.Add(1)

		// capture
		r := r

		// submit task
		wp.Submit(func() error {

			// decrement wait group when done
			defer wg.Done()
			time.Sleep(time.Millisecond)
			rspChan <- r

			// increment wait group for nested task
			wg.Add(1)
			wp.Submit(func() error {

				// decrement wait group when done
				defer wg.Done()
				rspChan <- r
				return nil
			})
			return nil
		})
	}

	// receive responses
	go func() {
		for {
			select {
			case _, ok := <-rspChan:
				if !ok {
					return
				}
			}
		}
	}()

	// wait for all tasks to be done
	wg.Wait()

	// close
	err := wp.Wait()
	assert.Nil(t, err)
	close(rspChan)
}
