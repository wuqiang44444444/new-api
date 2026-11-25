// verify-ark-prefix-cache runs an isolated Ark Responses billing experiment.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	planPath := flag.String("plan", "", "local JSON experiment plan")
	execute := flag.Bool("execute", false, "send the planned requests to the isolated Ark endpoint and incur provider charges")
	flag.Parse()
	if *planPath == "" || flag.NArg() != 0 {
		return fmt.Errorf("provide -plan with a local experiment plan")
	}
	file, err := os.Open(*planPath)
	if err != nil {
		return fmt.Errorf("cannot open the experiment plan")
	}
	defer file.Close()
	var plan experimentPlan
	if err := common.DecodeJsonBody(file, &plan); err != nil {
		return fmt.Errorf("invalid experiment JSON")
	}
	if err := plan.validate(time.Now()); err != nil {
		return err
	}
	if !*execute {
		fmt.Printf("Validated %d planned requests\n", len(plan.Steps))
		return nil
	}
	key := strings.TrimSpace(os.Getenv("ARK_API_KEY"))
	if key == "" {
		return fmt.Errorf("ARK_API_KEY for the isolated Ark project is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	protocols := new(http.Protocols)
	protocols.SetHTTP1(true)
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Protocols:         protocols,
			DisableKeepAlives: true,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return runExperiment(ctx, client, arkResponsesURL, key, plan, os.Stdout)
}
