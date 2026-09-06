package system_setting

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/QuantumNous/new-api/common"
)

// 音视频请求证据的部署配置。证据默认开启；显式环境变量优先，
// 加密密钥未配置时从 CRYPTO_SECRET/SESSION_SECRET 派生（见
// ResolveTaskRequestEvidenceConfig 的告警语义）。保留期默认 0 表示
// 不自动清理。
const (
	TaskRequestEvidenceEnabledEnv             = "TASK_REQUEST_EVIDENCE_ENABLED"
	TaskRequestEvidenceStorageDirEnv          = "TASK_REQUEST_EVIDENCE_STORAGE_DIR"
	TaskRequestEvidenceEncryptionKeyEnv       = "TASK_REQUEST_EVIDENCE_ENCRYPTION_KEY"
	TaskRequestEvidenceMaxBodyBytesEnv        = "TASK_REQUEST_EVIDENCE_MAX_BODY_BYTES"
	TaskRequestEvidenceMaxResponseBytesEnv    = "TASK_REQUEST_EVIDENCE_MAX_RESPONSE_BYTES"
	TaskRequestEvidenceWriteTimeoutSecondsEnv = "TASK_REQUEST_EVIDENCE_WRITE_TIMEOUT_SECONDS"
	TaskRequestEvidenceRetentionDaysEnv       = "TASK_REQUEST_EVIDENCE_RETENTION_DAYS"

	defaultTaskRequestEvidenceStorageDir  = "data/task-request-evidence"
	defaultTaskRequestEvidenceMaxBody     = int64(8 << 20)
	maxTaskRequestEvidenceMaxBody         = int64(256 << 20)
	defaultTaskRequestEvidenceMaxResponse = int64(32 << 20)
	maxTaskRequestEvidenceMaxResponse     = int64(1 << 30)
	defaultTaskRequestEvidenceTimeout     = 5
	maxTaskRequestEvidenceTimeout         = 60
)

// TaskRequestEvidenceConfig 是证据子系统的运行配置合同。
type TaskRequestEvidenceConfig struct {
	Enabled             bool
	StorageDir          string
	EncryptionKeyHex    string
	MaxBodyBytes        int64
	MaxResponseBytes    int64
	WriteTimeoutSeconds int
	RetentionDays       int
	// KeyDerived 表示密钥由 CRYPTO_SECRET/SESSION_SECRET 派生而非显式配置。
	KeyDerived    bool
	InvalidReason string
}

// LoadTaskRequestEvidenceConfig 读取环境变量并做语法校验；不访问文件系统、
// 不做密钥派生。密钥派生在 InitEnv 之后的惰性解析中进行。
func LoadTaskRequestEvidenceConfig() TaskRequestEvidenceConfig {
	config := TaskRequestEvidenceConfig{
		Enabled:             strings.ToLower(strings.TrimSpace(common.GetEnvOrDefaultString(TaskRequestEvidenceEnabledEnv, "true"))) != "false",
		StorageDir:          common.GetEnvOrDefaultString(TaskRequestEvidenceStorageDirEnv, defaultTaskRequestEvidenceStorageDir),
		EncryptionKeyHex:    strings.TrimSpace(os.Getenv(TaskRequestEvidenceEncryptionKeyEnv)),
		MaxBodyBytes:        int64(common.GetEnvOrDefault(TaskRequestEvidenceMaxBodyBytesEnv, int(defaultTaskRequestEvidenceMaxBody))),
		MaxResponseBytes:    int64(common.GetEnvOrDefault(TaskRequestEvidenceMaxResponseBytesEnv, int(defaultTaskRequestEvidenceMaxResponse))),
		WriteTimeoutSeconds: common.GetEnvOrDefault(TaskRequestEvidenceWriteTimeoutSecondsEnv, defaultTaskRequestEvidenceTimeout),
		RetentionDays:       common.GetEnvOrDefault(TaskRequestEvidenceRetentionDaysEnv, 0),
	}
	if raw, ok := os.LookupEnv(TaskRequestEvidenceEnabledEnv); ok && raw != "" && !strings.EqualFold(strings.TrimSpace(raw), "true") && !strings.EqualFold(strings.TrimSpace(raw), "false") {
		config.InvalidReason = "enabled must be true or false"
	}
	for _, key := range []string{TaskRequestEvidenceMaxBodyBytesEnv, TaskRequestEvidenceMaxResponseBytesEnv, TaskRequestEvidenceWriteTimeoutSecondsEnv, TaskRequestEvidenceRetentionDaysEnv} {
		if raw, ok := os.LookupEnv(key); ok && raw != "" {
			if _, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err != nil {
				config.InvalidReason = "invalid integer evidence configuration"
			}
		}
	}
	if err := ValidateTaskRequestEvidenceConfig(config); err != nil {
		common.SysError("invalid task request evidence configuration: " + err.Error() + "; evidence requests will be rejected")
	}
	return config
}

