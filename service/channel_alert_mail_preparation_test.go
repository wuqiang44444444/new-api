package service

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChannelAlertMailSkipsSecondClaimWithoutNewDelivery(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ChannelAlertDelivery{}))
	withErrorReportSetting(t, "")
	var claims, preparationReads int
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(t.Name(), func(tx *gorm.DB) {
		if tx.Statement.Table == "channel_alert_deliveries" {
			claims++
		}
	}))
	require.NoError(t, db.Callback().Row().After("gorm:row").Register(t.Name(), func(tx *gorm.DB) {
		if tx.Statement.Table == "channel_alert_episodes" {
			preparationReads++
		}
	}))
	for range 3 {
		processed, err := deliverChannelAlertMail("idle-worker", nil, time.Now)
		require.NoError(t, err)
		assert.False(t, processed)
	}
	assert.Equal(t, 3, claims, "each empty worker pass only needs one claim")
	assert.Equal(t, 3, preparationReads, "each worker must still check for unprepared alerts")

	episode := model.ChannelAlertEpisode{Kind: "upstream_balance", MailPending: true}
	require.NoError(t, db.Create(&episode).Error)
	processed, err := deliverChannelAlertMail("idle-worker", nil, time.Now)
	require.NoError(t, err)
	assert.False(t, processed)
	assert.Equal(t, 4, claims, "preparing an alert without recipients creates no delivery to claim")
	require.NoError(t, db.First(&episode, episode.ID).Error)
	assert.False(t, episode.MailPending)
}

func TestChannelAlertMailConcurrentPreparationDeliversOnce(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ChannelAlertDelivery{}))
	conn, err := db.DB()
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	withErrorReportSetting(t, "ops@example.com")
	now := time.Now()
	episode := model.ChannelAlertEpisode{Kind: "upstream_balance", ChannelName: "fixture", FirstAt: now.UnixMilli(), MailPending: true}
	require.NoError(t, db.Create(&episode).Error)
	var sent atomic.Int32
	send := func(context.Context, string, string, string) error {
		sent.Add(1)
		return nil
	}
	start, done := make(chan struct{}), make(chan error, 3)
	for i := range 3 {
		go func() {
			<-start
			_, err := deliverChannelAlertMail(fmt.Sprintf("worker-%d", i), send, func() time.Time { return now })
			done <- err
		}()
	}
	close(start)
	for range 3 {
		require.NoError(t, <-done)
	}
	assert.EqualValues(t, 1, sent.Load())
	var deliveries []model.ChannelAlertDelivery
	require.NoError(t, db.Find(&deliveries).Error)
	require.Len(t, deliveries, 1)
	assert.Equal(t, episode.ID, deliveries[0].EpisodeID)
	assert.Equal(t, model.MailDeliveryAccepted, deliveries[0].Status)
}

func TestChannelAlertMailClaimContentionKeepsBothWorkersDelivering(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ChannelAlertDelivery{}))
	conn, err := db.DB()
	require.NoError(t, err)
	conn.SetMaxOpenConns(1)
	require.NoError(t, db.Create(&model.Option{Key: "error_report_setting.recipients", Value: "ops@example.com"}).Error)
	now := time.Now()
	episodes := []model.ChannelAlertEpisode{
		{Kind: "upstream_balance", ChannelName: "first", ConfirmedAt: now.UnixMilli()},
		{Kind: "upstream_balance", ChannelName: "second", ConfirmedAt: now.UnixMilli()},
	}
	require.NoError(t, db.Create(&episodes).Error)
	deliveries := []model.ChannelAlertDelivery{
		{EpisodeID: episodes[0].ID, Recipient: "ops@example.com", Status: model.MailDeliveryPending, QueuedAt: now.UnixNano()},
		{EpisodeID: episodes[1].ID, Recipient: "ops@example.com", Status: model.MailDeliveryPending, QueuedAt: now.UnixNano()},
	}
	require.NoError(t, db.Create(&deliveries).Error)

	// Force both workers to read the same oldest delivery before either can update it.
	var claims atomic.Int32
	selected := make(chan int64, 2)
	bothSelected := make(chan struct{})
	require.NoError(t, db.Callback().Query().After("gorm:query").Register(t.Name(), func(tx *gorm.DB) {
		delivery, ok := tx.Statement.Dest.(*model.ChannelAlertDelivery)
		if !ok || tx.Statement.Table != "channel_alert_deliveries" {
			return
		}
		switch claims.Add(1) {
		case 1:
			selected <- delivery.ID
			select {
			case <-bothSelected:
			case <-tx.Statement.Context.Done():
				tx.AddError(tx.Statement.Context.Err())
			}
		case 2:
			selected <- delivery.ID
			close(bothSelected)
		}
	}))
	var sent atomic.Int32
	send := func(context.Context, string, string, string) error {
		sent.Add(1)
		return nil
	}
	type outcome struct {
		processed bool
		err       error
	}
	done := make(chan outcome, 2)
	for i := range 2 {
		go func() {
			processed, err := deliverChannelAlertMail(fmt.Sprintf("worker-%d", i), send, func() time.Time { return now })
			done <- outcome{processed, err}
		}()
	}
	first, second := <-done, <-done
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	require.Len(t, selected, 2)
	assert.Equal(t, deliveries[0].ID, <-selected)
	assert.Equal(t, deliveries[0].ID, <-selected)
	assert.True(t, first.processed)
	assert.True(t, second.processed, "the losing worker should take the next ready delivery in the same pass")
	assert.EqualValues(t, 3, claims.Load(), "only the losing worker needs one additional claim")
	assert.EqualValues(t, 2, sent.Load())
	require.NoError(t, db.Order("id").Find(&deliveries).Error)
	require.Len(t, deliveries, 2)
	for _, delivery := range deliveries {
		assert.Equal(t, model.MailDeliveryAccepted, delivery.Status)
		assert.Equal(t, 1, delivery.Attempts)
	}
}

