package assets

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
)

// PluginAssetAdapter pins one compiled artifact for the entire asset
// operation, including group pagination. Credentials and signing remain here.
// The legacy adapter is used only as its host-owned connection/signer state;
// none of its Provider request or response conversion methods are invoked.
type PluginAssetAdapter struct {
	connection  *OfficialActionAdapter
	cmcc        *CMCCAICCV2Adapter
	bearer      client
	plugin      *jsplugin.LoadedPlugin
	protocol    string
	declaration jsplugin.SeedanceAssetConfiguration
}

func NewPluginAssetAdapter(plugin *jsplugin.LoadedPlugin, declaration *jsplugin.SeedanceAssetConfiguration, protocol dto.AssetUpstreamProtocol, baseURL, credential, region, project string, httpClient HTTPDoer) (*PluginAssetAdapter, error) {
	if plugin == nil || plugin.Meta.Key != jsplugin.SeedancePluginKey || declaration == nil || declaration.Protocol != string(protocol) {
		return nil, fmt.Errorf("Seedance asset plugin is unavailable")
	}
	var connection *OfficialActionAdapter
	var err error
	switch protocol {
	case dto.AssetUpstreamProtocolVolcengineAction:
		connection, err = NewVolcengineActionAdapter(credential, project, httpClient)
	case dto.AssetUpstreamProtocolBytePlusAction:
		connection, err = NewBytePlusActionAdapter(credential, region, project, httpClient)
	case dto.AssetUpstreamProtocolCMCCAICCV2:
		connection, err := NewCMCCAICCV2Adapter(credential, httpClient)
		if err != nil {
			return nil, err
		}
		return &PluginAssetAdapter{cmcc: connection, plugin: plugin, protocol: string(protocol), declaration: *declaration}, nil
	case dto.AssetUpstreamProtocolArkAssetsV1, dto.AssetUpstreamProtocolTokenSaveAssetsV1, dto.AssetUpstreamProtocolMoxingVolcAssetsV1, dto.AssetUpstreamProtocolFunCloudMaterial:
		return &PluginAssetAdapter{plugin: plugin, protocol: string(protocol), declaration: *declaration, bearer: newClient(baseURL, credential, httpClient)}, nil
	default:
		return nil, ErrAssetOperationUnsupported
	}
	if err != nil {
		return nil, err
	}
	return &PluginAssetAdapter{connection: connection, plugin: plugin, protocol: string(protocol), declaration: *declaration}, nil
}

func (a *PluginAssetAdapter) Profile() dto.AssetUpstreamProfile {
	return dto.AssetUpstreamProtocol(a.protocol).TransportProfile()
}
func (a *PluginAssetAdapter) Supports(kind, mediaType string) bool {
	for _, media := range a.declaration.Media {
		if media.Kind == kind && media.MediaType == mediaType {
			return true
		}
	}
	return false
}
func (a *PluginAssetAdapter) CanCheckConnectivity() bool { return a.declaration.Connectivity }

func (a *PluginAssetAdapter) CanSearchGroups() bool { return a.declaration.GroupSearch }

func (a *PluginAssetAdapter) GeneralAssetGroupPolicy() dto.GeneralAssetGroupPolicy {
	return dto.GeneralAssetGroupPolicy(a.declaration.GroupPolicy)
}

