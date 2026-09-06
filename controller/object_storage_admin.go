package controller

import (
	"errors"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
)

// 对象存储专用管理接口（本地扩展，仅最高管理员）。读取返回脱敏配置与
// credential_configured 状态；「测试连接」只验证当前表单值、不落库；
// 「保存并启用」对同一完整配置先校验再验证、通过后原子保存。审计只记
// 操作者、动作与变更字段，不记录任何秘密值。

type objectStorageSettingRequest struct {
	Backend     string `json:"backend"`
	Endpoint    string `json:"endpoint"`
	Bucket      string `json:"bucket"`
	Prefix      string `json:"prefix"`
	Region      string `json:"region"`
	AccountName string `json:"account_name"`
	// Credential 是明文凭据，只写不读；空白表示沿用已保存密钥。
	Credential string `json:"credential"`
	// InputMode 仅 Azure 使用：connection_string | manual。
	InputMode        string `json:"input_mode"`
	ConnectionString string `json:"connection_string"`
}

// GetObjectStorageSetting 返回脱敏后的对象存储配置与运行状态。
func GetObjectStorageSetting(c *gin.Context) {
	raw, err := model.GetObjectStorageSetting()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var config system_setting.ObjectStorageConfig
	if raw != "" {
		if err := common.UnmarshalJsonStr(raw, &config); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	_, envImportAvailable := service.BuildObjectStorageEnvImportPreview()
	lastTestStatus := config.LastTestStatus
	if lastTestStatus == "" {
		lastTestStatus = "none"
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"backend":               config.Backend,
			"endpoint":              config.Endpoint,
			"bucket":                config.Bucket,
			"prefix":                config.Prefix,
			"region":                config.Region,
			"account_name":          config.AccountName,
			"account_name_masked":   system_setting.MaskObjectStorageAccountName(config.AccountName),
			"credential_configured": config.CredentialCiphertext != "",
			"revision":              config.Revision,
			"last_test_status":      lastTestStatus,
			"last_test_at":          config.LastTestAt,
			"active":                service.GetTaskArtifactStore().Enabled(),
			"env_import_available":  envImportAvailable,
		},
	})
}

// TestObjectStorageConnection 用当前表单值执行连通性测试，不替换在线配置。
func TestObjectStorageConnection(c *gin.Context) {
	var req objectStorageSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "无效的参数")
		return
	}
	config, credential, err := normalizeObjectStorageSettingRequest(&req)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	respondObjectStorageTestResult(c, service.RunObjectStorageConnectionTest(config, credential))
}

// UpdateObjectStorageSetting 校验并验证完整配置，通过后原子保存并启用。
// 测试失败时保留既有有效配置；显式关闭存储前检查待执行图片任务依赖。
func UpdateObjectStorageSetting(c *gin.Context) {
	var req objectStorageSettingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "无效的参数")
		return
	}
	config, credential, err := normalizeObjectStorageSettingRequest(&req)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}

	if config.Backend == system_setting.ObjectStorageBackendUpstream {
		// 关闭动作只切换配置，不删除远端既有对象。
		config.Endpoint = ""
		config.Bucket = ""
		config.Prefix = ""
		config.Region = ""
		config.AccountName = ""

		config.CredentialCiphertext = ""
		config.Revision = common.GetUUID()
		config.LastTestStatus = ""
		config.LastTestAt = 0
		if err := service.SaveObjectStorageSetting(config); err != nil {
			common.ApiError(c, err)
			return
		}
		recordManageAudit(c, "object_storage.update", gin.H{"backend": config.Backend})
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "对象存储已关闭；远端既有对象不会被删除",
		})
		return
	}

	testResult := service.RunObjectStorageConnectionTest(config, credential)
	if !testResult.Success {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "对象存储连通性测试未通过，已保留既有配置",
			"data":    testResult,
		})
		return
	}

	ciphertext, err := common.EncryptObjectStorageCredential(credential)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	config.CredentialCiphertext = ciphertext
	config.Revision = common.GetUUID()
	config.LastTestStatus = "passed"
	config.LastTestAt = time.Now().Unix()
	if err := service.SaveObjectStorageSetting(config); err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "object_storage.update", gin.H{"backend": config.Backend, "bucket": config.Bucket})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "对象存储配置已保存并启用",
		"data":    testResult,
	})
}

