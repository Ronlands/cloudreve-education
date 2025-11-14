package sms

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/cloudreve/Cloudreve/v4/pkg/request"
)

// AliyunSMSProvider 阿里云短信服务提供商
type AliyunSMSProvider struct {
	accessKeyID     string
	accessKeySecret string
	signName        string
	templateCode    string
	logger          logging.Logger
	requestClient   request.Client
}

// AliyunSMSConfig 阿里云短信配置
type AliyunSMSConfig struct {
	AccessKeyID     string
	AccessKeySecret string
	SignName        string
	TemplateCode    string
}

// NewAliyunSMSProvider 创建阿里云短信服务提供商
func NewAliyunSMSProvider(config AliyunSMSConfig, logger logging.Logger, requestClient request.Client) SMSProvider {
	return &AliyunSMSProvider{
		accessKeyID:     config.AccessKeyID,
		accessKeySecret: config.AccessKeySecret,
		signName:        config.SignName,
		templateCode:    config.TemplateCode,
		logger:          logger,
		requestClient:   requestClient,
	}
}

// Send 发送短信
func (a *AliyunSMSProvider) Send(ctx context.Context, phone, code string) error {
	endpoint := "https://dysmsapi.aliyuncs.com"
	action := "SendSms"
	version := "2017-05-25"

	params := map[string]string{
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureNonce":   fmt.Sprintf("%d", time.Now().UnixNano()),
		"AccessKeyId":      a.accessKeyID,
		"SignatureVersion": "1.0",
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"Format":           "JSON",
		"Action":           action,
		"Version":           version,
		"RegionId":         "cn-hangzhou",
		"PhoneNumbers":     phone,
		"SignName":         a.signName,
		"TemplateCode":     a.templateCode,
		"TemplateParam":    fmt.Sprintf(`{"code":"%s"}`, code),
	}

	// 生成签名
	signature := a.generateSignature(params, "POST")
	params["Signature"] = signature

	// 构建请求URL
	query := url.Values{}
	for k, v := range params {
		query.Set(k, v)
	}

	reqURL := endpoint + "?" + query.Encode()

	// 发送请求
	resp := a.requestClient.Request(http.MethodGet, reqURL, nil,
		request.WithContext(ctx),
		request.WithLogger(a.logger),
	).CheckHTTPResponse(http.StatusOK)

	if resp.Err != nil {
		return fmt.Errorf("failed to send SMS: %w", resp.Err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(resp.Response), &result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	if result["Code"] != "OK" {
		return fmt.Errorf("SMS send failed: %s", result["Message"])
	}

	return nil
}

// generateSignature 生成签名
func (a *AliyunSMSProvider) generateSignature(params map[string]string, method string) string {
	// 排序参数
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 构建查询字符串
	var queryParts []string
	for _, k := range keys {
		queryParts = append(queryParts, fmt.Sprintf("%s=%s", percentEncode(k), percentEncode(params[k])))
	}
	queryString := strings.Join(queryParts, "&")

	// 构建待签名字符串
	stringToSign := method + "&" + percentEncode("/") + "&" + percentEncode(queryString)

	// 计算HMAC-SHA1签名
	mac := hmac.New(sha1.New, []byte(a.accessKeySecret+"&"))
	mac.Write([]byte(stringToSign))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	return signature
}

// percentEncode URL编码
func percentEncode(s string) string {
	return url.QueryEscape(s)
}

