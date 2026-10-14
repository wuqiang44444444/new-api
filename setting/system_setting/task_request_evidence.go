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

// 请求证据的部署配置。采集默认关闭，仅显式配置开启；显式开启后统一采集
// 已登记媒体操作。正文及事件明细保留期为 1～7 天（默认 7），非法配置在
// 加载期明确报错，不静默改成永久保存或无证据模式。加密密钥未配置时从
// CRYPTO_SECRET/SESSION_SECRET 派生（见 ResolveTaskRequestEvidenceConfig）。
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
	defaultTaskRequestEvidenceRetention   = 7
	maxTaskRequestEvidenceRetention       = 7
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
	// 只有显式 "true" 才开启采集；未配置、空白或任何其它值都不开启。
	enabledRaw := strings.ToLower(strings.TrimSpace(os.Getenv(TaskRequestEvidenceEnabledEnv)))
	invalid := &configInvalidReason{}
	config := TaskRequestEvidenceConfig{
		Enabled:             enabledRaw == "true",
		StorageDir:          common.GetEnvOrDefaultString(TaskRequestEvidenceStorageDirEnv, defaultTaskRequestEvidenceStorageDir),
		EncryptionKeyHex:    strings.TrimSpace(os.Getenv(TaskRequestEvidenceEncryptionKeyEnv)),
		MaxBodyBytes:        int64(common.GetEnvOrDefault(TaskRequestEvidenceMaxBodyBytesEnv, int(defaultTaskRequestEvidenceMaxBody))),
		MaxResponseBytes:    int64(common.GetEnvOrDefault(TaskRequestEvidenceMaxResponseBytesEnv, int(defaultTaskRequestEvidenceMaxResponse))),
		WriteTimeoutSeconds: common.GetEnvOrDefault(TaskRequestEvidenceWriteTimeoutSecondsEnv, defaultTaskRequestEvidenceTimeout),
		RetentionDays:       common.GetEnvOrDefault(TaskRequestEvidenceRetentionDaysEnv, defaultTaskRequestEvidenceRetention),
	}
	if raw, ok := os.LookupEnv(TaskRequestEvidenceEnabledEnv); ok && raw != "" && enabledRaw != "true" && enabledRaw != "false" {
		invalid.set("enabled must be true or false")
		// 非法启用值不被当成开启，但必须响亮报告，避免部署方误以为已开启。
		common.SysError("invalid " + TaskRequestEvidenceEnabledEnv + " value; collection stays disabled")
	}
	for _, key := range []string{TaskRequestEvidenceMaxBodyBytesEnv, TaskRequestEvidenceMaxResponseBytesEnv, TaskRequestEvidenceWriteTimeoutSecondsEnv, TaskRequestEvidenceRetentionDaysEnv} {
		if raw, ok := os.LookupEnv(key); ok && raw != "" {
			if _, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err != nil {
				invalid.set("invalid integer evidence configuration")
			}
		}
	}
	if invalid.reason != "" {
		config.InvalidReason = invalid.reason
	}
	if err := ValidateTaskRequestEvidenceConfig(config); err != nil {
		common.SysError("invalid task request evidence configuration: " + err.Error() + "; evidence requests will be rejected")
	}
	return config
}

// configInvalidReason 保存首个配置错误，保持加载期错误信息稳定。
type configInvalidReason struct{ reason string }

func (r *configInvalidReason) set(reason string) {
	if r.reason == "" {
		r.reason = reason
	}
}

// ResolveTaskRequestEvidenceConfig 在运行时解析生效配置：未配置显式密钥时，
// 从 CRYPTO_SECRET（或其回退 SESSION_SECRET）派生密钥。派生在采集关闭时同样
// 进行，历史正文的读取与过期清理不依赖采集开关；派生密钥随宿主密钥轮换而
// 失效，读取会得到明确的解密失败而非静默错误。必须在 main 的 InitEnv 之后调用。
func ResolveTaskRequestEvidenceConfig(loaded TaskRequestEvidenceConfig) TaskRequestEvidenceConfig {
	if strings.TrimSpace(loaded.EncryptionKeyHex) != "" {
		return loaded
	}
	if loaded.Enabled && os.Getenv("CRYPTO_SECRET") == "" && os.Getenv("SESSION_SECRET") == "" {
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

// ValidateTaskRequestEvidenceConfig 校验显式开启时的完整配置合同；不访问
// 文件系统或密钥派生。关闭时忽略采集合同。
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
	if config.RetentionDays < 1 || config.RetentionDays > maxTaskRequestEvidenceRetention {
		return fmt.Errorf("retention days must be an integer between 1 and %d; permanent retention is no longer supported", maxTaskRequestEvidenceRetention)
	}
	return nil
}
