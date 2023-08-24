package thirtdparty

import (
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"gitlab.com/ics-project/back-thirdparty/controllers/shared"
	"gitlab.com/ics-project/back-thirdparty/helper"
	"gitlab.com/ics-project/back-thirdparty/integration/compute"
	"gitlab.com/ics-project/back-thirdparty/integration/networking"
	"gitlab.com/ics-project/back-thirdparty/models"
	"gitlab.com/ics-project/back-thirdparty/service"
	"gitlab.com/ics-project/back-thirdparty/utils"
)

// LambdaController struct
type LambdaController struct {
	shared.BaseController
}

// URLMapping URL mapping
func (m *LambdaController) URLMapping() {
	m.Mapping("Create", m.Create)
	m.Mapping("CreatePHP", m.CreatePHP)
	m.Mapping("List", m.FlavorLambda)
}

// Create Lambda stack ...
// @Title Create
// @Description hint Create
// @Param	flavor	string true "flavor"
// @Param	lambda_full_name	string true "lambda_full_name"
// @Param	key	string true "key"
// @Failure 403
// @router /create/go [post]
func (m *LambdaController) Create() {
	claims := m.Claim()

	defer func() {
		if r := recover(); r != nil {
			fmt.Println(r)
			m.RespondPanic(r)
		} else {
			m.Respond()
		}
	}()

	// RequestedParams ...
	type RequestedParams struct {
		ID               string `json:"requestId" bind:"required"`
		Key              string `json:"key" bind:"required"`
		AllowedIP        string `json:"allowedIp" bind:"required"`
		CallbackUrl      string `json:"callbackUrl" bind:"required"`
		Flavor           string `json:"flavor" default:"4f38ef92-120f-4d10-8485-79d672138935" bind:"required"`
		LambdaFullName   string `json:"name" bind:"required"`
		SysAdminLogin    string `json:"sysAdminLogin"  bind:"required"`
		SysAdminPassword string `json:"sysAdminPassword"  bind:"required"`
		SysAdminEmail    string `json:"sysAdminEmail"  bind:"required"`
	}

	params := RequestedParams{}
	if m.BindJSON(&params) != nil {
		return
	}
	lambdaFullName := params.LambdaFullName
	key := params.Key
	flavorID := params.Flavor
	diskSize := 15
	callbackUrl := params.CallbackUrl
	allowedIp := fmt.Sprintf("%s/32", params.AllowedIP)

	/* find default security group */
	securityGroupList, err := compute.GetCompute(claims.Username).ListSecGroups()
	if err != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), err.Error(), fmt.Sprintf("lambda-%v", claims.UserID))
	}

	var sgIDs []interface{}
	for _, secGroup := range securityGroupList {
		if secGroup.Name == "default" {
			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "mysql", 3306, 3306, "tcp", allowedIp)
			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "app", 8080, 8080, "tcp", allowedIp)
			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "http", 80, 80, "tcp", "0.0.0.0/0")
			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "https", 443, 443, "tcp", "0.0.0.0/0")
			sgIDs = append(sgIDs, secGroup.Name)
		}
	}

	networkID := ""
	imageID := models.GetConfig("3thparty_image")

	if networkID == "" {
		adminProvider, _ := shared.AuthOSAdmin()
		networkID, _ = networking.GetNetworking("admin").GetAvailableNetworks(adminProvider)
	}

	secGroup := make([]string, len(sgIDs))
	for i, v := range sgIDs {

		secGroup[i] = fmt.Sprint(v)
	}

	userData, err := ioutil.ReadFile("files/lambda/base.yml")
	if err != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), err.Error(), "lambda")
		return
	}
	lines := strings.Split(string(userData), "\n")
	generatedUsername := utils.RandSeq(5)
	generatedPassword := utils.RandSeq(10)
	generatedDB := utils.RandSeq(5)
	for i, line := range lines {
		if strings.Contains(line, "KEY") {
			lines[i] = strings.Replace(lines[i], "KEY", key, -1)
		}
		if strings.Contains(line, "[DB_NAME]") {
			lines[i] = strings.Replace(lines[i], "[DB_NAME]", fmt.Sprintf(" %s", generatedDB), -1)
		}
		if strings.Contains(line, "[DB_USER]") {
			lines[i] = strings.Replace(lines[i], "[DB_USER]", fmt.Sprintf(" %s", generatedUsername), -1)
		}
		if strings.Contains(line, "[DB_PASS]") {
			lines[i] = strings.Replace(lines[i], "[DB_PASS]", fmt.Sprintf(" %s", generatedPassword), -1)
		}
		if strings.Contains(line, "[SYSADMIN_LOGIN]") {
			lines[i] = strings.Replace(lines[i], "[SYSADMIN_LOGIN]", fmt.Sprintf(" %s", params.SysAdminLogin), -1)
		}
		if strings.Contains(line, "[SYSADMIN_PASSWORD]") {
			lines[i] = strings.Replace(lines[i], "[SYSADMIN_PASSWORD]", fmt.Sprintf(" %s", params.SysAdminPassword), -1)
		}
		if strings.Contains(line, "[SYSADMIN_EMAIL]") {
			lines[i] = strings.Replace(lines[i], "[SYSADMIN_EMAIL]", fmt.Sprintf(" %s", params.SysAdminEmail), -1)
		}
	}

	output := strings.Join(lines, "\n")
	fmt.Println(output)

	scripted := []byte(output)

	server, errServer := compute.GetCompute(claims.Username).CreateServerForVolume(imageID, lambdaFullName, flavorID, "", int(diskSize), secGroup, scripted, "nova", true, networkID)
	if errServer != nil {
		fmt.Print(errServer)
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), err.Error(), fmt.Sprintf("lambda-%v", claims.UserID))
		return
	}

	service.CreateLogAction(server.ID, "Lambda", lambdaFullName, "Create", claims.UserID, err)
	_, errGetFlavor := compute.GetCompute(claims.Username).GetFlavor(flavorID, "CLOUD.MN")
	if errGetFlavor != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), errServer.Error(), fmt.Sprintf("lambda-%v", claims.UserID))
	}

	go func(serverid, username, generatedDB, generatedUsername, generatedPassword string) {
		provider := service.GetProvider(username)
		for {
			server, _ := servers.Get(service.GetClientCompute(provider), serverid).Extract()
			if server.Status == "ACTIVE" {
				var ip string
				if server.Addresses["public1"] != nil {
					for _, l := range server.Addresses["public1"].([]interface{}) {
						tmp := l.(map[string]interface{})
						ip = tmp["addr"].(string)
					}
				}
				if server.Addresses["public2"] != nil {
					for _, l := range server.Addresses["public2"].([]interface{}) {
						tmp := l.(map[string]interface{})
						ip = tmp["addr"].(string)

					}
				}

				type ReturnParams struct {
					ID             string `json:"requestId"`
					DBName         string `json:"databaseName"`
					DBUsername     string `json:"databaseUser"`
					DBPassword     string `json:"databasePwd"`
					Ip             string `json:"databaseUrl"`
					ServerName     string `json:"serverName"`
					ServerPassword string `json:"serverPwd"`
				}

				returnParams := ReturnParams{ID: params.ID, DBName: generatedDB, DBUsername: generatedUsername, DBPassword: generatedPassword, Ip: ip, ServerName: "fibo", ServerPassword: "fibo123"}
				/* sending callback request */
				fmt.Println("sending callback request")
				shared.CallbackFunction(callbackUrl, returnParams)
				return
			}
			if server.Status == "ERROR" {
				return
			}
			time.Sleep(3 * time.Second)
		}
	}(server.ID, claims.Username, generatedDB, generatedUsername, generatedPassword)

	m.SetBody(server)
}

