package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/joho/godotenv"
)

var apply = flag.Bool("apply", false, "apply the migration; otherwise only validate and print a plan")

func main() {
	flag.Parse()
	_ = godotenv.Load(".env")
	common.InitEnv()

	var input model.OfficialAssetCredentialMigrationInput
	if err := common.DecodeJson(os.Stdin, &input); err != nil {
		exitWithError(fmt.Errorf("decode migration input from stdin: %w", err))
	}
	if err := model.InitDBForOfficialAssetCredentialMigration(); err != nil {
		exitWithError(err)
	}
	defer func() {
		_ = model.CloseDB()
	}()

	if *apply {
		if !input.AcknowledgeSameProviderAccount {
			exitWithError(errors.New("same Provider account acknowledgement is required"))
		}
		if _, err := model.RunOfficialAssetCredentialMigration(input, false); err != nil {
			exitWithError(err)
		}
		channel, err := model.GetChannelById(input.ChannelID, true)
		if err != nil {
			exitWithError(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := service.CheckProposedOfficialAssetCredentialConnectivity(
			ctx,
			channel,
			&dto.ChannelAssetCredentialInput{
				AccessKeyID:     input.AssetAccessKeyID,
				SecretAccessKey: input.AssetSecretAccessKey,
			},
		); err != nil {
			exitWithError(fmt.Errorf("read-only official asset Action verification failed: %w", err))
		}
	}

	result, err := model.RunOfficialAssetCredentialMigration(input, *apply)
	if err != nil {
		exitWithError(err)
	}
	payload, err := common.Marshal(map[string]any{
		"success": true,
		"data":    result,
	})
	if err != nil {
		exitWithError(err)
	}
	_, _ = os.Stdout.Write(append(payload, '\n'))
}

func exitWithError(err error) {
	payload, marshalErr := common.Marshal(map[string]any{
		"success": false,
		"message": err.Error(),
	})
	if marshalErr == nil {
		_, _ = os.Stderr.Write(append(payload, '\n'))
	} else {
		_, _ = fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(1)
}
