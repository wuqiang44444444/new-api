package service

import (
	"errors"
	"sync"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
)

// 存储运行时装载（本地扩展）。数据库是配置的唯一持久事实：观察者收到该
// 命名空间的最新配置后，以完整不可变快照构造新的存储实例并原子替换；
// revision 未变化时不重建，单次操作期间持有的始终是同一个实例。任何解析、
// 校验或解密失败都失败关闭为禁用并记录告警，不保留半更新状态。

const taskArtifactStoreBackendAzureBlob = system_setting.ObjectStorageBackendAzureBlob

type taskArtifactStoreRuntimeHolder struct {
	mu           sync.RWMutex
	store        TaskArtifactStore
	revision     string
	imageSession *imageObjectSession
}

var taskArtifactStoreRuntime = taskArtifactStoreRuntimeHolder{
	store: &disabledArtifactStore{},
}

// legacyEnvHintOnce 保证旧环境变量迁移提示只打印一次；lastFailedRaw 保证
// 同一份坏配置在多节点周期同步下只告警一次。
var legacyEnvHintOnce sync.Once
var lastFailedRaw string
var objectStorageApplyMu sync.Mutex

func (h *taskArtifactStoreRuntimeHolder) get() TaskArtifactStore {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.store
}

func (h *taskArtifactStoreRuntimeHolder) swap(store TaskArtifactStore, revision string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.revision == revision && revision != "" {
		return false
	}
	h.store = store
	h.revision = revision
	h.imageSession = nil
	return true
}

// applyTaskArtifactStoreSetting 解析数据库配置并重建存储实例。
// 空配置表示未启用；构建失败保持禁用（首次装载）或回退禁用并告警。
func applyTaskArtifactStoreSetting(raw string) {
	objectStorageApplyMu.Lock()
	defer objectStorageApplyMu.Unlock()
	if raw == "" {
		legacyEnvHintOnce.Do(warnLegacyObjectStorageEnv)
	}
	config, err := decodeObjectStorageSetting(raw)
	if err != nil {
		logObjectStorageSettingFailureOnce(raw, "invalid object storage setting ignored: "+err.Error())
		taskArtifactStoreRuntime.swap(&disabledArtifactStore{}, "")
		return
	}
	store, err := buildTaskArtifactStore(config)
	if err != nil {
		logObjectStorageSettingFailureOnce(raw, "object storage setting failed to activate: "+err.Error())
		taskArtifactStoreRuntime.swap(&disabledArtifactStore{}, "")
		return
	}
	lastFailedRaw = ""
	if taskArtifactStoreRuntime.swap(store, config.Revision) && store.Enabled() {
		common.SysLog("object storage activated: backend=" + config.Backend + " revision=" + config.Revision)
	}
}

func logObjectStorageSettingFailureOnce(raw, message string) {
	if raw == lastFailedRaw {
		return
	}
	lastFailedRaw = raw
	common.SysError(message)
}

// warnLegacyObjectStorageEnv 向仍依赖启动环境变量的旧部署提示一次性导入，
// 避免无提示切换配置源。
func warnLegacyObjectStorageEnv() {
	if _, ok := system_setting.ObjectStorageConfigFromLegacyEnv(); ok {
		common.SysLog("legacy TASK_ARTIFACT_STORE_* environment variables detected: object storage is now configured from the database; import it via System Settings > Object Storage")
	}
}

// decodeObjectStorageSetting 解析持久化配置 JSON；空值表示未配置存储。
func decodeObjectStorageSetting(raw string) (system_setting.ObjectStorageConfig, error) {
	var config system_setting.ObjectStorageConfig
	if raw == "" {
		return config, nil
	}
	if err := common.UnmarshalJsonStr(raw, &config); err != nil {
		return config, err
	}
	return config, nil
}