func (a *PluginAssetAdapter) CreateAsset(ctx context.Context, request AssetRequest) (AssetResult, error) {
	var result AssetResult
	// Source readers, keys and HTTP clients never enter the engine.
	err := a.executeWithSource(ctx, "create_asset", map[string]any{"GroupResourceID": request.GroupResourceID, "URL": request.URL, "Name": request.Name, "MediaType": request.MediaType}, &result, &request)
	return result, err
}
func (a *PluginAssetAdapter) GetAsset(ctx context.Context, id string) (AssetResult, error) {
	if a.protocol == string(dto.AssetUpstreamProtocolFunCloudMaterial) {
		var result AssetResult
		err := a.findMaterial(ctx, "get_asset", id, &result)
		return result, err
	}
	var result AssetResult
	err := a.execute(ctx, "get_asset", map[string]any{"id": id}, &result)
	return result, err
}
func (a *PluginAssetAdapter) UpdateAsset(ctx context.Context, id, name string) (AssetResult, error) {
	var result AssetResult
	err := a.execute(ctx, "update_asset", map[string]any{"id": id, "name": name}, &result)
	return result, err
}
func (a *PluginAssetAdapter) DeleteAsset(ctx context.Context, id string) error {
	err := a.execute(ctx, "delete_asset", map[string]any{"id": id}, nil)
	if a.declaration.DeleteNotFoundIsSuccess && upstreamNotFound(err) {
		return nil
	}
	return err
}
func (a *PluginAssetAdapter) CreateGroup(ctx context.Context, request GroupRequest) (GroupResult, error) {
	var result GroupResult
	err := a.execute(ctx, "create_group", request, &result)
	return result, err
}
func (a *PluginAssetAdapter) GetGroup(ctx context.Context, id string) (GroupResult, error) {
	if a.protocol == string(dto.AssetUpstreamProtocolFunCloudMaterial) {
		var result GroupResult
		err := a.findMaterial(ctx, "get_group", id, &result)
		return result, err
	}
	var result GroupResult
	err := a.execute(ctx, "get_group", map[string]any{"id": id}, &result)
	return result, err
}
func (a *PluginAssetAdapter) ListAssets(ctx context.Context, request AssetListRequest) ([]AssetResult, int, error) {
	var result struct {
		Items []AssetResult
		Total int
	}
	err := a.execute(ctx, "list_assets", request, &result)
	return result.Items, result.Total, err
}
func (a *PluginAssetAdapter) ListGroups(ctx context.Context, request GroupListRequest) ([]GroupResult, int, error) {
	var result struct {
		Items []GroupResult
		Total int
	}
	err := a.execute(ctx, "list_groups", request, &result)
	return result.Items, result.Total, err
}
func (a *PluginAssetAdapter) CheckConnectivity(ctx context.Context) error {
	if !a.CanCheckConnectivity() {
		return ErrAssetOperationUnsupported
	}
	_, _, err := a.ListAssets(ctx, AssetListRequest{GroupType: "AIGC", Page: 1, PageSize: 1})
	return err
}
func (a *PluginAssetAdapter) CreateVerificationSession(ctx context.Context, request VerificationRequest) (VerificationResult, error) {
	var result VerificationResult
	err := a.execute(ctx, "create_verification", request, &result)
	return result, err
}
func (a *PluginAssetAdapter) GetVerificationSession(ctx context.Context, id string) (VerificationResult, error) {
	var result VerificationResult
	err := a.execute(ctx, "get_verification_session", map[string]any{"id": id}, &result)
	return result, err
}
func (a *PluginAssetAdapter) GetVerificationResult(ctx context.Context, id string) (VerificationResult, error) {
	var result VerificationResult
	err := a.execute(ctx, "get_verification", map[string]any{"id": id}, &result)
	return result, err
}

func (a *PluginAssetAdapter) execute(ctx context.Context, operation string, input, result any) error {
	return a.executeWithSource(ctx, operation, input, result, nil)
}