// Create lambda-php stack ...
// @Title Create
// @Description hint Create
// @Param	id	string true "id"
// @Param	callback_url	string true "callback_url"
// @Param	allow_ip	string false "allow_ip"
// @Param	flavor_id	string true "flavor_id"
// @Param db_database string false "db_database"
// @Param db_username string false "db_username"
// @Param db_password string false "db_password"
// @Param sysadmin_login string true "sysadmin_login"
// @Param sysadmin_password string true "sysadmin_password"
// @Param sysadmin_email string true "sysadmin_email"
// @Failure 403
// @router /create/php [post]
func (m *LambdaController) CreatePHP() {
	claims := m.Claim()

	defer func() {
		if r := recover(); r != nil {
			m.RespondPanic(r)
		} else {
			m.Respond()
		}
	}()

	// RequestedParams ...
	type RequestedParams struct {
		ID               string `json:"requestId" bind:"required"`
		Key              string `json:"key" bind:"required"`
		AllowedIP        string `json:"allowedIp" bind:"required"`
		CallbackUrl      string `json:"callbackUrl" bind:"required"`
		FlavorID         string `json:"flavorId"  bind:"required"`
		SysAdminLogin    string `json:"sysAdminLogin"  bind:"required"`
		SysAdminPassword string `json:"sysAdminPassword"  bind:"required"`
		SysAdminEmail    string `json:"sysAdminEmail"  bind:"required"`
	}

	params := RequestedParams{}
	if m.BindJSON(&params) != nil {
		return
	}
	id := params.ID
	flavorID := params.FlavorID // ics4  - 8gb -ram 4 vcpu
	allowedIp := fmt.Sprintf("%s/32", params.AllowedIP)
	callbackUrl := params.CallbackUrl

	sysAdminLogin := params.SysAdminLogin
	sysAdminPassword := params.SysAdminPassword
	sysAdminEmail := params.SysAdminEmail

	diskSize := 10

	/* find default security group */
	securityGroupList, err := compute.GetCompute(claims.Username).ListSecGroups()
	if err != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), err.Error(), fmt.Sprintf("bbx-%v", claims.UserID))
	}

	var sgIDs []interface{}
	for _, secGroup := range securityGroupList {
		if secGroup.Name == "default" {
			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "web server", 80, 80, "tcp", "0.0.0.0/0")
			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "mysql", 3306, 3306, "tcp", allowedIp)
			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "app", 8080, 8080, "tcp", allowedIp)
			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "https", 443, 443, "tcp", "0.0.0.0/0")
			sgIDs = append(sgIDs, secGroup.Name)
		}
	}

	networkID := ""
	imageID := models.GetConfig("3thparty_image")

	if networkID == "" {
		adminProvider, _ := shared.AuthOSAdmin()
		networkID, _ = networking.GetNetworking("admin").GetAvailableNetworks(adminProvider)
	}

	secGroup := make([]string, len(sgIDs))
	for i, v := range sgIDs {
		secGroup[i] = fmt.Sprint(v)
	}

	userData, err := ioutil.ReadFile("files/lambda-php/base.yml")
	if err != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), err.Error(), "bbx")
		return
	}

	lines := strings.Split(string(userData), "\n")

	generatedUsername := utils.RandSeq(5)
	generatedPassword := utils.RandSeq(10)

	for i, line := range lines {
		if strings.Contains(line, "{DB_USERNAME}") {
			lines[i] = strings.Replace(lines[i], "{DB_USERNAME}", generatedUsername, -1)
		}
		if strings.Contains(line, "{DB_PASSWORD}") {
			lines[i] = strings.Replace(lines[i], "{DB_PASSWORD}", generatedPassword, -1)
		}
		if strings.Contains(line, "{SYSADMIN_LOGIN}") {
			lines[i] = strings.Replace(lines[i], "{SYSADMIN_LOGIN}", sysAdminLogin, -1)
		}
		if strings.Contains(line, "{SYSADMIN_PASSWORD}") {
			lines[i] = strings.Replace(lines[i], "{SYSADMIN_PASSWORD}", sysAdminPassword, -1)
		}
		if strings.Contains(line, "{SYSADMIN_EMAIL}") {
			lines[i] = strings.Replace(lines[i], "{SYSADMIN_EMAIL}", sysAdminEmail, -1)
		}
	}

	output := strings.Join(lines, "\n")
	fmt.Println(output)

	scripted := []byte(output)

	server, errServer := compute.GetCompute(claims.Username).CreateServerForVolume(imageID, id, flavorID, "", int(diskSize), secGroup, scripted, "nova", true, networkID)
	if errServer != nil {
		fmt.Print(errServer)
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), err.Error(), fmt.Sprintf("bbx-%v", claims.UserID))
		return
	}

	service.CreateLogAction(server.ID, "lambda-php", id, "Create", claims.UserID, err)
	_, errGetFlavor := compute.GetCompute(claims.Username).GetFlavor(flavorID, "CLOUD.MN")
	if errGetFlavor != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), errServer.Error(), fmt.Sprintf("lambda-php-%v", claims.UserID))
	}

	go func(serverid string, username string) {
		provider := service.GetProvider(username)
		for {
			server, _ := servers.Get(service.GetClientCompute(provider), serverid).Extract()
			if server.Status == "ACTIVE" {
				var ip string
				if server.Addresses["public1"] != nil {
					for _, l := range server.Addresses["public1"].([]interface{}) {
						tmp := l.(map[string]interface{})
						ip = tmp["addr"].(string)
					}
				}
				if server.Addresses["public2"] != nil {
					for _, l := range server.Addresses["public2"].([]interface{}) {
						tmp := l.(map[string]interface{})
						ip = tmp["addr"].(string)

					}
				}

				type ReturnParams struct {
					ID             string `json:"requestId"`
					DBName         string `json:"databaseName"`
					DBUsername     string `json:"databaseUser"`
					DBPassword     string `json:"databasePwd"`
					Ip             string `json:"databaseUrl"`
					ServerName     string `json:"serverName"`
					ServerPassword string `json:"serverPwd"`
				}

				returnParams := ReturnParams{ID: id, DBName: "primebbx", DBUsername: generatedUsername, DBPassword: generatedPassword, Ip: ip, ServerName: "fibo", ServerPassword: "fibo123"}
				/* sending callback request */
				fmt.Println("sending callback request")
				shared.CallbackFunction(callbackUrl, returnParams)
				return
			}
			if server.Status == "ERROR" {
				return
			}
			time.Sleep(3 * time.Second)
		}
	}(server.ID, claims.Username)

	m.SetBody(server)
}

// FlavorLambda ...
// @Title FlavorLambda
// @Description hint FlavorLambda
// @Failure 403
// @router /list [get]
func (m *LambdaController) FlavorLambda() {
	claims := m.Claim()

	var mainRes []flavors.Flavor
	defer func() {
		if r := recover(); r != nil {
			m.RespondPanic(r)
		} else {
			m.Respond()
		}
	}()

	pAccess := flavors.AllAccess
	listFlavors, err := compute.GetCompute(claims.Username).ListFlavors(pAccess)
	if err != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), err.Error(), claims.UserID)
		return
	}
	for _, c := range listFlavors {
		if strings.Contains(c.Name, "Lambda") {
			mainRes = append(mainRes, c)
		}

	}

	m.SetBody(mainRes)
}
