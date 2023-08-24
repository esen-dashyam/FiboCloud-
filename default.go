package thirtdparty

import (
	"gitlab.com/ics-project/back-thirdparty/controllers/shared"
	"gitlab.com/ics-project/back-thirdparty/service"
	"gitlab.com/ics-project/back-thirdparty/utils"
)

// MainController struct
type MainController struct {
	shared.BaseController
}

// Get ...
func (c *MainController) Get() {
	c.TplName = "index.html"
}

// GenerateCredential ...
// @Title GenerateCredential
// @Description get GenerateCredential
// @Success 200 {string} successful generated credential
// @Failure 403
// @router /generate [post]
func (c *MainController) GenerateCredential() {
	claims := c.Claim()

	defer func() {
		if r := recover(); r != nil {
			c.RespondPanic(r)
		} else {
			c.Respond()
		}
	}()

	var secretKey = utils.RandSeq(50)
	cred, errget := service.GetCredential(claims.UserID)
	if errget != nil {
		c.SetError(1, errget.Error(), errget.Error(), claims.UserID)
		return
	}
	err := service.GenerateCredential(claims.UserID, cred.AccessKey, secretKey)
	if err != nil {
		c.SetError(1, err.Error(), err.Error(), claims.UserID)
		return
	}
	service.CreateLogAction(cred.AccessKey, "Generate Credential", "", "Generate", claims.UserID, err)

	if err != nil {
		c.SetError(1, err.Error(), err.Error(), claims.UserID)
		return
	}
	var key map[string]string = map[string]string{"accessKey": cred.AccessKey, "secretKey": secretKey}
	c.SetBody(key)
}
