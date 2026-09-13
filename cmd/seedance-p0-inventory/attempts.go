package main

import (
	"fmt"
	"io"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

func printAttemptBillingInventory(db *gorm.DB, out io.Writer) error {
	var rows []struct {
		Status           string
		BillingHoldState string
		Count            int64
	}
	// Include frozen Link identity even if the current channel has been deleted.
	err := db.Table("task_create_attempts").Select("status, billing_hold_state, COUNT(*) AS count").
		Where("channel_id IN (?) OR client_protocol = ?", db.Table("channels").Select("id").Where("type = ?", constant.ChannelTypeSeedanceLink), model.TaskClientProtocolModelArkV3).
		Group("status").Group("billing_hold_state").Order("status, billing_hold_state").Find(&rows).Error
	if err != nil {
		return fmt.Errorf("attempt funding inventory unavailable")
	}
	fmt.Fprintln(out, "task_create_attempts status × billing_hold_state:")
	var unresolved int64
	for _, row := range rows {
		fmt.Fprintf(out, "  %s | %s | %d\n", row.Status, row.BillingHoldState, row.Count)
		if row.BillingHoldState == "held" || (row.Status != "complete" && row.Status != "rejected") {
			unresolved += row.Count
		}
	}
	fmt.Fprintf(out, "unresolved_create_attempts: %d\n", unresolved)
	return nil
}
