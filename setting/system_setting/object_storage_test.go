package system_setting

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAzureBlobConnectionString(t *testing.T) {
	t.Run("default endpoint from protocol and suffix", func(t *testing.T) {
		account, err := ParseAzureBlobConnectionString(
			"DefaultEndpointsProtocol=https;AccountName=storeadmin;AccountKey=YWJjZGVmZ2hpamtsbW5vcA==;EndpointSuffix=core.windows.net")
		require.NoError(t, err)
		assert.Equal(t, "storeadmin", account.AccountName)
		// Base64 尾部等号必须原样保留。
		assert.Equal(t, "YWJjZGVmZ2hpamtsbW5vcA==", account.AccountKey)
		assert.Equal(t, "https://storeadmin.blob.core.windows.net", account.BlobEndpoint)
	})

	t.Run("explicit BlobEndpoint wins and is preserved", func(t *testing.T) {
		account, err := ParseAzureBlobConnectionString(
			"DefaultEndpointsProtocol=https;AccountName=storeadmin;AccountKey=YWJjZA==;BlobEndpoint=https://storeadmin.blob.chinacloudapi.cn/;EndpointSuffix=core.windows.net")
		require.NoError(t, err)
		// 自定义 BlobEndpoint 必须显式生效，不能忽略后改用默认端点。
		assert.Equal(t, "https://storeadmin.blob.chinacloudapi.cn/", account.BlobEndpoint)
	})

	t.Run("rejects conflicting duplicate property", func(t *testing.T) {
		_, err := ParseAzureBlobConnectionString(
			"AccountName=storeadmin;AccountName=otheradmin;AccountKey=YWJjZA==")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflicting property")
	})

	t.Run("rejects SAS credentials", func(t *testing.T) {
		_, err := ParseAzureBlobConnectionString(
			"AccountName=storeadmin;SharedAccessSignature=sv=2020-02-10&sig=abc")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "SharedKey")
	})

	t.Run("rejects missing account key and invalid base64", func(t *testing.T) {
		_, err := ParseAzureBlobConnectionString("AccountName=storeadmin")
		require.Error(t, err)
		_, err = ParseAzureBlobConnectionString("AccountName=storeadmin;AccountKey=%%%not-base64%%%")
		require.Error(t, err)
	})

	t.Run("rejects invalid account name", func(t *testing.T) {
		_, err := ParseAzureBlobConnectionString("AccountName=Bad_Account;AccountKey=YWJjZA==")
		require.Error(t, err)
	})
}

func TestValidateObjectStorageConfig(t *testing.T) {
	validAzure := ObjectStorageConfig{
		Backend:     ObjectStorageBackendAzureBlob,
		Endpoint:    "https://storeadmin.blob.core.windows.net",
		Bucket:      "task-artifacts",
		AccountName: "storeadmin",
	}

	t.Run("upstream accepts empty configuration", func(t *testing.T) {
		require.NoError(t, ValidateObjectStorageConfig(ObjectStorageConfig{Backend: ObjectStorageBackendUpstream}, ""))
	})

	t.Run("azure blob requires endpoint container account and credential", func(t *testing.T) {
		require.NoError(t, ValidateObjectStorageConfig(validAzure, "a2V5"))

		// 密文非空即视为已配置；两者皆空才是不完整配置。
		withCiphertext := validAzure
		withCiphertext.CredentialCiphertext = "objstore.v1.c2FtcGxl"
		require.NoError(t, ValidateObjectStorageConfig(withCiphertext, ""))
		require.Error(t, ValidateObjectStorageConfig(validAzure, ""))

		noEndpoint := validAzure
		noEndpoint.Endpoint = ""
		require.Error(t, ValidateObjectStorageConfig(noEndpoint, "a2V5"))

		noContainer := validAzure
		noContainer.Bucket = ""
		require.Error(t, ValidateObjectStorageConfig(noContainer, "a2V5"))
	})

	t.Run("azure container name rules", func(t *testing.T) {
		bad := []string{"ab", " UPPER", "up--per", "-lead", "trail-", "toolong" +
			strings.Repeat("x", 64)}
		for _, name := range bad {
			config := validAzure
			config.Bucket = name
			assert.Error(t, ValidateObjectStorageConfig(config, "a2V5"), name)
		}
	})

	t.Run("s3 requires region and bucket syntax", func(t *testing.T) {
		config := ObjectStorageConfig{
			Backend:     ObjectStorageBackendS3,
			Endpoint:    "https://s3.example.com",
			Bucket:      "task-artifacts",
			Region:      "us-east-1",
			AccountName: "AKIAIOSFODNN7EXAMPLE",
		}
		require.NoError(t, ValidateObjectStorageConfig(config, "secret"))

		noRegion := config
		noRegion.Region = ""
		require.Error(t, ValidateObjectStorageConfig(noRegion, "secret"))
	})

	t.Run("rejects unsupported backend", func(t *testing.T) {
		require.Error(t, ValidateObjectStorageConfig(ObjectStorageConfig{Backend: "gcs"}, "key"))
	})

	t.Run("rejects invalid endpoint and prefix", func(t *testing.T) {
		badEndpoint := validAzure
		badEndpoint.Endpoint = "not-a-url"
		require.Error(t, ValidateObjectStorageConfig(badEndpoint, "a2V5"))

		badPrefix := validAzure
		badPrefix.Prefix = "/absolute"
		require.Error(t, ValidateObjectStorageConfig(badPrefix, "a2V5"))

		dotPrefix := validAzure
		dotPrefix.Prefix = "a/../b"
		require.Error(t, ValidateObjectStorageConfig(dotPrefix, "a2V5"))
	})
}

func TestMaskObjectStorageAccountName(t *testing.T) {
	assert.Equal(t, "****", MaskObjectStorageAccountName(""))
	assert.Equal(t, "****", MaskObjectStorageAccountName("abc"))
	assert.Equal(t, "****dmin", MaskObjectStorageAccountName("storeadmin"))
}

func TestObjectStorageConfigFromLegacyEnv(t *testing.T) {
	t.Run("no env configuration", func(t *testing.T) {
		_, ok := ObjectStorageConfigFromLegacyEnv()
		assert.False(t, ok)
	})

	t.Run("s3 env mapping", func(t *testing.T) {
		t.Setenv(TaskArtifactStoreModeEnv, TaskArtifactStoreModeS3)
		t.Setenv(TaskArtifactStoreS3EndpointEnv, "https://s3.example.com")
		t.Setenv(TaskArtifactStoreS3BucketEnv, "task-artifacts")
		t.Setenv(TaskArtifactStoreS3RegionEnv, "us-east-1")
		t.Setenv(TaskArtifactStoreS3AccessKeyEnv, "AKIAIOSFODNN7EXAMPLE")
		t.Setenv(TaskArtifactStoreS3SecretKeyEnv, "wJalrXUtnFEMI")
		config, ok := ObjectStorageConfigFromLegacyEnv()
		require.True(t, ok)
		assert.Equal(t, ObjectStorageBackendS3, config.Backend)
		assert.Equal(t, "https://s3.example.com", config.Endpoint)
		assert.Equal(t, "us-east-1", config.Region)
		assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", config.AccountName)
		// 环境变量密钥不进入标准化配置文档。
		assert.Empty(t, config.CredentialCiphertext)
	})
}
