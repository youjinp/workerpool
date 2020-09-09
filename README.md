# workerpool

Concurrency limiting goroutine pool. Limits the concurrency of task execution, not the number of tasks queued. Never blocks submitting tasks, no matter how many tasks are queued.

This implementation builds on the repo:

- https://github.com/gammazero/workerpool

Changes made:

- Add context support
- Update task to have an error return value

## Example

```go
package main

import (
    "fmt"
    "github.com/youjinp/workerpool"
)

func main() {
    wp := workerpool.New(context.TODO(), 2)
    requests := []string{"alpha", "beta", "gamma", "delta", "epsilon"}

    for _, r := range requests {
        r := r
        wp.Submit(func() error {
            fmt.Println("Handling request:", r)
            return nil
        })
    }

    if err := wp.Wait(); err != nil {
        log.Fatal(err)
    }
}
```
