package model

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/QuantumNous/new-api/clienterrlog"
	"gorm.io/gorm"
)

const CustomerWalletRejectionMail = "customer_wallet_rejection"
const WalletLowNoticeMail = "wallet_low_notice"

// WalletDigestAccount records inclusion in a prepared digest, independent
// of the source log's retention, arrival time or database.
type WalletDigestAccount struct {
	DayStart int64 `gorm:"primaryKey;autoIncrement:false"`
	UserID   int   `gorm:"primaryKey;autoIncrement:false"`
}

type WalletRejectionCount struct {
	UserID     int
	Rejections int64
}

func ReadWalletRejectionCounts(ctx context.Context, start, end int64) ([]WalletRejectionCount, error) {
	var counts []WalletRejectionCount
	err := LOG_DB.WithContext(ctx).Model(&ErrorEvent{}).
		Where("event_type = ? AND reason = ? AND user_id > 0 AND created_at >= ? AND created_at < ?", clienterrlog.EventAPIError, clienterrlog.WalletQuotaInsufficient, start, end).
		Select("user_id, COUNT(*) AS rejections").Group("user_id").Order("rejections DESC, user_id ASC").Scan(&counts).Error
	return counts, err
}

// SelectWalletDigestAccounts computes the pending membership inside the
// publication transaction, so it reads the same preferences as the writes after it.
func SelectWalletDigestAccounts(tx *gorm.DB, day int64, counts []WalletRejectionCount) ([]WalletRejectionCount, error) {
	next := make([]WalletRejectionCount, 0, len(counts))
	for start := 0; start < len(counts); start += 200 {
		batch := counts[start:min(start+200, len(counts))]
		ids := make([]int, 0, len(batch))
		for _, row := range batch {
			ids = append(ids, row.UserID)
		}
		var prior []int
		if err := tx.Model(&WalletDigestAccount{}).Where("day_start = ? AND user_id IN ?", day, ids).Pluck("user_id", &prior).Error; err != nil {
			return nil, err
		}
		seen := make(map[int]bool, len(prior))
		for _, id := range prior {
			seen[id] = true
		}
		for _, row := range batch {
			if !seen[row.UserID] {
				next = append(next, row)
			}
		}
	}
	return next, nil
}

// PublishWalletRejectionDigest freezes recipients and membership with a body
// prepared outside the transaction against that exact membership.
func PublishWalletRejectionDigest(ctx context.Context, day int64, end time.Time, changedAt int64, recipients []string, counts, prepared []WalletRejectionCount, episode ResourceAlertEpisode) error {
	if len(recipients) == 0 || len(prepared) == 0 {
		return nil
	}
	return DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		prefs, err := loadMailPreferences(tx)
		if err != nil || !prefs.Allows(MailWalletRejection, end) || prefs.Types[MailWalletRejection].ChangedAt != changedAt {
			return err
		}
		included, err := SelectWalletDigestAccounts(tx, day, counts)
		if err != nil {
			return err
		}
		if len(included) == 0 {
			return nil
		}
		if !slices.Equal(included, prepared) {
			return errors.New("wallet digest membership changed while preparing details")
		}
		opened, err := OpenResourceAlertInTx(tx, episode, recipients)
		if err != nil || !opened {
			return err
		}
		members := make([]WalletDigestAccount, 0, len(included))
		for _, row := range included {
			members = append(members, WalletDigestAccount{DayStart: day, UserID: row.UserID})
		}
		return tx.CreateInBatches(members, 200).Error
	})
}

func CloseWalletNotice(ctx context.Context, id, at int64) error {
	return DB.WithContext(ctx).Model(&ResourceAlertEpisode{}).
		Where("id = ? AND resource = ? AND closed_at = 0 AND open_key IS NOT NULL", id, WalletLowNoticeMail).
		Updates(map[string]any{"open_key": nil, "closed_at": at}).Error
}

func PruneWalletDigestAccounts(ctx context.Context, cutoff int64) error {
	db := DB.WithContext(ctx)
	var rows []WalletDigestAccount
	if err := db.Where("day_start < ?", cutoff).Order("day_start, user_id").Limit(500).Find(&rows).Error; err != nil || len(rows) == 0 {
		return err
	}
	last := rows[len(rows)-1]
	return db.Where("day_start < ? OR (day_start = ? AND user_id <= ?)", last.DayStart, last.DayStart, last.UserID).Delete(&WalletDigestAccount{}).Error
}
