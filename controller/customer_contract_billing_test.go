package controller

import (
	"errors"
	"math"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	kittypes "github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contractReservation struct {
	held    int
	targets []int
	err     error
}

func (b *contractReservation) Reserve(target int) error {
	b.targets = append(b.targets, target)
	if b.err != nil {
		return b.err
	}
	b.held = max(b.held, target)
	return nil
}
func (b *contractReservation) GetPreConsumedQuota() int { return b.held }
func (*contractReservation) Refund(*gin.Context)        {}
func (*contractReservation) NeedsRefund() bool          { return false }
func (*contractReservation) Settle(int) error           { return nil }

func TestCustomerContractRelayBillingRefreshesGroupWithoutCompoundingDiscount(t *testing.T) {
	for _, perCall := range []bool{false, true} {
		t.Run(map[bool]string{false: "tokens", true: "per-call"}[perCall], func(t *testing.T) {
			billing := &contractReservation{}
			info := &relaycommon.RelayInfo{Billing: billing, ContractBillingFact: &hosttypes.ContractBillingFact{RatioUnits: 50_000_000}, PriceData: hosttypes.PriceData{UsePrice: perCall, ModelPrice: 0.02, ModelRatio: 2}}
			info.PriceData.AddOtherRatio("n", 3)
			meta := &kittypes.TokenCountMeta{MaxTokens: 100}
			base := (max(1000, common.PreConsumedQuota) + 100) * 2
			if perCall {
				base = common.QuotaFromFloat(0.02 * common.QuotaPerUnit * 3)
			}
			for _, ratio := range []float64{1, 4, 2, 0} {
				info.PriceData.GroupRatioInfo.GroupRatio = ratio
				require.Nil(t, prepareCustomerContractRelayBilling(nil, info, 1000, meta))
				assert.Equal(t, common.QuotaFromFloat(float64(base)*ratio*0.5), info.PriceData.QuotaToPreConsume)
				assert.False(t, info.PriceData.FreeModel, "an existing paid session survives paid-to-free retries")
			}
			assert.Equal(t, base*2, info.FinalPreConsumedQuota, "only the highest target is reserved, cheaper attempts settle later")
			billing.err = errors.New("key quota exhausted")
			info.PriceData.GroupRatioInfo.GroupRatio = 5
			err := prepareCustomerContractRelayBilling(nil, info, 1000, meta)
			require.NotNil(t, err)
			assert.True(t, kittypes.IsSkipRetryError(err))
		})
	}
}