func TestChannelAlertMailKeepsDueRetryAndRecoversPartialPreparation(t *testing.T) {
	db := withErrorReportTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.ChannelAlertDelivery{}))
	withErrorReportSetting(t, "ops@example.com")
	now := time.Now()
	episodes := []model.ChannelAlertEpisode{
		{Kind: "upstream_balance", ChannelName: "due-retry", FirstAt: now.UnixMilli(), ConfirmedAt: now.UnixMilli()},
		{Kind: "upstream_balance", ChannelName: "committed", FirstAt: now.UnixMilli(), ConfirmedAt: now.UnixMilli(), MailPending: true},
		{Kind: "upstream_balance", ChannelName: "uncommitted", FirstAt: now.UnixMilli(), ConfirmedAt: now.UnixMilli(), MailPending: true},
	}
	require.NoError(t, db.Create(&episodes).Error)
	due := model.ChannelAlertDelivery{EpisodeID: episodes[0].ID, Recipient: "ops@example.com",
		Status: model.MailDeliveryRetryWait, NextAttemptAt: now.UnixMilli(), QueuedAt: now.UnixNano()}
	require.NoError(t, db.Create(&due).Error)
	send := func(context.Context, string, string, string) error { return nil }
	clock := func() time.Time { return now }
	processed, err := deliverChannelAlertMail("worker", send, clock)
	require.NoError(t, err)
	require.True(t, processed)
	require.NoError(t, db.First(&due, due.ID).Error)
	assert.Equal(t, model.MailDeliveryAccepted, due.Status)
	var count int64
	require.NoError(t, db.Model(&model.ChannelAlertDelivery{}).Count(&count).Error)
	assert.EqualValues(t, 1, count, "a ready retry is delivered before preparing new alerts")

	prepareErr := errors.New("fixture failed to queue the second alert")
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register(t.Name(), func(tx *gorm.DB) {
		rows, ok := tx.Statement.Dest.(*[]model.ChannelAlertDelivery)
		if ok && len(*rows) > 0 && (*rows)[0].EpisodeID == episodes[2].ID {
			_ = tx.AddError(prepareErr)
		}
	}))
	processed, err = deliverChannelAlertMail("worker", send, clock)
	require.ErrorIs(t, err, prepareErr)
	assert.False(t, processed)
	require.NoError(t, db.First(&episodes[1], episodes[1].ID).Error)
	require.NoError(t, db.First(&episodes[2], episodes[2].ID).Error)
	assert.False(t, episodes[1].MailPending, "the first alert committed independently")
	assert.True(t, episodes[2].MailPending, "the failed alert remains eligible for retry")
	require.NoError(t, db.Callback().Create().Remove(t.Name()))

	for range 2 {
		processed, err = deliverChannelAlertMail("next-worker", send, clock)
		require.NoError(t, err)
		require.True(t, processed)
	}
	var deliveries []model.ChannelAlertDelivery
	require.NoError(t, db.Order("id").Find(&deliveries).Error)
	require.Len(t, deliveries, len(episodes))
	for i, delivery := range deliveries {
		assert.Equal(t, episodes[i].ID, delivery.EpisodeID)
		assert.Equal(t, model.MailDeliveryAccepted, delivery.Status)
	}
}
