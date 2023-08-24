package thirtdparty

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/astaxie/beego/orm"
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

// SCSController struct
type SCSController struct {
	shared.BaseController
}

// URLMapping URL mapping
func (m *SCSController) URLMapping() {
	m.Mapping("Create", m.Create)
	m.Mapping("Action", m.Action)
	m.Mapping("List", m.Flavors)
}

// ResultRealm ...
type ResultRealm struct {
	Realm       string `json:"realm" `
	ClientId    string `json:"clientId" bind:"required"`
	PublicKey   string `json:"publicKey" bind:"required"`
	Certificate string `json:"certificate" bind:"required"`
}

// ResponseRealm ...

type ResponseRealm struct {
	Code   string      `json:"code" bind:"required"`
	Key    string      `json:"key" bind:"required"`
	Result ResultRealm `json:"result" bind:"required"`
}

// Create ifinance stack ...
// @Title Create
// @Description hint Create
// @Param	id	string true "id"
// @Param	callback_url	string true "callback_url"
// @Param	allow_ip	string true "allow_ip"
// @Param	flavor_id	string true "flavor_id"
// @Failure 403
// @router /create [post]
func (m *SCSController) Create() {
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
		ID          string `json:"requestId" bind:"required"`
		Name        string `json:"name" bind:"required"`
		AllowedIP   string `json:"allowedIp" bind:"required"`
		CallbackUrl string `json:"callbackUrl" bind:"required"`
		FlavorID    string `json:"flavorId"  bind:"required"`
	}

	params := RequestedParams{}
	if m.BindJSON(&params) != nil {
		return
	}
	id := params.ID
	flavorID := params.FlavorID // ics4  - 8gb -ram 4 vcpu
	allowedIp := fmt.Sprintf("%s/32", params.AllowedIP)
	callbackUrl := params.CallbackUrl

	diskSize := 30

	/* find default security group */
	securityGroupList, err := compute.GetCompute(claims.Username).ListSecGroups()
	if err != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), err.Error(), fmt.Sprintf("ifinance-%v", claims.UserID))
	}

	var sgIDs []interface{}
	for _, secGroup := range securityGroupList {
		if secGroup.Name == "default" {
			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "psql", 8080, 8080, "tcp", allowedIp)
			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "psql", 9990, 9990, "tcp", allowedIp)
			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "psql", 8282, 8282, "tcp", allowedIp)
			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "psql", 7077, 7077, "tcp", allowedIp)
			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "psql", 5432, 5432, "tcp", allowedIp)
			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "psql", 6060, 6060, "tcp", allowedIp)
			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "public", 80, 80, "tcp", "0.0.0.0/0")
			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "public", 443, 443, "tcp", "0.0.0.0/0")
			sgIDs = append(sgIDs, secGroup.Name)
		}
	}

	requestBody := map[string]interface{}{
		"subdomain": "fibo",
	}

	response, err := CreateRealm("https://accounts.252.mn/keyclock/api/v1/realms", requestBody)
	if err != nil {
		m.SetError(helper.StatusError, "Realm create error", err.Error(), "scs")
		return
	}
	fmt.Print(response)

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

	userData, err := ioutil.ReadFile("files/scs/base.yml")
	if err != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), err.Error(), "scs")
		return
	}

	lines := strings.Split(string(userData), "\n")

	/* generate random ssh name, password */
	generatedSshName := fmt.Sprintf("scs-%v", utils.RandSeq(6))
	generatedSshPassword := utils.RandSeq(10)
	generatedDB := utils.RandSeq(8)
	generatedDBUser := utils.RandSeq(5)
	generatedDBPass := utils.RandSeq(12)
	generatedDBRootPass := utils.RandSeq(15)

	fmt.Println("generatedSshName: ", generatedSshName, " generatedSshPassword: ", generatedSshPassword)

	for i, line := range lines {
		if strings.Contains(line, "[SSH_PASSWORD]") {
			lines[i] = strings.Replace(lines[i], "[SSH_PASSWORD]", generatedSshPassword, -1)
		}
		if strings.Contains(line, "[DB_ROOT_PASSWORD]") {
			lines[i] = strings.Replace(lines[i], "[DB_ROOT_PASSWORD]", generatedDBRootPass, -1)
		}
		if strings.Contains(line, "[DB]") {
			lines[i] = strings.Replace(lines[i], "[DB]", generatedDB, -1)
		}
		if strings.Contains(line, "[DB_USER]") {
			lines[i] = strings.Replace(lines[i], "[DB_USER]", generatedDBUser, -1)
		}
		if strings.Contains(line, "[DB_PASSWORD]") {
			lines[i] = strings.Replace(lines[i], "[DB_PASSWORD]", generatedDBPass, -1)
		}
		if strings.Contains(line, "[APP_ACCESS_SECRET]") {
			lines[i] = strings.Replace(lines[i], "[APP_ACCESS_SECRET]", response.Result.PublicKey, -1)
		}
		if strings.Contains(line, "[APP_CLIENT_ID]") {
			lines[i] = strings.Replace(lines[i], "[APP_CLIENT_ID]", response.Result.ClientId, -1)
		}
		if strings.Contains(line, "[APP_REALM]") {
			lines[i] = strings.Replace(lines[i], "[APP_REALM]", response.Result.Realm, -1)
		}
	}

	output := strings.Join(lines, "\n")
	fmt.Println(output)

	scripted := []byte(output)

	server, errServer := compute.GetCompute(claims.Username).CreateServerForVolume(imageID, params.Name, flavorID, "", int(diskSize), secGroup, scripted, "nova", true, networkID)
	if errServer != nil {
		fmt.Print(errServer)
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), err.Error(), fmt.Sprintf("ifinance-%v", claims.UserID))
		return
	}

	service.CreateLogAction(server.ID, "scs", id, "Create", claims.UserID, err)
	_, errGetFlavor := compute.GetCompute(claims.Username).GetFlavor(flavorID, "CLOUD.MN")
	if errGetFlavor != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), errServer.Error(), fmt.Sprintf("ifinance-%v", claims.UserID))
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
					ID          string `json:"requestId"`
					DBName      string `json:"databaseName"`
					DBUsername  string `json:"databaseUser"`
					DBPassword  string `json:"databasePwd"`
					SSHName     string `json:"sshName"`
					SSHPassword string `json:"sshPassword"`
					Ip          string `json:"databaseUrl"`
				}

				returnParams := ReturnParams{ID: id, DBName: "primeifinance", SSHName: "fibo", SSHPassword: generatedSshPassword, Ip: ip}
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

