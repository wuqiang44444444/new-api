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

type mailSubmissionGuardKey struct{}

// WithMailSubmissionGuard checks policy after transport authentication and just before submission.
func WithMailSubmissionGuard(ctx context.Context, guard func(context.Context) error) context.Context {
	return context.WithValue(ctx, mailSubmissionGuardKey{}, guard)
}

func checkMailSubmission(ctx context.Context) error {
	if guard, ok := ctx.Value(mailSubmissionGuardKey{}).(func(context.Context) error); ok {
		return guard(ctx)
	}
	return nil
}
