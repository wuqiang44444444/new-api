package relay

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func freezeImageTaskHeaders(taskID string, c *gin.Context, info *relaycommon.RelayInfo) (string, error) {
	headers, err := channel.ResolveHeaderOverride(info, c)
	if err != nil {
		return "", err
	}
	encoded, err := common.Marshal(headers)
	if err != nil {
		return "", err
	}
	return common.EncryptShortLivedSecretForScope("image-headers:"+taskID, string(encoded))
}

func restoreImageTaskHeaders(task *model.Task) (map[string]string, error) {
	if task.PrivateData.ImageTask == nil || task.PrivateData.ImageTask.HeadersCiphertext == "" {
		return nil, errors.New("image request headers snapshot is missing")
	}
	encoded, err := common.DecryptShortLivedSecretForScope("image-headers:"+task.TaskID, task.PrivateData.ImageTask.HeadersCiphertext)
	if err != nil {
		return nil, errors.New("image request headers could not be restored")
	}
	var headers map[string]string
	if err := common.Unmarshal([]byte(encoded), &headers); err != nil {
		return nil, errors.New("image request headers snapshot is invalid")
	}
	return headers, nil
}
