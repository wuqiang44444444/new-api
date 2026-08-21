package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	assetadapter "github.com/QuantumNous/new-api/relay/channel/task/seedance/assets"
	_ "modernc.org/sqlite"
)

const (
	defaultChannelID      = 79
	cmccVideoTaskListPath = "/api/v3/contents/generations/tasks?page_size=1"
	maximumResponseBytes  = 4 << 20
	expectedCustomerModel = "seedance-2.0-cmcc"
)

type channelCredentials struct {
	ID              int
	Status          int
	BaseURL         string
	VideoAPIKey     string
	AssetAccessKey  string
	AssetSecretKey  string
	CustomerModel   string
	ModelMapping    string
	ProviderMapping string
}

type verificationReport struct {
	ChannelID             int    `json:"channel_id"`
	CustomerModel         string `json:"customer_model"`
	ProviderOrigin        string `json:"provider_origin"`
	ChannelManuallyLocked bool   `json:"channel_manually_disabled"`
	VideoAuthenticated    bool   `json:"video_authenticated"`
	AssetAuthenticated    bool   `json:"asset_authenticated"`
	Passed                bool   `json:"passed"`
	ErrorCode             string `json:"error_code,omitempty"`
}

func main() {
	databasePath := flag.String("database", "one-api.db", "SQLite database path")
	channelID := flag.Int("channel-id", defaultChannelID, "CMCC Seedance channel ID")
	timeout := flag.Duration("timeout", 30*time.Second, "overall read-only verification timeout")
	flag.Parse()

	report, err := verify(*databasePath, *channelID, *timeout)
	if err != nil {
		report.ErrorCode = safeVerificationErrorCode(err)
	}
	payload, marshalErr := common.Marshal(report)
	if marshalErr != nil {
		fmt.Fprintln(os.Stderr, `{"passed":false,"error_code":"report_encoding_failed"}`)
		os.Exit(1)
	}
	fmt.Println(string(payload))
	if err != nil || !report.Passed {
		os.Exit(1)
	}
}

func verify(databasePath string, channelID int, timeout time.Duration) (verificationReport, error) {
	report := verificationReport{ChannelID: channelID, CustomerModel: expectedCustomerModel}
	credential, err := readChannelCredentials(databasePath, channelID)
	if err != nil {
		return report, err
	}
	report.ProviderOrigin = credential.BaseURL
	report.ChannelManuallyLocked = credential.Status == 2
	if !report.ChannelManuallyLocked {
		return report, errors.New("channel_not_manually_disabled")
	}
	if credential.CustomerModel != expectedCustomerModel ||
		credential.ProviderMapping != "doubao-seedance-2.0" {
		return report, errors.New("channel_contract_mismatch")
	}
	if credential.VideoAPIKey == "" || credential.AssetAccessKey == "" || credential.AssetSecretKey == "" {
		return report, errors.New("credentials_not_configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	client := &http.Client{Timeout: timeout}
	report.VideoAuthenticated, err = verifyVideoCredential(ctx, client, credential.BaseURL, credential.VideoAPIKey)
	if err != nil {
		return report, err
	}
	asset, err := assetadapter.NewCMCCAICCV2Adapter(credential.AssetAccessKey+"|"+credential.AssetSecretKey, client)
	if err != nil {
		return report, errors.New("asset_credential_invalid")
	}
	if err := asset.CheckConnectivity(ctx); err != nil {
		if diagnostic, ok := assetadapter.SafeUpstreamDiagnostic(err); ok {
			return report, fmt.Errorf("asset_%s", diagnostic)
		}
		return report, errors.New("asset_connectivity_failed")
	}
	report.AssetAuthenticated = true
	report.Passed = report.VideoAuthenticated && report.AssetAuthenticated
	return report, nil
}

func readChannelCredentials(databasePath string, channelID int) (channelCredentials, error) {
	absolutePath, err := filepath.Abs(strings.TrimSpace(databasePath))
	if err != nil {
		return channelCredentials{}, errors.New("database_path_invalid")
	}
	database, err := sql.Open("sqlite", "file:"+filepath.ToSlash(absolutePath)+"?mode=ro")
	if err != nil {
		return channelCredentials{}, errors.New("database_open_failed")
	}
	defer database.Close()
	var credential channelCredentials
	err = database.QueryRow(`
		SELECT c.id, c.status, c.base_url, c.key,
		       COALESCE(a.access_key_id, ''), COALESCE(a.secret_access_key, ''),
		       c.models, c.model_mapping
		FROM channels c
		LEFT JOIN channel_asset_credentials a ON a.channel_id = c.id
		WHERE c.id = ? AND c.type = 61
	`, channelID).Scan(
		&credential.ID, &credential.Status, &credential.BaseURL, &credential.VideoAPIKey,
		&credential.AssetAccessKey, &credential.AssetSecretKey, &credential.CustomerModel,
		&credential.ModelMapping,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return channelCredentials{}, errors.New("channel_not_found")
	}
	if err != nil {
		return channelCredentials{}, errors.New("database_query_failed")
	}
	credential.BaseURL = strings.TrimRight(strings.TrimSpace(credential.BaseURL), "/")
	credential.VideoAPIKey = strings.TrimSpace(credential.VideoAPIKey)
	credential.AssetAccessKey = strings.TrimSpace(credential.AssetAccessKey)
	credential.AssetSecretKey = strings.TrimSpace(credential.AssetSecretKey)
	credential.CustomerModel = strings.TrimSpace(credential.CustomerModel)
	var mapping map[string]string
	if err := common.UnmarshalJsonStr(credential.ModelMapping, &mapping); err != nil {
		return channelCredentials{}, errors.New("channel_model_mapping_invalid")
	}
	credential.ProviderMapping = strings.TrimSpace(mapping[expectedCustomerModel])
	return credential, nil
}

func verifyVideoCredential(ctx context.Context, client *http.Client, baseURL, apiKey string) (bool, error) {
	endpoint, err := url.Parse(strings.TrimRight(baseURL, "/") + cmccVideoTaskListPath)
	if err != nil || endpoint.Host == "" ||
		(endpoint.Scheme != "http" && endpoint.Scheme != "https") {
		return false, errors.New("video_origin_invalid")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return false, errors.New("video_request_failed")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+apiKey)
	response, err := client.Do(request)
	if err != nil {
		return false, errors.New("video_connectivity_failed")
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maximumResponseBytes+1))
	if err != nil || len(payload) > maximumResponseBytes {
		return false, errors.New("video_response_invalid")
	}
	if response.StatusCode != http.StatusOK {
		return false, fmt.Errorf("video_http_%d", response.StatusCode)
	}
	if err := validateVideoListResponse(payload); err != nil {
		return false, err
	}
	return true, nil
}

func validateVideoListResponse(payload []byte) error {
	var response struct {
		Total *int              `json:"total"`
		Items []json.RawMessage `json:"items"`
	}
	if err := common.Unmarshal(payload, &response); err != nil || response.Total == nil || *response.Total < 0 {
		return errors.New("video_response_invalid")
	}
	return nil
}

func safeVerificationErrorCode(err error) string {
	code := strings.TrimSpace(err.Error())
	if len(code) == 0 || len(code) > 160 {
		return "verification_failed"
	}
	for _, character := range code {
		if character >= 'a' && character <= 'z' || character >= '0' && character <= '9' ||
			character == '_' || character == '-' || character == '=' || character == '.' {
			continue
		}
		return "verification_failed"
	}
	return code
}
