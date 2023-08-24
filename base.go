package shared

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"time"

	"gitlab.com/ics-project/back-thirdparty/helper"
	"gitlab.com/ics-project/back-thirdparty/models"

	"gitlab.com/ics-project/back-thirdparty/utils"

	"go.uber.org/zap"

	"github.com/astaxie/beego"
	"github.com/dgrijalva/jwt-go"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"gitlab.com/ics-project/back-thirdparty/structs"
)

// BaseController operations for Auth
type BaseController struct {
	beego.Controller
	sync.RWMutex
	Response structs.Response
	Logger   *zap.Logger
}

// EncryptedData operations for Auth
type EncryptedData struct {
	Body string `json:"body"`
}

// Claims ...
//JW token custom claims
type Claims struct {
	SysUserID uint32 `json:"sysUserID"`
	OsUserID  string `json:"os_user_id"`
	Username  string `json:"username"`
	UserID    string `json:"userID"`
	TenantID  string `json:"tenantID"`
	UserRole  string `json:"userRole"`
	QuotaPlan string `json:"quota_plan"`
	Email     string `json:"email"`
	Firstname string `json:"firstname"`
	Lastname  string `json:"lastname"`
	Test      bool   `json:"test"`
	jwt.StandardClaims
}

// GetLang ...
func (c *BaseController) GetLang() {
	lang := c.Ctx.Request.Header.Get("Multi-Language")
	helper.SetLang(lang)
}

// Prepare ...
func (co *BaseController) Prepare() {
	co.GetLang()
}

func (c *BaseController) Claim() Claims {
	return c.Ctx.Input.GetData(helper.AuthKey).(Claims)
}

// GenerateJWTString ...
func GenerateJWTString(user models.SysUser) string {
	expirationTime := time.Now().Add(24 * 60 * time.Minute)

	claims := &Claims{
		SysUserID: user.Base.ID,
		OsUserID:  user.OsUserID,
		Username:  user.Username,
		UserID:    user.OsUserID,
		TenantID:  user.OsTenantID,
		UserRole:  user.Role,
		Email:     user.Email,
		Firstname: user.Firstname,
		Lastname:  user.Lastname,
		Test:      user.Test,
		QuotaPlan: user.QuotaPlan,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: expirationTime.Unix(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(helper.JwtKey))
	fmt.Printf("token %v", tokenString)
	return tokenString
}

// ExtractJWTString ...
func ExtractJWTString(tokenString string) *Claims {
	retClaim := &Claims{}
	_, err := jwt.ParseWithClaims(tokenString, retClaim, func(t *jwt.Token) (interface{}, error) {
		return []byte(helper.JwtKey), nil
	})
	if err != nil {
		return nil
	}
	return retClaim
}

// AuthOSAdmin ...
func AuthOSAdmin() (provider *gophercloud.ProviderClient, err error) {
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: os.Getenv("identityEndpoint"),
		Username:         os.Getenv("username"),
		Password:         os.Getenv("password"),
		TenantID:         os.Getenv("tenantID"),
		DomainID:         "default",
	}
	//authenticate to openstack
	provider, err = openstack.AuthenticatedClient(opts)
	return
}

// AuthOSAdmin ...
func AuthOSAdminICS() (provider *gophercloud.ProviderClient, err error) {
	opts := gophercloud.AuthOptions{
		IdentityEndpoint: os.Getenv("ics.identityEndpoint"),
		Username:         os.Getenv("ics.username"),
		Password:         os.Getenv("ics.password"),
		TenantID:         os.Getenv("ics.tenantID"),
		DomainID:         "default",
	}
	//authenticate to openstack
	provider, err = openstack.AuthenticatedClient(opts)
	return
}

// SetBody successfully response
func (co *BaseController) BindJSON(params interface{}) error {
	// var encryptedJson EncryptedData

	json.NewDecoder(co.Ctx.Request.Body).Decode(&params) // encrypted := []byte(encryptedJson.Body)

	// decrypted := utils.Decrypt(encrypted, []byte("246E6E35247D35C7D36E9EB1"))
	// decrypted := utils.Decryp2t(encryptedJson.Body, []byte("246E6E35247D35C7D36E9EB1"))
	// decrypted := utils.Decryp3t(encrypted, []byte("246E6E35247D35C7D36E9EB1"))
	// json.Unmarshal([]byte(decrypted), &params)
	fields := reflect.ValueOf(params).Elem()
	for i := 0; i < fields.NumField(); i++ {
		tags := fields.Type().Field(i).Tag.Get("bind")
		if strings.Contains(tags, "required") && fields.Field(i).IsZero() {
			field := fields.Type().Field(i).Tag.Get("json")
			err := fmt.Sprintf("%v is required", field)
			co.SetError(100, err, err, "")
			return errors.New(err)
		}
		regex := fields.Type().Field(i).Tag.Get("regex")
		if len(regex) > 0 {
			match, _ := regexp.MatchString(regex, fields.Field(i).String())
			if !match {
				field := fields.Type().Field(i).Tag.Get("json")
				err := fmt.Sprintf("%v doesn't match regex", field)
				co.SetError(100, err, err, "")
				return errors.New(err)
			}
		}
	}
	return nil
}

