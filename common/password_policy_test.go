package common

import (
	"errors"
	"testing"
)

func TestValidateNewPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{name: "valid", password: "P@ssw0rd!", wantErr: nil},
		{name: "too short", password: "P@1", wantErr: ErrPasswordLength},
		{name: "too long", password: "P@ssw0rd!P@ssw0rd!xxx", wantErr: ErrPasswordLength},
		{name: "missing symbol", password: "Password1", wantErr: ErrPasswordComplexity},
		{name: "missing digit", password: "Password!", wantErr: ErrPasswordComplexity},
		{name: "missing letter", password: "12345678!", wantErr: ErrPasswordComplexity},
		{name: "contains space", password: "P@ss w0rd!", wantErr: ErrPasswordComplexity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNewPassword(tt.password)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("ValidateNewPassword() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ValidateNewPassword() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
