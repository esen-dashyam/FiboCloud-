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

// BBXController struct
type BBXController struct {
	shared.BaseController
}

// URLMapping URL mapping
func (m *BBXController) URLMapping() {
	m.Mapping("Create", m.Create)
	m.Mapping("List", m.FlavorBBX)
}

// Create bbx stack ...
// @Title Create
// @Description hint Create
// @Param	id	string true "id"
// @Param	callback_url	string true "callback_url"
// @Param	allow_ip	string true "allow_ip"
// @Param	flavor_id	string true "flavor_id"
// @Failure 403
// @router /create [post]
func (m *BBXController) Create() {
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
		ID          string `json:"id" bind:"required"`
		AllowedIP   string `json:"allowed_ip" bind:"required"`
		CallbackUrl string `json:"callback_url" bind:"required"`
		FlavorID    string `json:"flavor_id"  bind:"required"`
	}

	params := RequestedParams{}
	if m.BindJSON(&params) != nil {
		return
	}
	id := params.ID
	flavorID := params.FlavorID // ics4  - 8gb -ram 4 vcpu
	allowedIp := fmt.Sprintf("%s/32", params.AllowedIP)
	callbackUrl := params.CallbackUrl

	diskSize := 10

	/* find default security group */
	securityGroupList, err := compute.GetCompute(claims.Username).ListSecGroups()
	if err != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), err.Error(), fmt.Sprintf("bbx-%v", claims.UserID))
	}

	var sgIDs []interface{}
	for _, secGroup := range securityGroupList {
		if secGroup.Name == "default" {
			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "psql", 5432, 5432, "tcp", allowedIp)

			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "bbx-admin", 8081, 8081, "tcp", allowedIp)

			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "bbx-core", 8082, 8082, "tcp", allowedIp)

			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "bbx-scheduler", 8083, 8083, "tcp", allowedIp)

			networking.GetNetworking(claims.Username).CreateSecGroupRule("ingress", "", secGroup.ID, "bbx-uaa", 8084, 8084, "tcp", allowedIp)

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

	userData, err := ioutil.ReadFile("files/bbx/base.yml")
	if err != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), err.Error(), "bbx")
		return
	}

	lines := strings.Split(string(userData), "\n")

	generatedUsername := utils.RandSeq(5)
	generatedPassword := utils.RandSeq(10)

	for i, line := range lines {
		if strings.Contains(line, "{USERNAME}") {
			lines[i] = strings.Replace(lines[i], "{USERNAME}", generatedUsername, -1)
		}
		if strings.Contains(line, "{PASSWORD}") {
			lines[i] = strings.Replace(lines[i], "{PASSWORD}", generatedPassword, -1)
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

	service.CreateLogAction(server.ID, "bbx", id, "Create", claims.UserID, err)
	_, errGetFlavor := compute.GetCompute(claims.Username).GetFlavor(flavorID, "CLOUD.MN")
	if errGetFlavor != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), errServer.Error(), fmt.Sprintf("bbx-%v", claims.UserID))
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
					ID         string `json:"requestId"`
					DBName     string `json:"databaseName"`
					DBUsername string `json:"databaseUser"`
					DBPassword string `json:"databasePwd"`
					Ip         string `json:"databaseUrl"`
				}

				returnParams := ReturnParams{ID: id, DBName: "primebbx", DBUsername: generatedUsername, DBPassword: generatedPassword, Ip: ip}
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

// FlavorBBX ...
// @Title FlavorBBX
// @Description hint FlavorBBX
// @Failure 403
// @router /list [get]
func (m *BBXController) FlavorBBX() {

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
		if strings.Contains(c.Name, "BBX") {
			mainRes = append(mainRes, c)
		}

	}

	m.SetBody(mainRes)
}
