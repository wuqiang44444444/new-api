package common

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	PasswordMinLength = 8
	PasswordMaxLength = 20
)

var (
	ErrPasswordLength     = errors.New("密码长度需为8-20个字符")
	ErrPasswordComplexity = errors.New("密码需包含字母、数字和符号，且不能包含空格")
)

func GenerateHMACWithKey(key []byte, data string) string {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func GenerateHMAC(data string) string {
	h := hmac.New(sha256.New, []byte(CryptoSecret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func Password2Hash(password string) (string, error) {
	passwordBytes := []byte(password)
	hashedPassword, err := bcrypt.GenerateFromPassword(passwordBytes, bcrypt.DefaultCost)
	return string(hashedPassword), err
}

func ValidatePasswordAndHash(password string, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func ValidateNewPassword(password string) error {
	length := utf8.RuneCountInString(password)
	if length < PasswordMinLength || length > PasswordMaxLength {
		return ErrPasswordLength
	}

	hasLetter := false
	hasDigit := false
	hasSymbol := false
	for _, r := range password {
		if unicode.IsSpace(r) {
			return ErrPasswordComplexity
		}
		switch {
		case unicode.IsLetter(r):
			hasLetter = true
		case unicode.IsDigit(r):
			hasDigit = true
		default:
			hasSymbol = true
		}
	}
	if !hasLetter || !hasDigit || !hasSymbol {
		return ErrPasswordComplexity
	}
	return nil
}
