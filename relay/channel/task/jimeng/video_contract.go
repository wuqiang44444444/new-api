package jimeng

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

func jimengContractPayload(c *gin.Context, info *relaycommon.RelayInfo) (*requestPayload, bool, error) {
	contract, ok := relaycommon.GetVideoContractRequest(c)
	if !ok {
		return nil, false, nil
	}
	if contract.ContractID != dto.VideoContractJimeng || contract.Jimeng == nil {
		return nil, true, relaycommon.NewVideoContractError("invalid_video_contract", "Jimeng route received an incompatible video contract")
	}
	payload := &requestPayload{ReqKey: info.UpstreamModelName, Frames: 121}
	encoded, err := common.Marshal(contract.Jimeng)
	if err != nil {
		return nil, true, err
	}
	if err := common.Unmarshal(encoded, payload); err != nil {
		return nil, true, err
	}
	if strings.TrimSpace(info.UpstreamModelName) != "" {
		payload.ReqKey = info.UpstreamModelName
	}
	imageLen := lo.Max([]int{len(payload.BinaryDataBase64), len(payload.ImageUrls)})
	if strings.Contains(payload.ReqKey, "jimeng_v30") {
		switch {
		case payload.ReqKey == "jimeng_v30_pro":
			payload.ReqKey = "jimeng_ti2v_v30_pro"
		case imageLen > 1:
			payload.ReqKey = strings.TrimSuffix(strings.Replace(payload.ReqKey, "jimeng_v30", "jimeng_i2v_first_tail_v30", 1), "p")
		case imageLen == 1:
			payload.ReqKey = strings.TrimSuffix(strings.Replace(payload.ReqKey, "jimeng_v30", "jimeng_i2v_first_v30", 1), "p")
		default:
			payload.ReqKey = strings.Replace(payload.ReqKey, "jimeng_v30", "jimeng_t2v_v30", 1)
		}
	}
	return payload, true, nil
}
