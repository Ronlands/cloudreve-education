package user

import (
	"context"
	"strings"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/user"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/pkg/sms"
	"github.com/cloudreve/Cloudreve/v4/pkg/util"
	"github.com/gin-gonic/gin"
)

// SMSLoginParameterCtx define key for SMSLoginService
type SMSLoginParameterCtx struct{}

// SMSLoginService 手机号+验证码登录服务（无需密码）
type SMSLoginService struct {
	Phone string `form:"phone" json:"phone" binding:"required"`
	Code  string `form:"code" json:"code" binding:"required"`
}

// Login 手机号+验证码登录
func (service *SMSLoginService) Login(c *gin.Context) (*ent.User, error) {
	dep := dependency.FromContext(c)
	logger := logging.FromContext(c)
	userClient := dep.UserClient()

	// 规范化并验证手机号格式
	normalizedPhone := util.NormalizePhone(service.Phone)
	if !util.ValidatePhone(normalizedPhone) {
		return nil, serializer.NewError(serializer.CodeParamErr, "手机号格式不正确", nil)
	}

	// 验证短信验证码
	smsProvider := sms.GetSMSProvider(dep, logger)
	smsService := sms.NewSMSService(dep.KV(), logger, smsProvider)
	if err := smsService.VerifyCode(c, normalizedPhone, service.Code); err != nil {
		return nil, err
	}

	// 查找用户
	ctx := context.WithValue(c, inventory.LoadUserGroup{}, true)
	expectedUser, err := userClient.GetByPhone(ctx, normalizedPhone)
	if err != nil {
		return nil, serializer.NewError(serializer.CodeUserNotFound, "用户不存在", err)
	}

	// 检查用户状态
	if expectedUser.Status == user.StatusManualBanned || expectedUser.Status == user.StatusSysBanned {
		return nil, serializer.NewError(serializer.CodeUserBaned, "该账号已被封禁", nil)
	}
	if expectedUser.Status == user.StatusInactive {
		return nil, serializer.NewError(serializer.CodeUserNotActivated, "该账号未激活", nil)
	}

	return expectedUser, nil
}

