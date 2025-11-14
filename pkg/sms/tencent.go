package sms

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/cloudreve/Cloudreve/v4/pkg/request"
)

// TencentSMSProvider 腾讯云短信服务提供商
type TencentSMSProvider struct {
	secretID     string
	secretKey    string
	sdkAppID     string
	signName     string
	templateID   string
	logger       logging.Logger
	requestClient request.Client
}

// TencentSMSConfig 腾讯云短信配置
type TencentSMSConfig struct {
	SecretID   string
	SecretKey  string
	SDKAppID   string
	SignName   string
	TemplateID string
}

// NewTencentSMSProvider 创建腾讯云短信服务提供商
func NewTencentSMSProvider(config TencentSMSConfig, logger logging.Logger, requestClient request.Client) SMSProvider {
	return &TencentSMSProvider{
		secretID:      config.SecretID,
		secretKey:     config.SecretKey,
		sdkAppID:      config.SDKAppID,
		signName:      config.SignName,
		templateID:    config.TemplateID,
		logger:        logger,
		requestClient: requestClient,
	}
}

// Send 发送短信
func (t *TencentSMSProvider) Send(ctx context.Context, phone, code string) error {
	endpoint := "https://sms.tencentcloudapi.com"
	action := "SendSms"
	version := "2021-01-11"
	service := "sms"

	timestamp := time.Now().Unix()
	date := time.Now().UTC().Format("2006-01-02")

	// 构建请求参数
	requestPayload := map[string]interface{}{
		"PhoneNumberSet":   []string{phone},
		"SmsSdkAppId":      t.sdkAppID,
		"TemplateId":       t.templateID,
		"SignName":         t.signName,
		"TemplateParamSet": []string{code},
	}

	payloadBytes, err := json.Marshal(requestPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// 构建规范请求
	canonicalRequest := fmt.Sprintf("%s\n%s\n/\n", http.MethodPost, "/")
	canonicalHeaders := fmt.Sprintf("content-type:application/json; charset=utf-8\nhost:sms.tencentcloudapi.com\n")
	signedHeaders := "content-type;host"
	hashedPayload := sha256Hash(string(payloadBytes))
	canonicalRequest += canonicalHeaders + "\n" + signedHeaders + "\n" + hashedPayload

	// 构建待签名字符串
	algorithm := "TC3-HMAC-SHA256"
	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, service)
	stringToSign := fmt.Sprintf("%s\n%d\n%s\n%s", algorithm, timestamp, credentialScope, sha256Hash(canonicalRequest))

	// 计算签名
	secretDate := hmacSha256([]byte("TC3"+t.secretKey), date)
	secretService := hmacSha256(secretDate, service)
	secretSigning := hmacSha256(secretService, "tc3_request")
	signature := fmt.Sprintf("%x", hmacSha256(secretSigning, stringToSign))

	// 构建Authorization
	authorization := fmt.Sprintf("%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm, t.secretID, credentialScope, signedHeaders, signature)

	// 发送请求
	headers := map[string]string{
		"Content-Type":  "application/json; charset=utf-8",
		"Host":          "sms.tencentcloudapi.com",
		"X-TC-Action":   action,
		"X-TC-Version":  version,
		"X-TC-Timestamp": fmt.Sprintf("%d", timestamp),
		"Authorization": authorization,
	}

	resp := t.requestClient.Request(http.MethodPost, endpoint, strings.NewReader(string(payloadBytes)),
		request.WithContext(ctx),
		request.WithLogger(t.logger),
		request.WithHeader(http.Header{
			"Content-Type":  []string{headers["Content-Type"]},
			"X-TC-Action":   []string{headers["X-TC-Action"]},
			"X-TC-Version":  []string{headers["X-TC-Version"]},
			"X-TC-Timestamp": []string{headers["X-TC-Timestamp"]},
			"Authorization": []string{headers["Authorization"]},
		}),
	).CheckHTTPResponse(http.StatusOK)

	if resp.Err != nil {
		return fmt.Errorf("failed to send SMS: %w", resp.Err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp.Response), &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if result["Response"] != nil {
		response := result["Response"].(map[string]interface{})
		if response["Error"] != nil {
			errorInfo := response["Error"].(map[string]interface{})
			return fmt.Errorf("SMS send failed: %s", errorInfo["Message"])
		}
	}

	return nil
}

// sha256Hash 计算SHA256哈希
func sha256Hash(data string) string {
	hash := sha256.Sum256([]byte(data))
	return fmt.Sprintf("%x", hash)
}

// hmacSha256 计算HMAC-SHA256
func hmacSha256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

