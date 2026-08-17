package model

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUserEmailFormatValidation 保护 User.Email 的校验合同：非空 email 必须是合法格式，
// 空值保持合法（存量无邮箱用户与管理员编辑场景）。Register、UpdateUser、CreateUser
// 都通过 common.Validate.Struct 走到这里，因此直调 API 无法写入非法 email。
func TestUserEmailFormatValidation(t *testing.T) {
	cases := []struct {
		name      string
		email     string
		wantValid bool
	}{
		{"valid email", "user@example.com", true},
		{"valid email with local part symbols", "user.name+tag@example.com", true},
		{"empty email allowed for legacy users", "", true},
		{"missing at sign", "not-an-email", false},
		{"missing domain", "a@", false},
		{"missing local part", "@example.com", false},
		{"contains space", "a b@example.com", false},
		{"exceeds max length", strings.Repeat("a", 45) + "@example.com", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user := User{Username: "tester", Password: "password123", Email: tc.email}
			err := common.Validate.Struct(&user)
			if tc.wantValid {
				assert.NoErrorf(t, err, "email %q should pass validation", tc.email)
				return
			}
			require.Errorf(t, err, "email %q should fail validation", tc.email)
			assert.Contains(t, err.Error(), "Email")
		})
	}
}
