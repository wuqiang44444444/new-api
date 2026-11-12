package common

import (
	"context"
	"time"
)

type mailSubmissionObserverKey struct{}

func WithMailSubmissionObserver(ctx context.Context, observer func(time.Time)) context.Context {
	return context.WithValue(ctx, mailSubmissionObserverKey{}, observer)
}

func observeMailSubmission(ctx context.Context) {
	if observer, ok := ctx.Value(mailSubmissionObserverKey{}).(func(time.Time)); ok {
		observer(time.Now())
	}
}