func TestCustomerContractImageRetryReservesFullCount(t *testing.T) {
	for _, tc := range []struct {
		name       string
		firstGroup float64
		tokenQuota int
		wantError  bool
	}{
		{name: "paid to expensive", firstGroup: 1, tokenQuota: 100000},
		{name: "free to paid", firstGroup: 0, tokenQuota: 100000},
		{name: "key cannot cover expensive retry", firstGroup: 1, tokenQuota: 40000, wantError: true},
		{name: "key cannot cover free to paid", firstGroup: 0, tokenQuota: 40000, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, user, _ := setupCustomerContractControllerDB(t)
			oldBatch := common.BatchUpdateEnabled
			oldFreePreConsume := operation_setting.GetQuotaSetting().EnableFreeModelPreConsume
			common.BatchUpdateEnabled = false
			operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = false
			t.Cleanup(func() {
				common.BatchUpdateEnabled = oldBatch
				operation_setting.GetQuotaSetting().EnableFreeModelPreConsume = oldFreePreConsume
			})
			require.NoError(t, model.DB.Model(&user).Update("quota", 100000).Error)
			token := model.Token{UserId: user.Id, Key: "contract-image-retry-test", RemainQuota: tc.tokenQuota, Status: common.TokenStatusEnabled, ExpiredTime: -1}
			require.NoError(t, model.DB.Create(&token).Error)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("token_quota", 0) // Exercise actual holds, without the native trust bypass.
			info := &relaycommon.RelayInfo{
				UserId: user.Id, TokenId: token.Id, TokenKey: token.Key,
				UserSetting:         dto.UserSetting{BillingPreference: "wallet_only"},
				ContractBillingFact: &hosttypes.ContractBillingFact{RatioUnits: 50_000_000},
				PriceData: hosttypes.PriceData{UsePrice: true, ModelPrice: 0.04,
					GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: tc.firstGroup}},
			}
			info.PriceData.AddOtherRatio("n", 3)
			meta := &kittypes.TokenCountMeta{}
			// Native first-attempt pricing: 0.04 * 500000 * 3 * group * 0.5.
			firstHold := 0
			if tc.firstGroup != 0 {
				firstHold = 30000
				require.Nil(t, service.PreConsumeBilling(c, firstHold, info))
			}
			require.Nil(t, prepareCustomerContractRelayBilling(c, info, 0, meta))
			assert.Equal(t, firstHold, info.PriceData.QuotaToPreConsume)
			if tc.firstGroup == 0 {
				require.Nil(t, info.Billing, "free attempt must not create a billing session")
			}
			firstSession := info.Billing
			info.PriceData.GroupRatioInfo.GroupRatio = 2
			apiErr := prepareCustomerContractRelayBilling(c, info, 0, meta)
			assert.Equal(t, 60000, info.PriceData.QuotaToPreConsume)
			wantHeld := 60000
			if tc.wantError {
				require.NotNil(t, apiErr, "insufficient Key quota must stop the attempt before sending")
				assert.True(t, kittypes.IsSkipRetryError(apiErr))
				assert.False(t, shouldRetry(c, apiErr, 3))
				wantHeld = firstHold
			} else {
				require.Nil(t, apiErr)
				require.NotNil(t, info.Billing)
				assert.Equal(t, wantHeld, info.Billing.GetPreConsumedQuota())
				if firstSession != nil {
					assert.Same(t, firstSession, info.Billing)
				}
			}
			assert.Equal(t, wantHeld, info.FinalPreConsumedQuota)
			var heldUser model.User
			var heldToken model.Token
			require.NoError(t, model.DB.First(&heldUser, user.Id).Error)
			require.NoError(t, model.DB.First(&heldToken, token.Id).Error)
			assert.Equal(t, 100000-wantHeld, heldUser.Quota)
			assert.Equal(t, tc.tokenQuota-wantHeld, heldToken.RemainQuota)
			if info.Billing != nil {
				require.NoError(t, info.Billing.Settle(0))
			}
		})
	}
}

func TestCustomerContractBillingRejectsInvalidMultiplierBeforeReservation(t *testing.T) {
	for _, ratio := range []float64{math.Inf(1), math.NaN(), -1} {
		billing := &contractReservation{}
		info := &relaycommon.RelayInfo{Billing: billing, ContractBillingFact: &hosttypes.ContractBillingFact{RatioUnits: 50_000_000}, PriceData: hosttypes.PriceData{UsePrice: true, ModelPrice: 1, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: ratio}}}
		require.NotNil(t, prepareCustomerContractRelayBilling(nil, info, 0, &kittypes.TokenCountMeta{}))
		assert.Empty(t, billing.targets)
	}
}

func TestCustomerContractFixedPriceCountConvertsQuotaOnce(t *testing.T) {
	for _, tc := range []struct {
		name      string
		price     float64
		wantError bool
	}{
		{name: "fractional unit charge", price: 0.0000012},
		{name: "count overflows quota", price: float64(common.MaxQuota) / common.QuotaPerUnit / 2, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			billing := &contractReservation{}
			info := &relaycommon.RelayInfo{
				Billing: billing, ContractBillingFact: &hosttypes.ContractBillingFact{RatioUnits: 100_000_000},
				PriceData: hosttypes.PriceData{UsePrice: true, ModelPrice: tc.price, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}},
			}
			info.PriceData.AddOtherRatio("n", 3)
			apiErr := prepareCustomerContractRelayBilling(nil, info, 0, &kittypes.TokenCountMeta{})
			if tc.wantError {
				require.NotNil(t, apiErr)
				assert.True(t, kittypes.IsSkipRetryError(apiErr))
				assert.Empty(t, billing.targets, "an overflowing count must be rejected before reserving quota")
				return
			}
			require.Nil(t, apiErr)
			assert.Equal(t, 1, info.PriceData.QuotaToPreConsume, "0.6 per image * 3 is truncated once, after multiplying")
			assert.Equal(t, 1, billing.held)
		})
	}
}