// buildTaskArtifactStore 以完整配置快照构造对应 backend 的存储实例。
// credential 为空且密文非空时解密密文；解密失败（如主密钥不一致）失败关闭。
func buildTaskArtifactStore(config system_setting.ObjectStorageConfig) (TaskArtifactStore, error) {
	if config.Backend == "" || config.Backend == system_setting.ObjectStorageBackendUpstream {
		return &disabledArtifactStore{}, nil
	}
	credential, err := common.DecryptObjectStorageCredential(config.CredentialCiphertext)
	if err != nil {
		return nil, err
	}
	if err := system_setting.ValidateObjectStorageConfig(config, credential); err != nil {
		return nil, err
	}
	switch config.Backend {
	case system_setting.ObjectStorageBackendS3:
		return NewS3ArtifactStore(legacyS3Config(config, credential))
	case taskArtifactStoreBackendAzureBlob:
		return NewAzureBlobArtifactStore(config, credential)
	default:
		return nil, errors.New("unsupported object storage backend " + config.Backend)
	}
}

// legacyS3Config 把标准化配置映射为既有 S3 实现的启动配置；签名有效期沿用
// 旧消费者默认值（图片结果仍固定 300 秒，互不影响）。
func legacyS3Config(config system_setting.ObjectStorageConfig, credential string) system_setting.TaskArtifactStoreConfig {
	return system_setting.TaskArtifactStoreConfig{
		Mode:                system_setting.TaskArtifactStoreModeS3,
		S3Endpoint:          config.Endpoint,
		S3Bucket:            config.Bucket,
		S3Region:            config.Region,
		S3AccessKey:         config.AccountName,
		S3SecretKey:         credential,
		S3Prefix:            config.Prefix,
		S3PresignTTLSeconds: system_setting.DefaultTaskArtifactStorePresignTTLSeconds,
	}
}

// BuildObjectStorageEnvImportPreview 读取既有 S3 启动环境变量，生成不含
// 任何密钥的导入预览；没有可导入的环境配置时返回 false。
func BuildObjectStorageEnvImportPreview() (map[string]any, bool) {
	if raw, err := model.GetObjectStorageSetting(); err != nil || raw != "" {
		return nil, false
	}
	config, ok := system_setting.ObjectStorageConfigFromLegacyEnv()
	if !ok {
		return nil, false
	}
	return map[string]any{
		"backend":               config.Backend,
		"endpoint":              config.Endpoint,
		"bucket":                config.Bucket,
		"prefix":                config.Prefix,
		"region":                config.Region,
		"account_name_masked":   system_setting.MaskObjectStorageAccountName(config.AccountName),
		"credential_configured": true,
	}, true
}

// CommitObjectStorageEnvImport 把环境变量配置落库（含加密凭据），作为一次
// 性显式导入。导入完成后数据库是唯一配置源，旧变量不再参与运行期装载，
// 也不再作为失败 fallback。
func CommitObjectStorageEnvImport() (system_setting.ObjectStorageConfig, error) {
	if raw, err := model.GetObjectStorageSetting(); err != nil || raw != "" {
		return system_setting.ObjectStorageConfig{}, errors.New("object storage already configured or unavailable; environment import is only available before initial setup")
	}
	config, ok := system_setting.ObjectStorageConfigFromLegacyEnv()
	if !ok {
		return config, errors.New("no importable object storage environment configuration")
	}
	legacy := system_setting.LoadTaskArtifactStoreConfig()
	ciphertext, err := common.EncryptObjectStorageCredential(legacy.S3SecretKey)
	if err != nil {
		return config, err
	}
	config.CredentialCiphertext = ciphertext
	config.Revision = common.GetUUID()
	if err := saveValidatedObjectStorageSetting(config); err != nil {
		return config, err
	}
	return config, nil
}

// saveValidatedObjectStorageSetting 校验并原子保存完整配置文档；调用方负责
// 先完成连通性验证、加密凭据与 revision 生成。
func saveValidatedObjectStorageSetting(config system_setting.ObjectStorageConfig) error {
	if err := system_setting.ValidateObjectStorageConfig(config, ""); err != nil {
		return err
	}
	data, err := common.Marshal(config)
	if err != nil {
		return err
	}
	return model.SaveObjectStorageSetting(string(data))
}

// SaveObjectStorageSetting 校验并原子保存完整配置文档；连通性验证由调用方
// 在保存前完成，测试失败不落库。
func SaveObjectStorageSetting(config system_setting.ObjectStorageConfig) error {
	return saveValidatedObjectStorageSetting(config)
}