// ResolveTaskRequestEvidenceConfig 在运行时解析生效配置：启用但未配置显式
// 密钥时，从 CRYPTO_SECRET（或其回退 SESSION_SECRET）派生密钥。派生密钥
// 随宿主密钥轮换而失效，读取会得到明确的解密失败而非静默错误，且会输出
// 告警提示部署方显式配置。必须在 main 的 InitEnv 之后调用。
func ResolveTaskRequestEvidenceConfig(loaded TaskRequestEvidenceConfig) TaskRequestEvidenceConfig {
	if !loaded.Enabled {
		return loaded
	}
	if strings.TrimSpace(loaded.EncryptionKeyHex) != "" {
		return loaded
	}
	if os.Getenv("CRYPTO_SECRET") == "" && os.Getenv("SESSION_SECRET") == "" {
		common.SysError("task request evidence key derived from ephemeral secret; " +
			"evidence bodies will be unreadable after restart. Set CRYPTO_SECRET or " +
			"TASK_REQUEST_EVIDENCE_ENCRYPTION_KEY for production use")
	}
	digest := sha256.Sum256([]byte("task-request-evidence:" + common.CryptoSecret))
	loaded.EncryptionKeyHex = hex.EncodeToString(digest[:])
	loaded.KeyDerived = true
	return loaded
}

var (
	taskRequestEvidenceRuntimeOnce sync.Once
	taskRequestEvidenceRuntime     TaskRequestEvidenceConfig
)

// SetTaskRequestEvidenceConfig 供测试注入覆盖，跳过惰性解析。
func SetTaskRequestEvidenceConfig(config TaskRequestEvidenceConfig) {
	taskRequestEvidenceRuntimeOnce.Do(func() {})
	taskRequestEvidenceRuntime = config
}

// GetTaskRequestEvidenceConfig 返回进程内生效的证据配置；首次调用发生在
// InitEnv 之后（首个请求或首个使用方），保证派生密钥使用最终宿主密钥。
func GetTaskRequestEvidenceConfig() TaskRequestEvidenceConfig {
	taskRequestEvidenceRuntimeOnce.Do(func() {
		taskRequestEvidenceRuntime = ResolveTaskRequestEvidenceConfig(LoadTaskRequestEvidenceConfig())
	})
	return taskRequestEvidenceRuntime
}

// ValidateTaskRequestEvidenceConfig 只做语法校验，不访问文件系统或密钥派生。
func ValidateTaskRequestEvidenceConfig(config TaskRequestEvidenceConfig) error {
	if !config.Enabled {
		return nil
	}
	if config.InvalidReason != "" {
		return fmt.Errorf("%s", config.InvalidReason)
	}
	if strings.TrimSpace(config.StorageDir) == "" {
		return fmt.Errorf("storage dir is required when evidence is enabled")
	}
	if config.EncryptionKeyHex != "" {
		key, err := hex.DecodeString(config.EncryptionKeyHex)
		if err != nil || len(key) != 32 {
			return fmt.Errorf("encryption key must be 64 hex chars (32 bytes)")
		}
	}
	if config.MaxBodyBytes <= 0 || config.MaxBodyBytes > maxTaskRequestEvidenceMaxBody {
		return fmt.Errorf("max body bytes must be between 1 and %d", maxTaskRequestEvidenceMaxBody)
	}
	if config.MaxResponseBytes <= 0 || config.MaxResponseBytes > maxTaskRequestEvidenceMaxResponse {
		return fmt.Errorf("max response bytes must be between 1 and %d", maxTaskRequestEvidenceMaxResponse)
	}
	if config.WriteTimeoutSeconds <= 0 || config.WriteTimeoutSeconds > maxTaskRequestEvidenceTimeout {
		return fmt.Errorf("write timeout must be between 1 and %d seconds", maxTaskRequestEvidenceTimeout)
	}
	if config.RetentionDays < 0 || config.RetentionDays > 3650 {
		return fmt.Errorf("retention days must be between 0 (no cleanup) and 3650")
	}
	return nil
}
