// customer-original-replay inventories historical customer list prices using
// the release's resolver. Use a read-only database role on deployed sources.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func run(ctx context.Context, from, through string, userID int) error {
	if userID < 0 {
		return fmt.Errorf("invalid -user-id")
	}
	zone := model.UsageAnalyticsZone()
	first, err := time.ParseInLocation(time.DateOnly, from, zone)
	if err != nil {
		return fmt.Errorf("invalid -from date")
	}
	last, err := time.ParseInLocation(time.DateOnly, through, zone)
	if err != nil || last.Before(first) || last.After(time.Now()) {
		return fmt.Errorf("invalid -through date")
	}
	ctx, closeSources, err := model.OpenCustomerOriginalReplayInventory(ctx)
	if err != nil {
		return err
	}
	defer closeSources()
	for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
		inventory, err := model.InspectCustomerOriginalReplay(ctx, day, userID)
		if err != nil {
			return fmt.Errorf("inventory failed for %s", day.Format(time.DateOnly))
		}
		raw, err := common.Marshal(inventory)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintln(os.Stdout, string(raw)); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	gin.DefaultWriter = os.Stderr
	from := flag.String("from", "", "first Beijing calendar date (YYYY-MM-DD)")
	through := flag.String("through", "", "last Beijing calendar date (YYYY-MM-DD)")
	userID := flag.Int("user-id", 0, "inspect only this customer (zero includes all customers)")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, *from, *through, *userID); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