func (a *PluginAssetAdapter) executeWithSource(ctx context.Context, operation string, input, result any, source *AssetRequest) error {
	// This is an authorization boundary: a script cannot sign another operation
	// or inject a host, method, version, credential or arbitrary HTTP header.
	allowedActions := map[string]string{
		"create_asset": "CreateAsset", "get_asset": "GetAsset", "update_asset": "UpdateAsset", "delete_asset": "DeleteAsset",
		"create_group": "CreateAssetGroup", "get_group": "GetAssetGroup", "list_assets": "ListAssets", "list_groups": "ListAssetGroups",
		"create_verification": "CreateVisualValidateSession", "get_verification": "GetVisualValidateResult", "get_verification_session": "GetVisualValidateResult",
	}
	publishedOperations := map[string]string{
		"create_asset": "create_asset", "get_asset": "get_asset", "update_asset": "update_asset", "delete_asset": "delete_asset",
		"create_group": "create_asset_group", "get_group": "get_asset_group", "list_assets": "get_asset", "list_groups": "get_asset_group",
		"create_verification": "get_asset_group_verification", "get_verification": "get_asset_group_verification", "get_verification_session": "get_asset_group_verification",
	}
	if operation == "list_groups" && !a.declaration.GroupSearch {
		return ErrAssetOperationUnsupported
	}
	if !slices.Contains(a.declaration.Operations, publishedOperations[operation]) {
		return ErrAssetOperationUnsupported
	}
	action, ok := allowedActions[operation]
	if !ok {
		return ErrAssetOperationUnsupported
	}
	inputBytes, err := common.Marshal(input)
	if err != nil {
		return err
	}
	var requestInput map[string]any
	if err = common.Unmarshal(inputBytes, &requestInput); err != nil {
		return err
	}
	if id, ok := requestInput["id"].(string); ok {
		requestInput["escapedID"] = url.PathEscape(id)
		requestInput["queryID"] = url.QueryEscape(id)
	}
	args := map[string]any{"operation": operation, "request": requestInput}
	if a.connection != nil {
		args["project"] = a.connection.providerProject
	}
	output, err := a.plugin.Engine.CallPathWithAdmissionTimeout(ctx, 2*time.Second, "seedanceAssets", []string{a.protocol, "buildRequest"}, args)
	if err != nil {
		return fmt.Errorf("Seedance asset request conversion failed")
	}
	descriptor, ok := output.(map[string]any)
	if !ok {
		return fmt.Errorf("Seedance asset request descriptor is invalid")
	}
	if descriptor["unsupported"] == true && len(descriptor) == 1 {
		return ErrAssetOperationUnsupported
	}
	response, err := a.performRequest(ctx, operation, action, descriptor, source)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseLimit := int64(4 << 20)
	if source != nil && a.protocol == string(dto.AssetUpstreamProtocolFunCloudMaterial) {
		responseLimit = funCloudUploadResponseMaxBytes
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, responseLimit+1))
	if err != nil {
		return invalidUpstreamResponse(err)
	}
	if int64(len(responseBody)) > responseLimit {
		return invalidUpstreamResponse(fmt.Errorf("asset response exceeds size limit"))
	}
	args["status"] = response.StatusCode
	args["body"] = string(responseBody)
	args["now"] = time.Now().UTC().Unix()
	if a.cmcc != nil {
		args["now"] = a.cmcc.now().UTC().Unix()
	}
	if a.connection != nil {
		args["now"] = a.connection.now().UTC().Unix()
	}
	output, err = a.plugin.Engine.CallPathWithAdmissionTimeout(ctx, 2*time.Second, "seedanceAssets", []string{a.protocol, "parseResponse"}, args)
	// HTTP status is transport truth even if a Provider returned non-JSON.
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		statusError := &upstreamHTTPError{StatusCode: response.StatusCode}
		if err == nil {
			if root, ok := output.(map[string]any); ok {
				if detail, ok := root["httpError"].(map[string]any); ok {
					if a.cmcc != nil && detail["notFound"] == true && (operation == "get_asset" || operation == "delete_asset") {
						return ErrAssetResourceNotFound
					}
					if code, ok := detail["code"].(string); ok {
						statusError.ProviderCode = sanitizeProviderCodeForDiagnostic(code)
					}
				}
			}
		}
		return statusError
	}
	if err != nil {
		return invalidUpstreamResponse(fmt.Errorf("Seedance asset response conversion failed"))
	}
	normalized, ok := output.(map[string]any)
	if !ok || len(normalized) != 1 {
		return invalidUpstreamResponse(fmt.Errorf("invalid asset plugin response"))
	}
	if normalized["notFound"] == true {
		return ErrAssetResourceNotFound
	}
	if raw, ok := normalized["stringError"]; ok {
		data, err := common.Marshal(raw)
		if err != nil {
			return err
		}
		var detail struct {
			Code       string `json:"code"`
			Definitive bool   `json:"definitive"`
			NotFound   bool   `json:"notFound"`
		}
		if err = common.Unmarshal(data, &detail); err != nil {
			return invalidUpstreamResponse(err)
		}
		return &upstreamStringApplicationError{provider: "Seedance", code: sanitizeProviderCodeForDiagnostic(detail.Code), definitive: detail.Definitive, notFound: detail.NotFound}
	}
	if raw, ok := normalized["applicationError"]; ok {
		data, err := common.Marshal(raw)
		if err != nil {
			return err
		}
		var detail struct {
			Code       int  `json:"code"`
			Definitive bool `json:"definitive"`
		}
		if err = common.Unmarshal(data, &detail); err != nil {
			return invalidUpstreamResponse(err)
		}
		return &upstreamApplicationError{provider: "Seedance", code: detail.Code, definitive: detail.Definitive}
	}
	raw, ok := normalized["result"].(map[string]any)
	if !ok {
		return invalidUpstreamResponse(fmt.Errorf("invalid asset plugin result"))
	}
	if result == nil {
		return nil
	}
	data, err := common.Marshal(raw)
	if err != nil {
		return err
	}
	if err = common.Unmarshal(data, result); err != nil {
		return invalidUpstreamResponse(err)
	}
	return nil
}