// Flavors ...
// @Title Flavors
// @Description hint Flavors
// @Failure 403
// @router /list [get]
func (m *SCSController) Flavors() {
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
		if strings.Contains(c.Name, "IFinance") {
			mainRes = append(mainRes, c)
		}

	}

	m.SetBody(mainRes)
}

// Action's ifinance stack ...
// @Title Actions
// @Description hint Actions
// @Param	id	string true "id"
// @Param	callback_url	string true "callback_url"
// @Param	allow_ip	string true "allow_ip"
// @Param	flavor_id	string true "flavor_id"
// @Failure 403
// @router /action [post]
func (m *SCSController) Action() {
	defer func() {
		if r := recover(); r != nil {
			m.RespondPanic(r)
		} else {
			m.Respond()
		}
	}()

	// RequestedParams ...
	type RequestedParams struct {
		InstanceID string `json:"id" bind:"required"`
		Action     string `json:"action" bind:"required"`
	}

	params := RequestedParams{}
	if m.BindJSON(&params) != nil {
		return
	}
	if params.Action == "start" {
		m.Start(params.InstanceID)
	} else if params.Action == "stop" {
		m.Stop(params.InstanceID)
	} else if params.Action == "restart" {
		m.Reboot(params.InstanceID)
	}
	m.SetBody(params)
}

