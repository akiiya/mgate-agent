package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"strings"
)

type SignInput struct {
	Method    string
	Path      string
	Timestamp string
	Nonce     string
	Body      []byte
}

func Sign(secret []byte, in SignInput) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(CanonicalString(in)))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func Verify(secret []byte, in SignInput, signature string) bool {
	expected := Sign(secret, in)
	// 验签必须使用常量时间比较，避免通过比较耗时推断签名片段。
	return subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) == 1
}

func BodySHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func CanonicalString(in SignInput) string {
	return strings.ToUpper(in.Method) + "\n" +
		in.Path + "\n" +
		in.Timestamp + "\n" +
		in.Nonce + "\n" +
		BodySHA256(in.Body)
}
