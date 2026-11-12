package model

import "context"

const overdueBillingRefundSeconds int64 = 86400

func CountOverdueBillingRefunds(ctx context.Context, userID int, start, end, asOf int64) (int64, error) {
	var count int64
	err := DB.WithContext(ctx).Model(&Task{}).
		Where("user_id = ? AND video_refund_state = ?", userID, "pending").
		Where("video_refund_requested_at > 0 AND video_refund_requested_at < ? AND COALESCE(video_refund_completed_at, 0) = 0", asOf-overdueBillingRefundSeconds).
		Where("CASE WHEN finish_time > 0 THEN finish_time ELSE video_refund_requested_at END BETWEEN ? AND ?", start, end).
		Count(&count).Error
	return count, err
}
