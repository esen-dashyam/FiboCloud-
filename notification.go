package shared

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"gitlab.com/ics-project/back-thirdparty/helper"
	"gitlab.com/ics-project/back-thirdparty/service"
	"gitlab.com/ics-project/back-thirdparty/structs"
	"gitlab.com/ics-project/back-thirdparty/utils"
)

// NotificationController struct
type NotificationController struct {
	BaseController
}

// URLMapping ...
func (i *NotificationController) URLMapping() {
	i.Mapping("GetNotification", i.GetNotification)
	i.Mapping("ChangeNotificationStatus", i.ChangeNotificationStatus)
	i.Mapping("ListNotification", i.ListNotification)
	i.Mapping("ReadAll", i.ReadAll)
}

/****************************
	Connect to Notification Websocket
*****************************/
type (
	// NotifyMessage ...
	NotifyMessage struct {
		Action string     `json:"action"`
		UserID string     `json:"userId"`
		Data   NotifyData `json:"data"`
	}
	// NotifyData ...
	NotifyData struct {
		ID        int       `json:"ID"`
		UserID    string    `json:"UserID"`
		TopicName string    `json:"TopicName"`
		NotifName string    `json:"NotifName"`
		IsError   int       `json:"IsError"`
		Date      time.Time `json:"Date"`
		TenantID  string    `json:"TenantID"`
		IsNew     int       `json:"IsNew"`
		URL       string    `json:"URL"`
	}
	// SocketUser ...
	SocketUser struct {
		Broadcast chan NotifyMessage
		Interrupt chan os.Signal
	}
)

// GetNotification ...
// @Title GetNotification
// @Description get GetNotification
// @Param    notificationID    int    false    "notificationID"
// @Failure 403
// @router /getNotification [post]
func (i *NotificationController) GetNotification() {
	claims := i.Claim()
	logger := utils.GetLogger()
	var response structs.ResponseBody

	defer func() {
		i.Data["json"] = response
		i.ServeJSON()
	}()

	bodyResult := RetrieveDataFromBody(i.Ctx.Request.Body)
	bodyString := []string{
		"notificationID",
	}

	if sCod, eMsg := CheckBodyResult(bodyResult, bodyString); sCod == 100 {
		response.StatusCode = sCod
		response.ErrorMsg = eMsg
		return
	}

	notification, err := service.GetNotification(bodyResult["notificationID"].(int))
	id := fmt.Sprint(notification.Base.ID)
	service.CreateLogAction(id, "Notification", "", "Get", claims.UserID, err)

	if err != nil {
		i.RWMutex.Lock()
		defer i.RWMutex.Unlock()
		utils.SetLumlog(claims.UserID)
		logger.Error(err.Error())
		response.StatusCode = 100
		response.ErrorMsg = err.Error()
		return
	}
	response.StatusCode = 0
	response.ErrorMsg = ""
	response.Body = notification
}

// ListNotification ...
// @Title ListNotification
// @Description get ListNotification
// @Failure 403
// @router /listNotification [get]
func (i *NotificationController) ListNotification() {
	claims := i.Claim()
	logger := utils.GetLogger()
	var response structs.ResponseBody

	defer func() {
		i.Data["json"] = response
		i.ServeJSON()
	}()

	notifications, err := service.ListNotification(claims.UserID)

	if err != nil {
		i.RWMutex.Lock()
		defer i.RWMutex.Unlock()
		utils.SetLumlog(claims.UserID)
		logger.Error(err.Error())
		response.StatusCode = 100
		response.ErrorMsg = err.Error()
		return
	}
	response.StatusCode = 0
	response.ErrorMsg = ""
	response.Body = notifications
}

// ReadAll ...
// @Title ReadAll
// @Description get ReadAll
// @Failure 403
// @router /read-all [get]
func (i *NotificationController) ReadAll() {
	claims := i.Claim()
	logger := utils.GetLogger()
	var response structs.ResponseBody

	defer func() {
		i.Data["json"] = response
		i.ServeJSON()
	}()

	err := service.ReadAll(claims.UserID)

	if err != nil {
		i.RWMutex.Lock()
		defer i.RWMutex.Unlock()
		utils.SetLumlog(claims.UserID)
		logger.Error(err.Error())
		response.StatusCode = 100
		response.ErrorMsg = err.Error()
		return
	}
	response.StatusCode = 0
	response.ErrorMsg = ""
	response.Body = "ok"
}

