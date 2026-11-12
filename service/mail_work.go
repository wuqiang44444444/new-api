package service

import (
	"context"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

type mailWorkQueue struct {
	jobs    chan func()
	workers int
	start   sync.Once
}

var mailWork = mailWorkQueue{jobs: make(chan func(), 64), workers: 2}

// EnqueueMailWork reserves a bounded background slot without blocking the caller.
func EnqueueMailWork(work func()) bool {
	return mailWork.enqueue(work)
}

func (q *mailWorkQueue) enqueue(work func()) bool {
	q.start.Do(func() {
		for range q.workers {
			go func() {
				for work := range q.jobs {
					runMailWork(work)
				}
			}()
		}
	})
	select {
	case q.jobs <- work:
		return true
	default:
		return false
	}
}

func runMailWork(work func()) {
	defer func() {
		if recover() != nil {
			common.SysError("mail worker recovered from a failed job")
		}
	}()
	work()
}

// Durable operations mail retries definitive submission failures with bounded backoff.
func mailDeliveryBackoff(attempts int) int64 {
	steps := [...]int64{60, 300, 900, 3600}
	return steps[min(max(attempts, 1), len(steps))-1]
}

type mailSendFunc func(ctx context.Context, subject, receiver, body string) error