func (m *SCSController) Start(instanceID string) {
	claims := m.Claim()
	clientIP := m.GetClientIP()
	errStart := compute.GetCompute(claims.Username).StartServer(instanceID, "CLOUD.MN")

	server, errServer := compute.GetCompute(claims.Username).GetServer(instanceID, "CLOUD.MN")
	flavor, err := compute.GetCompute(claims.Username).GetFlavor(server.Flavor["id"].(string), "CLOUD.MN")
	logID := service.CreateLogAction(instanceID, "Instance", server.Name, "Start", claims.UserID, errStart)
	myCache := utils.GetUCache()
	if errStart != nil {
		m.SetError(helper.StatusBadRequest, errStart.Error(), errStart.Error(), claims.UserID)
		return
	}
	o := orm.NewOrm()
	if errServer == nil {
		usageObj := models.UsgHistory{
			SysUserID:       uint32(claims.SysUserID),
			OsUserID:        claims.OsUserID,
			Type:            "Instance",
			Hostname:        server.Name,
			OsResourceID:    server.ID,
			OsInstanceID:    server.ID,
			Flavor:          flavor.Name,
			DiskSize:        0,
			CPU:             flavor.VCPUs,
			RAM:             flavor.RAM,
			Status:          "ACTIVE",
			StartDate:       time.Now().Format(helper.TimeFormatYYYYMMDDHHMMSS),
			EndDate:         "",
			IP:              clientIP,
			LastLogActionID: uint32(logID),
		}
		_, errTable := o.Insert(&usageObj)
		if errTable != nil {
			m.SetError(helper.StatusBadRequest, errTable.Error(), errTable.Error(), claims.UserID)
			return
		}
		myCache.Set(server.ID, helper.Starting)
	}

	service.CreateLogAction(instanceID, "Instance", server.Name, "Start", claims.UserID, err)
	if err != nil {
		m.SetError(helper.StatusBadRequest, err.Error(), err.Error(), claims.UserID)
	}
}

func (m *SCSController) Stop(instanceID string) {
	claims := m.Claim()
	myCache := utils.GetUCache()
	clientIP := m.GetClientIP()
	err := compute.GetCompute(claims.Username).StopServer(instanceID, "CLOUD.MN")
	if err != nil {
		m.SetError(helper.StatusBadRequest, err.Error(), err.Error(), claims.UserID)
		return
	}
	server, _ := compute.GetCompute(claims.Username).GetServer(instanceID, "CLOUD.MN")
	logID := service.CreateLogAction(instanceID, "Instance", server.Name, "Stop", claims.UserID, err)
	o := orm.NewOrm()
	_, errTable := o.QueryTable("usg_history").Filter("os_resource_id", instanceID).Filter("end_date", "").Update(orm.Params{
		"sys_user_id":        claims.SysUserID,
		"os_user_id":         claims.OsUserID,
		"status":             "SHUTOFF",
		"end_date":           time.Now().Format(helper.TimeFormatYYYYMMDDHHMMSS),
		"ip":                 clientIP,
		"last_log_action_id": logID,
	})

	if errTable != nil {
		m.SetError(helper.StatusBadRequest, err.Error(), err.Error(), claims.UserID)
		return
	}

	myCache.Set(server.ID, helper.STOPPING)
	service.CreateLogAction(instanceID, "Instance", server.Name, "Start", claims.UserID, err)
	if err != nil {
		m.SetError(helper.StatusBadRequest, err.Error(), err.Error(), claims.UserID)
	}
}

func (m *SCSController) Reboot(instanceID string) {
	claims := m.Claim()

	server, errServer := compute.GetCompute(claims.Username).GetServer(instanceID, "CLOUD.MN")
	if errServer != nil {
		m.SetError(helper.StatusBadRequest, errServer.Error(), errServer.Error(), claims.UserID)
	}
	errStart := compute.GetCompute(claims.Username).RebootServer(instanceID, "hard", "CLOUD.MN")
	service.CreateLogAction(instanceID, "Instance", server.Name, "Reboot", claims.UserID, errStart)
	if errStart != nil {
		m.SetError(helper.StatusBadRequest, errStart.Error(), errStart.Error(), claims.UserID)
		return
	}
	m.SetBody(nil)
}

// CreateRealm ...
func CreateRealm(url string, jsonData interface{}) (*ResponseRealm, error) {

	jsonValue, _ := json.Marshal(jsonData)
	response, err := http.Post(url, "application/json", bytes.NewBuffer(jsonValue))
	if err != nil {
		return nil, err
	}
	data, err := ioutil.ReadAll(response.Body)

	var obj ResponseRealm
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	return &obj, nil

}