// ChangeNotificationStatus ...
// @Title ChangeNotificationStatus
// @Description get ChangeNotificationStatus
// @Param    notificationID    float64    false    "notificationID"
// @Param    status    int    false    "status"
// @Failure 403
// @router /changeNotificationStatus [post]
func (i *NotificationController) ChangeNotificationStatus() {
	claims := i.Claim()
	logger := utils.GetLogger()
	var response structs.ResponseBody

	defer func() {
		i.Data["json"] = response
		i.ServeJSON()
	}()

	bodyResult := RetrieveDataFromBody(i.Ctx.Request.Body)
	bodyString := []string{
		"status",
	}

	if sCod, eMsg := CheckBodyResult(bodyResult, bodyString); sCod == 100 {
		response.StatusCode = sCod
		response.ErrorMsg = eMsg
		return
	}

	notif, err := service.ChangeNotificationStatus(int(bodyResult["notificationID"].(float64)), int(bodyResult["status"].(float64)))

	if err != nil {
		i.RWMutex.Lock()
		defer i.RWMutex.Unlock()
		utils.SetLumlog(claims.UserID)
		logger.Error(err.Error())
		response.StatusCode = 100
		response.ErrorMsg = err.Error()
		return
	}
	response.StatusCode = 0
	response.ErrorMsg = ""
	response.Body = notif
}

// SendPushNotificationToUser ...
func SendPushNotificationToUser(userid, topicname, notifname, tenantid, url string, iserror error) error {
	now := time.Now()

	if iserror == nil {
		ok := PushNotification(userid, topicname, notifname)
		_, err := service.CreateNotification(userid, topicname, notifname, tenantid, url, "", 1, now)
		fmt.Println(ok)
		return err
	}

	return nil
}

// SendConfirmationMail ...
func SendConfirmationMail(email, userid, code string) (bool, error) {
	jsonData := map[string]string{
		"email":       email,
		"userId":      userid,
		"confirmCode": code,
		"url":         "true",
	}
	jsonValue, _ := json.Marshal(jsonData)
	response, err := http.Post("https://01qfxin5zd.execute-api.ap-east-1.amazonaws.com/prod/user-confirm", "application/json", bytes.NewBuffer(jsonValue))
	data, _ := ioutil.ReadAll(response.Body)
	ok := string(data)
	fmt.Println("Ok", ok)
	if ok != "Ok" {
		return false, err
	}
	return true, err
}

// CallbackFunction ...
func CallbackFunction(url string, jsonData interface{}) (bool, error) {

	jsonValue, _ := json.Marshal(jsonData)
	response, err := http.Post(url, "application/json", bytes.NewBuffer(jsonValue))
	data, _ := ioutil.ReadAll(response.Body)
	ok := string(data)
	fmt.Println("Ok", ok)
	if ok != "Ok" {
		return false, err
	}
	return true, err
}

// InstanceStartNotify ...
func InstanceStartNotify(email, instanceName, createdAt, ip string) (bool, error) {
	fmt.Println("email", email)
	fmt.Println("instanceName", instanceName)
	fmt.Println("createdAt", createdAt)
	fmt.Println("ip", ip)
	jsonData := map[string]string{
		"email":         email,
		"instance_name": instanceName,
		"created_at":    createdAt,
		"ip":            ip,
	}
	jsonValue, _ := json.Marshal(jsonData)
	response, err := http.Post("https://01qfxin5zd.execute-api.ap-east-1.amazonaws.com/prod/instance-created", "application/json", bytes.NewBuffer(jsonValue))
	data, _ := ioutil.ReadAll(response.Body)
	ok := string(data)
	fmt.Println("Ok", ok)
	if ok != "Ok" {
		return true, err
	}
	return false, err
}

// InstanceDeleteNotify ...
func InstanceDeleteNotify(email, instanceName, deletedAt, ip string) (bool, error) {
	fmt.Println("email", email)
	fmt.Println("instanceName", instanceName)
	fmt.Println("deletedAt", deletedAt)
	fmt.Println("ip", ip)
	jsonData := map[string]string{
		"email":         email,
		"instance_name": instanceName,
		"deleted_at":    deletedAt,
		"ip":            ip,
	}
	jsonValue, _ := json.Marshal(jsonData)
	response, err := http.Post("https://01qfxin5zd.execute-api.ap-east-1.amazonaws.com/prod/instance-deleted", "application/json", bytes.NewBuffer(jsonValue))
	data, _ := ioutil.ReadAll(response.Body)
	ok := string(data)
	fmt.Println("ok", ok)
	if ok != "Ok" {
		return true, err
	}
	return false, err
}

