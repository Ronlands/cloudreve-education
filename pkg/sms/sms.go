package sms

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/pkg/cache"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/cloudreve/Cloudreve/v4/pkg/request"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
)

const (
	// SMS验证码缓存前缀
	smsCodePrefix = "sms_code_"
	// 验证码有效期（秒）
	smsCodeTTL = 300 // 5分钟
	// 验证码发送间隔（秒）
	smsCodeInterval = 60 // 1分钟
)

// SMSProvider 短信服务提供者接口
type SMSProvider interface {
	// Send 发送短信验证码
	Send(ctx context.Context, phone, code string) error
}

// SMSService 短信验证码服务
type SMSService struct {
	kv       cache.Driver
	logger   logging.Logger
	provider SMSProvider
}

// NewSMSService 创建短信验证码服务
func NewSMSService(kv cache.Driver, logger logging.Logger, provider SMSProvider) *SMSService {
	return &SMSService{
		kv:       kv,
		logger:   logger,
		provider: provider,
	}
}

// SendCode 发送验证码
func (s *SMSService) SendCode(ctx context.Context, phone string) error {
	// 检查发送间隔
	lastSendKey := fmt.Sprintf("%s%s_sent", smsCodePrefix, phone)
	if _, ok := s.kv.Get(lastSendKey); ok {
		return serializer.NewError(serializer.CodeParamErr, "验证码发送过于频繁，请稍后再试", nil)
	}

	// 生成6位随机验证码
	code := fmt.Sprintf("%06d", rand.Intn(1000000))

	// 发送短信
	if s.provider != nil {
		if err := s.provider.Send(ctx, phone, code); err != nil {
			s.logger.Warning("Failed to send SMS code to %s: %s", phone, err)
			return serializer.NewError(serializer.CodeInternalSetting, "发送验证码失败", err)
		}
	} else {
		// 如果没有配置短信服务，直接输出到日志（开发环境）
		s.logger.Info("SMS Code for %s: %s (SMS provider not configured)", phone, code)
	}

	// 保存验证码到缓存
	codeKey := fmt.Sprintf("%s%s", smsCodePrefix, phone)
	if err := s.kv.Set(codeKey, code, smsCodeTTL); err != nil {
		return serializer.NewError(serializer.CodeInternalSetting, "保存验证码失败", err)
	}

	// 记录发送时间
	if err := s.kv.Set(lastSendKey, time.Now().Unix(), smsCodeInterval); err != nil {
		s.logger.Warning("Failed to record SMS send time: %s", err)
	}

	return nil
}

// VerifyCode 验证验证码
func (s *SMSService) VerifyCode(ctx context.Context, phone, code string) error {
	codeKey := fmt.Sprintf("%s%s", smsCodePrefix, phone)
	storedCode, ok := s.kv.Get(codeKey)
	if !ok {
		return serializer.NewError(serializer.CodeParamErr, "验证码已过期或不存在", nil)
	}

	if storedCode.(string) != code {
		return serializer.NewError(serializer.CodeParamErr, "验证码错误", nil)
	}

	// 验证成功后删除验证码
	_ = s.kv.Delete(codeKey)

	return nil
}

// MockSMSProvider 模拟短信服务（用于开发测试）
type MockSMSProvider struct {
	logger logging.Logger
}

// NewMockSMSProvider 创建模拟短信服务
func NewMockSMSProvider(logger logging.Logger) SMSProvider {
	return &MockSMSProvider{logger: logger}
}

// Send 发送短信（模拟）
func (m *MockSMSProvider) Send(ctx context.Context, phone, code string) error {
	m.logger.Info("Mock SMS: Sending code %s to phone %s", code, phone)
	return nil
}

// SMSConfig 短信服务配置（从环境变量或配置文件读取）
type SMSConfig struct {
	Provider        string // "aliyun", "tencent", "mock"
	AliyunAccessKeyID     string
	AliyunAccessKeySecret string
	AliyunSignName        string
	AliyunTemplateCode    string
	TencentSecretID       string
	TencentSecretKey      string
	TencentSDKAppID       string
	TencentSignName       string
	TencentTemplateID     string
}

// GetSMSProvider 根据配置获取短信服务提供商
func GetSMSProvider(dep dependency.Dep, logger logging.Logger) SMSProvider {
	// 从环境变量读取配置
	provider := os.Getenv("SMS_PROVIDER")
	if provider == "" {
		provider = "mock" // 默认使用Mock
	}

	requestClient := dep.RequestClient(
		request.WithLogger(logger),
	)

	switch provider {
	case "aliyun":
		accessKeyID := os.Getenv("SMS_ALIYUN_ACCESS_KEY_ID")
		accessKeySecret := os.Getenv("SMS_ALIYUN_ACCESS_KEY_SECRET")
		signName := os.Getenv("SMS_ALIYUN_SIGN_NAME")
		templateCode := os.Getenv("SMS_ALIYUN_TEMPLATE_CODE")

		if accessKeyID == "" || accessKeySecret == "" || signName == "" || templateCode == "" {
			logger.Warning("Aliyun SMS config incomplete, falling back to Mock")
			return NewMockSMSProvider(logger)
		}

		return NewAliyunSMSProvider(AliyunSMSConfig{
			AccessKeyID:     accessKeyID,
			AccessKeySecret: accessKeySecret,
			SignName:        signName,
			TemplateCode:    templateCode,
		}, logger, requestClient)

	case "tencent":
		secretID := os.Getenv("SMS_TENCENT_SECRET_ID")
		secretKey := os.Getenv("SMS_TENCENT_SECRET_KEY")
		sdkAppID := os.Getenv("SMS_TENCENT_SDK_APP_ID")
		signName := os.Getenv("SMS_TENCENT_SIGN_NAME")
		templateID := os.Getenv("SMS_TENCENT_TEMPLATE_ID")

		if secretID == "" || secretKey == "" || sdkAppID == "" || signName == "" || templateID == "" {
			logger.Warning("Tencent SMS config incomplete, falling back to Mock")
			return NewMockSMSProvider(logger)
		}

		return NewTencentSMSProvider(TencentSMSConfig{
			SecretID:   secretID,
			SecretKey:  secretKey,
			SDKAppID:   sdkAppID,
			SignName:   signName,
			TemplateID: templateID,
		}, logger, requestClient)

	default:
		return NewMockSMSProvider(logger)
	}
}

