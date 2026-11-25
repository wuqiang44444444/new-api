package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"gorm.io/gorm"
)

const ModelStorageHourlyExprOption = "billing_setting.storage_hourly_expr"

func validateModelStorageHourlyPrice(value any) error {
	expression, ok := value.(string)
	if !ok {
		return fmt.Errorf("%s must be an expression string", ModelStorageHourlyExprOption)
	}
	rate, err := resolveStorageHourlyExchangeRate(expression)
	if err != nil {
		return err
	}
	_, _, err = billingexpr.StorageHourlyPrice(expression, rate)
	return err
}

func resolveStorageHourlyExchangeRate(expression string) (*billingexpr.ExchangeRateContext, error) {
	if !billingexpr.UsesExchangeRate(expression) {
		return nil, nil
	}
	return operation_setting.CurrentUsdExchangeRateContext()
}

func GetModelStorageHourlyExpr(ctx context.Context, modelName string) (string, error) {
	var option Option
	err := DB.WithContext(ctx).Take(&option, commonKeyCol+" = ?", ModelStorageHourlyExprOption).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var expressions map[string]string
	if err := common.UnmarshalJsonStr(option.Value, &expressions); err != nil {
		return "", err
	}
	return expressions[modelName], nil
}