// PreviewObjectStorageEnvImport 生成既有 S3 环境变量的一次性导入预览；
// 预览不显示任何密钥。
func PreviewObjectStorageEnvImport(c *gin.Context) {
	preview, ok := service.BuildObjectStorageEnvImportPreview()
	if !ok {
		common.ApiErrorMsg(c, "当前没有可导入的对象存储环境变量配置")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    preview,
	})
}

// ImportObjectStorageEnvConfig 验证并提交环境变量配置到数据库。导入完成后
// 数据库是唯一配置源，旧环境变量不再参与运行期装载，也不作为失败 fallback。
func ImportObjectStorageEnvConfig(c *gin.Context) {
	config, ok := system_setting.ObjectStorageConfigFromLegacyEnv()
	if !ok {
		common.ApiErrorMsg(c, "当前没有可导入的对象存储环境变量配置")
		return
	}
	legacy := system_setting.LoadTaskArtifactStoreConfig()
	testResult := service.RunObjectStorageConnectionTest(config, legacy.S3SecretKey)
	if !testResult.Success {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "环境变量配置连通性测试未通过，未写入数据库",
			"data":    testResult,
		})
		return
	}
	saved, err := service.CommitObjectStorageEnvImport()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	recordManageAudit(c, "object_storage.import", gin.H{"backend": saved.Backend, "bucket": saved.Bucket})
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "环境变量配置已导入数据库并启用；后续请在系统设置中维护对象存储配置",
	})
}

// normalizeObjectStorageSettingRequest 把表单值标准化为配置与明文凭据。
// Azure 连接字符串只作为输入格式，解析为唯一标准化配置；凭据空白时按
// backend + 账号匹配沿用已保存密钥。
func normalizeObjectStorageSettingRequest(req *objectStorageSettingRequest) (system_setting.ObjectStorageConfig, string, error) {
	config := system_setting.ObjectStorageConfig{
		Backend: req.Backend,
		Prefix:  req.Prefix,
	}
	credential := req.Credential
	switch config.Backend {
	case system_setting.ObjectStorageBackendUpstream:
		return config, "", nil
	case system_setting.ObjectStorageBackendAzureBlob:
		if req.InputMode != system_setting.ManualConnectionInputMode && req.ConnectionString != "" {
			account, err := system_setting.ParseAzureBlobConnectionString(req.ConnectionString)
			if err != nil {
				return config, "", err
			}
			config.Endpoint = account.BlobEndpoint
			config.Bucket = req.Bucket
			config.AccountName = account.AccountName
			if credential == "" {
				credential = account.AccountKey
			}
			break
		}
		config.Endpoint = req.Endpoint
		config.Bucket = req.Bucket
		config.AccountName = req.AccountName
	case system_setting.ObjectStorageBackendS3:
		config.Endpoint = req.Endpoint
		config.Bucket = req.Bucket
		config.Region = req.Region
		config.AccountName = req.AccountName
	default:
		return config, "", errors.New("不支持的对象存储类型")
	}
	if credential == "" {
		resolved, err := resolveStoredObjectStorageCredential(config)
		if err != nil {
			return config, "", err
		}
		credential = resolved
	}
	return config, credential, nil
}

// resolveStoredObjectStorageCredential 在凭据未重新输入时，按 backend 与
// 账号匹配复用已保存密钥；账号或 backend 变化必须显式提供新密钥。
func resolveStoredObjectStorageCredential(config system_setting.ObjectStorageConfig) (string, error) {
	raw, err := model.GetObjectStorageSetting()
	if err != nil {
		return "", err
	}
	if raw == "" {
		return "", errors.New("请提供访问密钥")
	}
	var stored system_setting.ObjectStorageConfig
	if err := common.UnmarshalJsonStr(raw, &stored); err != nil {
		return "", err
	}
	if stored.Backend != config.Backend || stored.AccountName != config.AccountName || stored.CredentialCiphertext == "" {
		return "", errors.New("账号或存储类型已变化，请提供访问密钥")
	}
	credential, err := common.DecryptObjectStorageCredential(stored.CredentialCiphertext)
	if err != nil {
		return "", errors.New("已保存密钥解密失败，请重新提供访问密钥")
	}
	return credential, nil
}

func respondObjectStorageTestResult(c *gin.Context, result *service.ObjectStorageTestResult) {
	status := http.StatusOK
	message := "对象存储连通性测试通过"
	if !result.Success {
		message = "对象存储连通性测试未通过"
	}
	c.JSON(status, gin.H{
		"success": result.Success,
		"message": message,
		"data":    result,
	})
}
