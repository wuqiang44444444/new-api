package service

import (
	"context"
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

var errMailPreferenceDisabled = errors.New("mail preference disabled")

// The queue and transport stay independent of notification policy. Every
// managed email crosses this boundary, including mail already claimed by a worker.
func submitManagedMail(ctx context.Context, send mailSendFunc, kind model.MailType, queuedAt time.Time, subject, address, body string) error {
	guard := func(ctx context.Context) error {
		prefs, err := model.LoadMailPreferences(ctx)
		if err != nil {
			return err
		}
		if !prefs.Allows(kind, queuedAt) {
			return errMailPreferenceDisabled
		}
		return nil
	}
	if err := guard(ctx); err != nil {
		return err
	}
	return send(common.WithMailSubmissionGuard(ctx, guard), subject, address, body)
}

func sendManagedNotice(kind model.MailType, queuedAt time.Time, subject, address, body string) error {
	ctx, cancel := context.WithTimeout(context.Background(), common.ShortMailSubmissionTimeout)
	defer cancel()
	err := submitManagedMail(ctx, common.SendEmailContext, kind, queuedAt, subject, address, body)
	if errors.Is(err, errMailPreferenceDisabled) {
		return nil
	}
	return err
}
