package system_setting

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

// 对象存储标准化配置（本地扩展）。完整配置以单行 JSON 持久化在数据库
// Option 表（ObjectStorageSettingOptionKey），由 service 层在数据库就绪后
// 装载并原子替换存储实例；启动环境变量不再是运行期配置源，只作为一次性
// 显式导入的预览来源。该命名空间不进入通用 OptionMap。

const (
	ObjectStorageBackendUpstream  = "upstream"
	ObjectStorageBackendS3        = "s3"
	ObjectStorageBackendAzureBlob = "azure_blob"

	// ObjectStorageSettingOptionKey 是对象存储专用 Option 键。该命名空间
	// 只能通过专用最高管理员接口读写；普通选项接口不得读取或绕过校验写入。
	ObjectStorageSettingOptionKey = "ObjectStorageSetting"

	// AzureConnectionInputMode / ManualConnectionInputMode 是 Azure 连接
	// 信息的两种输入格式；连接字符串只作为输入格式，解析后只保存标准化字段。
	AzureConnectionInputMode = "connection_string"
	ManualConnectionInputMode = "manual"
)

var (
	objectStorageAzureContainerPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
	objectStorageAzureAccountPattern   = regexp.MustCompile(`^[a-z0-9]{3,24}$`)
	objectStorageSuffixPattern         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*$`)
)

// ObjectStorageConfig 是持久化的标准化配置文档。account_name 与凭据由
// backend 决定含义：azure_blob 下为 Storage Account Name + Account Key，
// s3 下为 Access Key ID + Secret Access Key；不按名称推断协议。
type ObjectStorageConfig struct {
	Backend     string `json:"backend"`
	Endpoint    string `json:"endpoint,omitempty"`
	Bucket      string `json:"bucket,omitempty"` // Azure 下为 Container
	Prefix      string `json:"prefix,omitempty"`
	Region      string `json:"region,omitempty"` // 仅 S3 使用
	AccountName string `json:"account_name,omitempty"`
	// CredentialCiphertext 是加密后的 Account Key / Secret Key；明文凭据
	// 不落库、不进入通用可读 OptionMap。
	CredentialCiphertext string `json:"credential_ciphertext,omitempty"`
	// Revision 在每次完整保存时重新生成，用于多节点缓存刷新判定。
	Revision string `json:"revision"`
	// 以下为审计元数据，不参与运行期合同。
	LastTestStatus string `json:"last_test_status,omitempty"`
	LastTestAt     int64  `json:"last_test_at,omitempty"`
}

// AzureStorageAccount 是连接字符串解析后的唯一标准化凭据表达。
type AzureStorageAccount struct {
	AccountName  string
	AccountKey   string
	BlobEndpoint string
}

// ParseAzureBlobConnectionString 把 Azure 连接字符串解析为标准化账号信息。
// 属性按首个等号拆分并保留 Base64 Key 尾部等号；冲突重复属性、SharedKey
// 之外的鉴权方式（SAS、开发存储模拟器）都显式拒绝，不静默解释为另一账号。
// 含自定义 BlobEndpoint 时使用该显式端点，否则按
// DefaultEndpointsProtocol/EndpointSuffix 构造默认端点。
func ParseAzureBlobConnectionString(raw string) (*AzureStorageAccount, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("connection string is empty")
	}
	properties := make(map[string]string)
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ';' || r == '\n' || r == '\r'
	}) {
		index := strings.Index(part, "=")
		if index <= 0 {
			return nil, errors.New("connection string property is invalid")
		}
		name := strings.ToLower(strings.TrimSpace(part[:index]))
		// 保留取值中的 Base64 尾部等号；仅去除两侧空白。
		value := strings.TrimSpace(part[index+1:])
		if previous, exists := properties[name]; exists && previous != value {
			return nil, fmt.Errorf("connection string has conflicting property %q", name)
		}
		properties[name] = value
	}

	if _, exists := properties["sharedaccesssignature"]; exists {
		return nil, errors.New("only SharedKey connection strings are supported")
	}
	if _, exists := properties["usedevelopmentstorage"]; exists {
		return nil, errors.New("development storage connection strings are not supported")
	}

	accountName := properties["accountname"]
	if !objectStorageAzureAccountPattern.MatchString(accountName) {
		return nil, errors.New("connection string is missing a valid AccountName")
	}
	accountKey := properties["accountkey"]
	if accountKey == "" {
		return nil, errors.New("connection string is missing AccountKey")
	}
	if _, err := base64.StdEncoding.DecodeString(accountKey); err != nil {
		return nil, errors.New("AccountKey is not valid base64")
	}

	endpoint := properties["blobendpoint"]
	if endpoint != "" {
		if err := validateObjectStorageEndpoint(endpoint); err != nil {
			return nil, fmt.Errorf("BlobEndpoint is invalid: %s", err.Error())
		}
	} else {
		protocol := properties["defaultendpointsprotocol"]
		if protocol != "" && protocol != "https" && protocol != "http" {
			return nil, errors.New("DefaultEndpointsProtocol must be http or https")
		}
		if protocol == "" {
			protocol = "https"
		}
		suffix := properties["endpointsuffix"]
		if suffix != "" {
			if !objectStorageSuffixPattern.MatchString(suffix) {
				return nil, errors.New("EndpointSuffix is invalid")
			}
		} else {
			suffix = "core.windows.net"
		}
		endpoint = fmt.Sprintf("%s://%s.blob.%s", protocol, accountName, suffix)
	}

	return &AzureStorageAccount{
		AccountName:  accountName,
		AccountKey:   accountKey,
		BlobEndpoint: endpoint,
	}, nil
}

// ValidateObjectStorageConfig 校验标准化配置语法。credential 是明文凭据；
// 为空表示沿用已保存密文（此时要求密文非空）。只做语法检查，不做网络访问。
func ValidateObjectStorageConfig(config ObjectStorageConfig, credential string) error {
	switch config.Backend {
	case ObjectStorageBackendUpstream:
		return nil
	case ObjectStorageBackendS3, ObjectStorageBackendAzureBlob:
	default:
		return fmt.Errorf("unsupported object storage backend %q", config.Backend)
	}

	if config.Endpoint == "" {
		return errors.New("storage endpoint is required")
	}
	if err := validateObjectStorageEndpoint(config.Endpoint); err != nil {
		return err
	}
	if config.Bucket == "" {
		if config.Backend == ObjectStorageBackendAzureBlob {
			return errors.New("storage container is required")
		}
		return errors.New("storage bucket is required")
	}
	if config.Backend == ObjectStorageBackendAzureBlob {
		if err := validateAzureContainerName(config.Bucket); err != nil {
			return err
		}
	} else {
		if !taskArtifactStoreBucketPattern.MatchString(config.Bucket) ||
			strings.Contains(config.Bucket, "..") || net.ParseIP(config.Bucket) != nil {
			return errors.New("storage bucket syntax is invalid")
		}
	}
	if config.Backend == ObjectStorageBackendS3 {
		if config.Region == "" {
			return errors.New("storage region is required")
		}
		if !taskArtifactStoreRegionPattern.MatchString(config.Region) {
			return errors.New("storage region syntax is invalid")
		}
	}
	if config.AccountName == "" {
		return errors.New("storage account name is required")
	}
	if config.Backend == ObjectStorageBackendAzureBlob {
		if !objectStorageAzureAccountPattern.MatchString(config.AccountName) {
			return errors.New("storage account name syntax is invalid")
		}
	} else if len(config.AccountName) > 256 || config.AccountName != strings.TrimSpace(config.AccountName) {
		return errors.New("storage account name syntax is invalid")
	}
	if credential == "" && config.CredentialCiphertext == "" {
		return errors.New("storage credential is required")
	}
	if err := validateObjectStorageSecret(credential); err != nil {
		return err
	}
	if err := validateObjectStoragePrefix(config.Prefix); err != nil {
		return err
	}
	return nil
}

func validateAzureContainerName(name string) error {
	if len(name) < 3 || len(name) > 63 || !objectStorageAzureContainerPattern.MatchString(name) ||
		strings.Contains(name, "--") {
		return errors.New("storage container name syntax is invalid")
	}
	return nil
}

func validateObjectStorageSecret(value string) error {
	if value == "" {
		return nil
	}
	if value != strings.TrimSpace(value) || len(value) > 1024 {
		return errors.New("storage credential syntax is invalid")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return errors.New("storage credential syntax is invalid")
		}
	}
	return nil
}

func validateObjectStoragePrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	if prefix != strings.TrimSpace(prefix) || len(prefix) > 512 ||
		strings.HasPrefix(prefix, "/") || strings.Contains(prefix, "\\") {
		return errors.New("storage prefix syntax is invalid")
	}
	for _, part := range strings.Split(prefix, "/") {
		if part == "." || part == ".." {
			return errors.New("storage prefix must not contain dot segments")
		}
	}
	for _, character := range prefix {
		if unicode.IsControl(character) {
			return errors.New("storage prefix must not contain control characters")
		}
	}
	return nil
}

func validateObjectStorageEndpoint(raw string) error {
	if raw != strings.TrimSpace(raw) {
		return errors.New("storage endpoint must not contain surrounding whitespace")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || parsed.Host == "" || parsed.User != nil || parsed.Opaque != "" {
		return errors.New("storage endpoint must be an absolute URL without userinfo")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("storage endpoint must use http or https")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return errors.New("storage endpoint must not contain a query or fragment")
	}
	return nil
}

// MaskObjectStorageAccountName 输出脱敏账号显示值，只保留末 4 位。
func MaskObjectStorageAccountName(name string) string {
	if len(name) <= 4 {
		return "****"
	}
	return "****" + name[len(name)-4:]
}

// ObjectStorageConfigFromLegacyEnv 把既有 S3 启动环境变量映射为标准化配置，
// 仅用于一次性显式导入；数据库已保存配置时环境变量不再参与运行期装载。
func ObjectStorageConfigFromLegacyEnv() (ObjectStorageConfig, bool) {
	legacy := LoadTaskArtifactStoreConfig()
	if legacy.Mode != TaskArtifactStoreModeS3 {
		return ObjectStorageConfig{}, false
	}
	return ObjectStorageConfig{
		Backend:     ObjectStorageBackendS3,
		Endpoint:    legacy.S3Endpoint,
		Bucket:      legacy.S3Bucket,
		Prefix:      legacy.S3Prefix,
		Region:      legacy.S3Region,
		AccountName: legacy.S3AccessKey,
	}, true
}
