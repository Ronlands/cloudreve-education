package user

import (
	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/pkg/sms"
	"github.com/cloudreve/Cloudreve/v4/pkg/util"
	"github.com/gin-gonic/gin"
)

// SendSMSCodeParameterCtx define key for SendSMSCodeService
type SendSMSCodeParameterCtx struct{}

// SendSMSCodeService 发送短信验证码服务
type SendSMSCodeService struct {
	Phone string `form:"phone" json:"phone" binding:"required"`
}

// SendCode 发送验证码
func (service *SendSMSCodeService) SendCode(c *gin.Context) serializer.Response {
	dep := dependency.FromContext(c)
	logger := logging.FromContext(c)

	// 规范化并验证手机号格式
	normalizedPhone := util.NormalizePhone(service.Phone)
	if !util.ValidatePhone(normalizedPhone) {
		return serializer.ErrWithDetails(c, serializer.CodeParamErr, "手机号格式不正确", nil)
	}

	// 获取短信服务提供商
	smsProvider := sms.GetSMSProvider(dep, logger)

	// 创建短信服务
	smsService := sms.NewSMSService(dep.KV(), logger, smsProvider)

	// 发送验证码
	if err := smsService.SendCode(c, normalizedPhone); err != nil {
		return serializer.Err(c, err)
	}

	return serializer.Response{
		Data: map[string]string{
			"message": "验证码已发送",
		},
	}
}

