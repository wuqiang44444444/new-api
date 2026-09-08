package service

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
)

func freezeNativeImageBilling(ctx context.Context, task *model.Task, info *relaycommon.RelayInfo, request *dto.ImageRequest) error {
	data := task.PrivateData.ImageTask
	price := info.PriceData
	data.Price = &price
	task.PrivateData.BillingContext.ModelRatio = price.ModelRatio
	if info.TieredBillingSnapshot == nil {
		return nil
	}
	probe := billingexpr.RequestInput{}
	if info.BillingRequestInput != nil {
		probe = *info.BillingRequestInput
	} else {
		north := *request
		north.Model = info.OriginModelName
		body, err := common.Marshal(north)
		if err != nil {
			return err
		}
		probe.Body = body
	}
	encoded, err := common.Marshal(probe)
	if err != nil {
		return err
	}
	ciphertext, err := common.EncryptShortLivedSecretForScope("image-billing:"+task.TaskID, string(encoded))
	if err != nil {
		return err
	}
	ctx, err = WithImageObjectStore(ctx)
	if err != nil {
		return err
	}
	data.NativeRequest.BillingProbe, err = StoreImageTaskPayload(ctx, task.TaskID, "billing", strings.NewReader(ciphertext))
	task.PrivateData.AsyncBilling.BillingProbe = nil
	return err
}

func restoreNativeImageBilling(ctx context.Context, task *model.Task) (string, error) {
	ctx, err := WithImageObjectStore(ctx)
	if err != nil {
		return "", err
	}
	file, err := RestoreImageTaskPayload(ctx, task.PrivateData.ImageTask.NativeRequest.BillingProbe)
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	ciphertext, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	return common.DecryptShortLivedSecretForScope("image-billing:"+task.TaskID, string(ciphertext))
}
