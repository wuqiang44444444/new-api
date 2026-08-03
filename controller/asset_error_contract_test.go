package controller

import (
	"errors"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
)

func TestAssetServiceErrorContractDoesNotExposeInternalFailuresAsClientErrors(t *testing.T) {
	assert.Equal(t, http.StatusBadRequest, assetServiceStatus(service.ErrInvalidAssetRequest))
	assert.Equal(t, "invalid_request", assetServiceCode(service.ErrInvalidAssetRequest))
	assert.Equal(t, http.StatusInternalServerError, assetServiceStatus(errors.New("database unavailable")))
	assert.Equal(t, "internal_error", assetServiceCode(errors.New("database unavailable")))
	assert.Equal(t, http.StatusForbidden, assetServiceStatus(service.ErrRealPersonVerificationRejected))
	assert.Equal(t, "real_person_verification_rejected", assetServiceCode(service.ErrRealPersonVerificationRejected))
}

func TestRealPersonAuthorizationResponseIncludesStableErrorCode(t *testing.T) {
	response := realPersonAuthorizationResponse(&model.RealPersonAuthorization{PublicID: "rpa_test", Status: model.RealPersonAuthorizationFailed, ErrorCode: "real_person_verification_rejected"})
	assert.Equal(t, "real_person_verification_rejected", response.ErrorCode)
}