// Credentials are injected only after the script has returned a bounded
// relative request. No plugin can change the configured credential origin.
func (a *PluginAssetAdapter) performRequest(ctx context.Context, operation, action string, descriptor map[string]any, source *AssetRequest) (*http.Response, error) {
	var payload []byte
	var err error
	var endpoint, method string
	var httpClient HTTPDoer
	if a.connection != nil {
		if len(descriptor) != 2 || descriptor["action"] != action {
			return nil, fmt.Errorf("invalid asset Action descriptor")
		}
		body, ok := descriptor["body"].(map[string]any)
		if !ok || body["ProjectName"] != a.connection.providerProject {
			return nil, fmt.Errorf("invalid asset Action project")
		}
		payload, err = common.Marshal(body)
		if err != nil {
			return nil, err
		}
		target, err := url.Parse(a.connection.baseURL)
		if err != nil {
			return nil, err
		}
		query := target.Query()
		query.Set("Action", action)
		query.Set("Version", officialActionVersion)
		target.RawQuery = query.Encode()
		endpoint, method, httpClient = target.String(), http.MethodPost, a.connection.http
	} else {
		if len(descriptor) != 3 {
			return nil, fmt.Errorf("invalid asset HTTP descriptor")
		}
		method, _ = descriptor["method"].(string)
		path, _ := descriptor["path"].(string)
		parsed, parseErr := url.Parse(path)
		if parseErr != nil || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || parsed.IsAbs() || parsed.Host != "" || parsed.User != nil || parsed.Fragment != "" || strings.Contains(parsed.Path, "..") || strings.Contains(path, "\\") {
			return nil, fmt.Errorf("invalid asset HTTP path")
		}
		if method != http.MethodGet && method != http.MethodPost && (method != http.MethodPut || operation != "update_asset") && (method != http.MethodDelete || operation != "delete_asset") {
			return nil, fmt.Errorf("invalid asset HTTP method")
		}
		if source != nil && a.protocol == string(dto.AssetUpstreamProtocolFunCloudMaterial) {
			if operation != "create_asset" || method != http.MethodPost {
				return nil, fmt.Errorf("invalid asset upload operation")
			}
			return a.uploadMaterial(ctx, path, descriptor["body"], *source)
		}
		if descriptor["body"] != nil {
			payload, err = common.Marshal(descriptor["body"])
			if err != nil {
				return nil, err
			}
		}
		endpoint, httpClient = a.bearer.baseURL+path, a.bearer.http
		if a.cmcc != nil {
			if parsed.RawQuery != "" {
				return nil, fmt.Errorf("invalid signed asset query")
			}
			target, err := url.Parse(a.cmcc.baseURL + path)
			if err != nil {
				return nil, err
			}
			query, err := a.cmcc.signedQuery(method, target.EscapedPath(), a.cmcc.now().UTC())
			if err != nil {
				return nil, err
			}
			target.RawQuery = query
			endpoint, httpClient = target.String(), a.cmcc.http
		}
	}
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if payload != nil || a.connection != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if a.connection != nil {
		a.connection.sign(req, payload, a.connection.now().UTC())
	} else if a.cmcc == nil {
		req.Header.Set("Authorization", "Bearer "+a.bearer.apiKey)
	}
	response, err := httpClient.Do(req)
	if err != nil {
		return nil, classifyTransportError(AssetStageWaitResponse, err)
	}
	return response, nil
}
