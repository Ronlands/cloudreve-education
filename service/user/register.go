package user

import (
	"errors"

	"github.com/cloudreve/Cloudreve/v4/application/dependency"
	"github.com/cloudreve/Cloudreve/v4/ent"
	"github.com/cloudreve/Cloudreve/v4/ent/user"
	"github.com/cloudreve/Cloudreve/v4/inventory"
	"github.com/cloudreve/Cloudreve/v4/pkg/hashid"
	"github.com/cloudreve/Cloudreve/v4/pkg/logging"
	"github.com/cloudreve/Cloudreve/v4/pkg/serializer"
	"github.com/cloudreve/Cloudreve/v4/pkg/setting"
	"github.com/cloudreve/Cloudreve/v4/pkg/sms"
	"github.com/cloudreve/Cloudreve/v4/pkg/util"
	"github.com/gin-gonic/gin"
)

// RegisterParameterCtx define key fore UserRegisterService
type RegisterParameterCtx struct{}

// UserRegisterService 管理用户注册的服务（仅支持手机号注册）
type UserRegisterService struct {
	// 手机号注册
	Phone        string `form:"phone" json:"phone" binding:"required"`                  // 手机号
	Code         string `form:"code" json:"code" binding:"required"`                  // 验证码
	Password     string `form:"password" json:"password" binding:"required,min=6,max=128"`          // 密码
	University   string `form:"university" json:"university" binding:"required"`      // 院校（必填）
	Major        string `form:"major" json:"major" binding:"required"`                // 专业（必填）
}

// Register 新用户注册（仅支持手机号注册）
func (service *UserRegisterService) Register(c *gin.Context) serializer.Response {
	dep := dependency.FromContext(c)
	settings := dep.SettingProvider()
	logger := logging.FromContext(c)

	// 只支持手机号注册
	if service.Phone == "" {
		return serializer.ErrWithDetails(c, serializer.CodeParamErr, "请提供手机号", nil)
	}

	return service.registerWithPhone(c, dep, settings, logger)
}

// registerWithPhone 手机号注册
func (service *UserRegisterService) registerWithPhone(c *gin.Context, dep dependency.Dep, settings setting.Provider, logger logging.Logger) serializer.Response {
	// 验证必填字段
	if service.Phone == "" {
		return serializer.ErrWithDetails(c, serializer.CodeParamErr, "手机号不能为空", nil)
	}
	if service.Code == "" {
		return serializer.ErrWithDetails(c, serializer.CodeParamErr, "验证码不能为空", nil)
	}
	if service.Password == "" {
		return serializer.ErrWithDetails(c, serializer.CodeParamErr, "密码不能为空", nil)
	}
	if len(service.Password) < 6 || len(service.Password) > 128 {
		return serializer.ErrWithDetails(c, serializer.CodeParamErr, "密码长度必须在6-128位之间", nil)
	}
	if service.University == "" {
		return serializer.ErrWithDetails(c, serializer.CodeParamErr, "院校不能为空", nil)
	}
	if service.Major == "" {
		return serializer.ErrWithDetails(c, serializer.CodeParamErr, "专业不能为空", nil)
	}

	// 规范化并验证手机号格式
	normalizedPhone := util.NormalizePhone(service.Phone)
	if !util.ValidatePhone(normalizedPhone) {
		return serializer.ErrWithDetails(c, serializer.CodeParamErr, "手机号格式不正确", nil)
	}

	// 验证短信验证码
	smsProvider := sms.GetSMSProvider(dep, logger)
	smsService := sms.NewSMSService(dep.KV(), logger, smsProvider)
	if err := smsService.VerifyCode(c, normalizedPhone, service.Code); err != nil {
		return serializer.Err(c, err)
	}

	args := &inventory.NewUserArgs{
		Phone:         normalizedPhone,
		PlainPassword: service.Password,
		Status:        user.StatusActive, // 手机号注册直接激活
		GroupID:       settings.DefaultGroup(c),
		University:    service.University,
		Major:        service.Major,
	}

	userClient := dep.UserClient()
	uc, tx, _, err := inventory.WithTx(c, userClient)
	if err != nil {
		return serializer.DBErr(c, "Failed to start transaction", err)
	}

	expectedUser, err := uc.Create(c, args)
	if expectedUser != nil {
		util.WithValue(c, inventory.UserCtx{}, expectedUser)
	}

	if err != nil {
		_ = inventory.Rollback(tx)
		if errors.Is(err, inventory.ErrUserPhoneExisted) {
			return serializer.ErrWithDetails(c, serializer.CodeEmailExisted, "手机号已被注册", err)
		}
		return serializer.DBErr(c, "注册失败", err)
	}

	if err := inventory.Commit(tx); err != nil {
		return serializer.DBErr(c, "Failed to commit user row", err)
	}

	return serializer.Response{Data: BuildUser(expectedUser, dep.HashIDEncoder())}
}

// ActivateUser 激活用户（保留用于其他场景，如管理员激活）
func ActivateUser(c *gin.Context) serializer.Response {
	uid := hashid.FromContext(c)
	dep := dependency.FromContext(c)
	userClient := dep.UserClient()

	// 查找待激活用户
	inactiveUser, err := userClient.GetByID(c, uid)
	if err != nil {
		return serializer.ErrWithDetails(c, serializer.CodeUserNotFound, "User not fount", err)
	}

	// 检查状态
	if inactiveUser.Status != user.StatusInactive {
		return serializer.ErrWithDetails(c, serializer.CodeUserCannotActivate, "This user cannot be activated", nil)
	}

	// 激活用户
	activeUser, err := userClient.SetStatus(c, inactiveUser, user.StatusActive)
	if err != nil {
		return serializer.DBErr(c, "Failed to update user", err)
	}

	util.WithValue(c, inventory.UserCtx{}, activeUser)
	return serializer.Response{Data: BuildUser(activeUser, dep.HashIDEncoder())}
}