// ConfirmAmazonDatabase ...
func ConfirmAmazonDatabase(email string) (bool, error) {
	jsonData := map[string]string{
		"email": email,
	}
	jsonValue, _ := json.Marshal(jsonData)
	response, err := http.Post("https://78x2hku860.execute-api.ap-east-1.amazonaws.com/pro/confirm", "application/json", bytes.NewBuffer(jsonValue))
	data, _ := ioutil.ReadAll(response.Body)
	ok := string(data)
	if ok != "SUCCESS" {
		return true, err
	}
	return false, err
}

// RegisterUserToAmazonDatabase ...
func RegisterUserToAmazonDatabase(email, password, firstname, lastname, stackid, defaultTenantID, stackPassword, role, username string, isActivated bool) (bool, error) {
	strActivte := strconv.FormatBool(isActivated)
	jsonData := map[string]string{
		"email":        email,
		"password":     password,
		"firstname":    firstname,
		"lastname":     lastname,
		"os_user_id":   stackid,
		"os_tenant_id": defaultTenantID,
		"os_pwd":       stackPassword,
		"role":         role,
		"is_active":    strActivte,
		"username":     username,
	}
	jsonValue, _ := json.Marshal(jsonData)
	response, err := http.Post("https://78x2hku860.execute-api.ap-east-1.amazonaws.com/pro", "application/json", bytes.NewBuffer(jsonValue))
	data, _ := ioutil.ReadAll(response.Body)
	ok := string(data)
	if ok != "SUCCESS" {
		return true, err
	}
	return false, err
}

// EmailForgotPassword ...
func EmailForgotPassword(firstname, lastname, email, token string) bool {
	url := "https://01qfxin5zd.execute-api.ap-east-1.amazonaws.com/prod/forgot-password"

	payloadObject := make(map[string]interface{})
	payloadObject["firstname"] = firstname
	payloadObject["lastname"] = lastname
	payloadObject["email"] = email
	payloadObject["token"] = token
	payloadString, _ := json.Marshal(payloadObject)
	payload := strings.NewReader(string(payloadString))
	req, _ := http.NewRequest("POST", url, payload)
	req.Header.Add("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	fmt.Println(err)
	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)
	fmt.Println(body)
	return true
}

// InviteUserToProject ...
func InviteUserToProject(invited_by, invited_project, email, token string) bool {
	url := "https://01qfxin5zd.execute-api.ap-east-1.amazonaws.com/prod/invitation"

	payloadObject := make(map[string]interface{})
	payloadObject["invited_by"] = invited_by
	payloadObject["invited_project"] = invited_project
	payloadObject["email"] = email
	payloadObject["token"] = token
	payloadString, _ := json.Marshal(payloadObject)
	payload := strings.NewReader(string(payloadString))
	req, _ := http.NewRequest("POST", url, payload)
	req.Header.Add("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	fmt.Println(err)
	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)
	fmt.Println(body)
	return true
}

func ProjectNotifcations(path, method string, payloadObject interface{}) bool {
	url := fmt.Sprintf("https://01qfxin5zd.execute-api.ap-east-1.amazonaws.com/prod/%v", path)

	payloadString, _ := json.Marshal(payloadObject)
	payload := strings.NewReader(string(payloadString))
	req, _ := http.NewRequest(method, url, payload)
	req.Header.Add("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	fmt.Println(err)
	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)
	fmt.Println(body)
	return true
}

// PushNotification ...
func PushNotification(userid, title, content string) bool {
	jsons := make(map[string]interface{})
	url := "https://onesignal.com/api/v1/notifications"

	payload := strings.NewReader("{\n    \"app_id\": \"" + helper.Appid + "\",\n    \"include_external_user_ids\": [\n        \"" + userid + "\"\n    ],\n    \"headings\": {\n        \"en\": \"" + title + "\"\n    },\n    \"contents\": {\n        \"en\": \"" + content + "\"\n    }\n}")

	req, _ := http.NewRequest("POST", url, payload)

	req.Header.Add("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)

	fmt.Println(err)

	defer res.Body.Close()
	body, _ := ioutil.ReadAll(res.Body)

	json.Unmarshal(body, &jsons)

	return true
}