// SetBody successfully response
func (co *BaseController) SetBody(body interface{}) {
	co.Response.StatusCode = http.StatusOK
	co.Response.Body.StatusCode = 0
	co.Response.Body.ErrorMsg = ""
	if body != nil {
		co.Response.Body.Body = body
	} else {
		co.Response.Body.Body = structs.SuccessResponse{Success: true}
	}

}

// SetErrorWithBody error response
func (co *BaseController) SetErrorWithBody(code int, body interface{}, message, log, user string) {

	if len(log) > 0 {
		co.RWMutex.Lock()
		defer co.RWMutex.Unlock()
		if len(user) > 0 {
			utils.SetLumlog(user)
		} else {
			utils.SetLumlog("admin")
		}
		co.Logger.Error(log)
		ekclient := utils.GetEKClient()
		body := utils.ElasticBody{System: "backend",
			Type:      "ERROR",
			Note:      user,
			IPAddress: co.GetClientIP(),
			UserAgent: co.GetUserAgent(),
			Action:    co.Ctx.Input.URL(),
			Timestamp: time.Now().String(),
			Message: utils.ElasticMessage{
				Status:    fmt.Sprintf("%v", code),
				Condition: message,
			},
		}
		ekclient.Put(body)
	}

	co.Response.StatusCode = code
	co.Response.Body.StatusCode = code
	co.Response.Body.ErrorMsg = message
	co.Response.Body.Body = body
}

// SetError error response
func (co *BaseController) SetError(code int, message, log, user string) {
	co.RWMutex.Lock()
	defer co.RWMutex.Unlock()
	if len(user) > 0 {
		utils.SetLumlog(user)
	} else {
		utils.SetLumlog("admin")
	}
	if len(log) > 0 {
		co.Logger.Error(log)
	} else {
		co.Logger.Error(message)
	}
	ekclient := utils.GetEKClient()
	body := utils.ElasticBody{System: "backend",
		Type:      "ERROR",
		Note:      user,
		IPAddress: co.GetClientIP(),
		UserAgent: co.GetUserAgent(),
		Action:    co.Ctx.Input.URL(),
		Timestamp: time.Now().Format("2006-01-02T15:04:05.999Z"),
		Message: utils.ElasticMessage{
			Status:    fmt.Sprintf("%v", code),
			Condition: message,
		},
	}
	ekclient.Put(body)

	co.Response.StatusCode = code
	co.Response.Body.StatusCode = code
	if os.Getenv("debug") == "1" {
		co.Response.Body.ErrorMsg = log
	} else {
		co.Response.Body.ErrorMsg = message
	}

	co.Response.Body.Body = nil
}

// SetLog error response
func (co *BaseController) SetLog(log, user string) {
	co.RWMutex.Lock()
	defer co.RWMutex.Unlock()
	if len(user) > 0 {
		utils.SetLumlog(user)
	} else {
		utils.SetLumlog("")
	}
	ekclient := utils.GetEKClient()
	body := utils.ElasticBody{System: "backend",
		Type:      "Log",
		Note:      user,
		IPAddress: co.GetClientIP(),
		UserAgent: co.GetUserAgent(),
		Action:    co.Ctx.Input.URL(),
		Timestamp: time.Now().String(),
		Message: utils.ElasticMessage{
			Status:    fmt.Sprintf("%v", 200),
			Condition: log,
		},
	}
	ekclient.Put(body)
	co.Logger.Error(log)
}

func (co *BaseController) GetClientIP() string {
	forwarded := co.Ctx.Request.Header.Get("X-FORWARDED-FOR")
	if forwarded != "" {
		return forwarded
	}
	return co.Ctx.Input.IP()
}
func (co *BaseController) GetUserAgent() string {
	return co.Ctx.Input.UserAgent()
}

// GetBody in response
func (co *BaseController) GetBody() (int, interface{}) {
	return co.Response.StatusCode, co.Response.Body
}

// Respond in response
func (co *BaseController) Respond() {
	status, body := co.GetBody()
	if status == http.StatusOK {
		co.Data["json"] = body
		co.ServeJSON()
	} else {
		co.Data["json"] = body
		co.ServeJSON()
		co.Logger.Sync()
		co.Ctx.ResponseWriter.WriteHeader(status)
	}
}

// Respond in response
func (co *BaseController) RespondXML() {
	status, body := co.GetBody()
	if status == http.StatusOK {
		co.Data["xml"] = body
		co.ServeXML()
	} else {
		co.Data["json"] = body
		co.ServeXML()
		co.Logger.Sync()
		co.Ctx.ResponseWriter.WriteHeader(status)
	}
}

// Respond in response
func (co *BaseController) RespondPanic(r interface{}) {
	if os.Getenv("debug") == "1" {
		co.Data["json"] = structs.ResponseBody{
			StatusCode: helper.StatusBadRequest,
			ErrorMsg:   fmt.Sprintf("%v", r),
		}

	} else {
		co.Data["json"] = structs.ResponseBody{
			StatusCode: helper.StatusBadRequest,
			ErrorMsg:   helper.StatusText(helper.StatusBadRequest),
		}
	}
	co.ServeJSON()
}
