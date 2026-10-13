package service

func init() {
	accessTokenVerificationScopes[VerificationScopeVideoRefund] = "ops:write"
	accessTokenVerificationScopes[VerificationScopeTaskUsageReview] = "ops:write"
	accessTokenVerificationScopes[VerificationScopeTaskAttemptRecover] = "ops:write"
	accessTokenVerificationScopes[VerificationScopeTaskAttemptReject] = "ops:write"
}
