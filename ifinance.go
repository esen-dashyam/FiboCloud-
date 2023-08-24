package thirtdparty

import (
	"fmt"
	"strings"
	"time"

	"github.com/astaxie/beego/orm"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/flavors"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/images"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"

	"gitlab.com/ics-project/back-thirdparty/controllers/shared"
	"gitlab.com/ics-project/back-thirdparty/helper"
	"gitlab.com/ics-project/back-thirdparty/integration/blockstorage"
	"gitlab.com/ics-project/back-thirdparty/integration/compute"
	"gitlab.com/ics-project/back-thirdparty/integration/networking"
	"gitlab.com/ics-project/back-thirdparty/integration/project"
	"gitlab.com/ics-project/back-thirdparty/models"
	"gitlab.com/ics-project/back-thirdparty/service"
	"gitlab.com/ics-project/back-thirdparty/utils"
)

// IFinanceController struct
type IFinanceController struct {
	shared.BaseController
}

// URLMapping URL mapping
func (m *IFinanceController) URLMapping() {
	m.Mapping("Create", m.Create)
	m.Mapping("Action", m.Action)
	m.Mapping("List", m.Flavors)
	m.Mapping("Images", m.Images)
	//    m.Mapping("Update", m.Update)
}

// Create ifinance stack ...
// @Title Create
// @Description hint Create
// @Param   id  string true "id"
// @Param   callback_url    string true "callback_url"
// @Param   allow_ip    string true "allow_ip"
// @Param   flavor_id   string true "flavor_id"
// @Failure 403
// @router /create [post]

func (m *IFinanceController) Create() {
	client := m.Ctx.Request.Header.Get("ICSAccess")

	claims := m.Claim()

	defer func() {
		if r := recover(); r != nil {
			m.RespondPanic(r)
		} else {
			m.Respond()
		}
	}()
	type PortParams struct {
		Port   int    `json:"port"`
		Source string `json:"source"`
		Name   string `json:"name"`
	}
	type Env struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	type IP_config struct {
		IP_address int 	`json:"IP_address"`
		controller 
	}
	// RequestedParams ...
	type RequestedParams struct {
		ID          string       `json:"requestId" bind:"required"`
		Name        string       `json:"name" bind:"required"`
		ProjectId   string       `json:"projectId" bind:"required"`
		CallbackUrl string       `json:"callbackUrl" bind:"required"`
		Image       string       `json:"imageImage"`
		FlavorID    string       `json:"flavorId"`
		CPU         int          `json:"cpu"`
		RAM         float64      `json:"ram"`
		Disk        int          `json:"disk"`
		IsNew       bool         `json:"isNew"`
		IsHDD       bool         `json:"isHDD"`
		Ports       []PortParams `json:"ports"`
		Env         []Env        `json:"env"`
	}

	// [{ port: 8081, allowed_ip: "192.168.0.1" },{ port: 8081, allowed_ip: "192.168.0.1" },{ port: 8081, allowed_ip: "192.168.0.1" }]

	params := RequestedParams{}
	if m.BindJSON(&params) != nil {
		return
	}
	params.IsHDD = true

	// login to project
	project.SetCurrentStack(claims.Email, params.ProjectId, "CLOUD.MN", claims.OsUserID, "", "")
	stack := project.GetCurrentStack(claims.Email)

	if stack.IsICS {
		m.SetError(helper.StatusError, "Failed login to project", "Failed login to project", claims.UserID)
		fmt.Println("Failed login to project. ")
		return
	}
	fmt.Println("\n✅✅✅ Successfully logged into the project: ", stack.ProjectID, stack.Tag)
	// create instance

	diskSize := 30
	id := params.ID
	flavorID := params.FlavorID // ics4  - 8gb -ram 4 vcpu
	callbackUrl := params.CallbackUrl

	if params.Disk > 0 {
		diskSize = params.Disk
	}

	/* find default security group */
	securityGroupList, err := compute.GetCompute(claims.Username).ListSecGroups()
	if err != nil {
		m.SetError(helper.StatusMissingParams, "Failed fetching security group list", "Failed fetching security group list", fmt.Sprintf("ifinance-%v", claims.UserID))
		fmt.Println("Failed fetching security group list")
	}
	var sgIDs []interface{}
	for _, secGroup := range securityGroupList {
		if secGroup.Name == "default" {
			for _, port := range params.Ports {
				networking.GetNetworking(claims.Username).CreateSecGroupRule("ingres", "", secGroup.ID, port.Name, port.Port, port.Port, "tcp", port.Source)
			}
			sgIDs = append(sgIDs, secGroup.Name)
		}
	}

	networkID := ""
	imageID := models.GetConfig("3thparty_image")
	if params.IsNew {
		imageID = models.GetConfig("image_ifinance_new")
	}
	secGroup := make([]string, len(sgIDs))
	for i, v := range sgIDs {
		secGroup[i] = fmt.Sprint(v)
	}
	userData, err := models.GetCloudInitByName(client)
	if err != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), err.Error(), client)
		return
	}

	lines := strings.Split(string(userData.YML), "\n")

	//  generate random ssh name, password */
	// generatedSshName := fmt.Sprintf("ifinance-%v", utils.RandSeq(6))
	generatedSshPassword1 := utils.RandSeq(10)
	generatedSshPassword2 := utils.RandSeq(10)

	// fmt.Println("generatedSshName: ", generatedSshName, " generatedSshPassword: ", generatedSshPassword1)
	// for i, _ := range lines {
	// 	for _, env := range params.Env {
	// 		lines[i] = strings.Replace(lines[i], env.Key, env.Value, -1)
	// 	}
	// }
	for i, line := range lines {
		if strings.Contains(line, "[SSH_PASSWORD_1]") {
			lines[i] = strings.Replace(lines[i], "[SSH_PASSWORD_1]", generatedSshPassword1, -1)
		}

		if strings.Contains(line, "[SSH_PASSWORD_2]") {
			lines[i] = strings.Replace(lines[i], "[SSH_PASSWORD_2]", generatedSshPassword2, -1)
		}
	}

	output := strings.Join(lines, "\n")
	fmt.Println(output)

	scripted := []byte(output)

	if len(params.FlavorID) == 0 {
		provider1, _ := shared.AuthOSAdmin()
		clientAdmin, err := openstack.NewComputeV2(provider1, gophercloud.EndpointOpts{
			Region: "RegionOne",
		})
		if err != nil {
			m.SetError(helper.StatusError, err.Error(), err.Error(), claims.UserID)
			return
		}
		flavor, err := compute.GetCompute(claims.Username).CreateFlavor(clientAdmin, id+"_ifinance", diskSize, int(params.RAM)*1024, params.CPU, 1.0, true)
		flavorID = flavor.ID
		diskSize = flavor.Disk
		if err != nil {
			m.SetError(helper.StatusError, err.Error(), err.Error(), claims.UserID)
			return
		}
	}
	var server *servers.Server
	var errServer error
	if !params.IsHDD {
		volume, err1 := blockstorage.GetBlockstorage(claims.Username).CreateVolumeFromImage(diskSize, fmt.Sprintf("%s-volume", params.Name), fmt.Sprintf("%s is volume", params.Name), "ssd", imageID)
		if err1 != nil {
			m.SetError(helper.StatusError, errServer.Error(), errServer.Error(), claims.UserID)
			return
		}
		blockstorage.GetBlockstorage(claims.Username).WaitVolumeStat(volume.ID, "Available")
		server, errServer = compute.GetCompute(claims.Username).CreateServerForSSDVolume(volume.ID, params.Name, flavorID, "", diskSize, secGroup, scripted, "nova", true, networkID)
		if errServer != nil {
			m.SetError(helper.StatusError, errServer.Error(), errServer.Error(), claims.UserID)
			return
		}

	} else {
		server, errServer = compute.GetCompute(claims.Username).CreateServerForVolume(imageID, params.Name, flavorID, "", diskSize, secGroup, scripted, "nova", true, networkID)

		if errServer != nil {
			m.SetError(helper.StatusError, errServer.Error(), errServer.Error(), claims.UserID)
			return
		}
	}
	service.CreateLogAction(server.ID, "ifinance", id, "Create", claims.UserID, err)
	_, errGetFlavor := compute.GetCompute(claims.Username).GetFlavor(flavorID, "CLOUD.MN")
	if errGetFlavor != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), errServer.Error(), fmt.Sprintf("ifinance-%v", claims.UserID))
		return
	}

	go func(serverid string, username string) {
		provider := service.GetProvider(username)
		for {
			server, _ := servers.Get(service.GetClientCompute(provider), serverid).Extract()
			if server.Status == "ACTIVE" {
				var ip string
				if server.Addresses["public-1"] != nil {
					for _, l := range server.Addresses["public-1"].([]interface{}) {
						tmp := l.(map[string]interface{})
						ip = tmp["addr"].(string)
					}
				}
				if server.Addresses["public-2"] != nil {
					for _, l := range server.Addresses["public-2"].([]interface{}) {
						tmp := l.(map[string]interface{})
						ip = tmp["addr"].(string)

					}
				}
				type ReturnParams struct {
					ID           string `json:"requestId"`
					DBName       string `json:"databaseName"`
					DBUsername   string `json:"databaseUser"`
					DBPassword   string `json:"databasePwd"`
					SSHName1     string `json:"sshName1"`
					SSHPassword1 string `json:"sshPassword1"`
					SSHName2     string `json:"sshName2"`
					SSHPassword2 string `json:"sshPassword2"`
					Ip           string `json:"databaseUrl"`
				}

				returnParams := ReturnParams{ID: id, DBName: "prime finance", SSHName1: "fibo", SSHPassword1: generatedSshPassword1, SSHName2: "ifinance", SSHPassword2: generatedSshPassword2, Ip: ip}
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
func (m *IFinanceController) Flavors() {
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
		if strings.Contains(c.Name, "cloud") {
			mainRes = append(mainRes, c)
		}

	}

	m.SetBody(mainRes)
}

// Flavors ...
// @Title Flavors
// @Description hint Flavors
// @Failure 403
// @router /delete-project [delete]
func (m *IFinanceController) DeleteProject() {
	claims := m.Claim()
	defer func() {
		if r := recover(); r != nil {
			m.RespondPanic(r)
		} else {
			m.Respond()
		}
	}()
	type RequestedParams struct {
		ProjectID string `json:"projectId"`
	}

	params := RequestedParams{}
	if m.BindJSON(&params) != nil {
		return
	}
	o := orm.NewOrm()

	var defaultProjectId string

	/* rules
	   You can't delete default project of the user of openstack
	   You can't delete project with bill invoice
	*/
	err := o.Raw("select su.os_tenant_id from sys_user su where su.os_user_id in (?)", claims.OsUserID).QueryRow(&defaultProjectId)
	if err != nil {
		m.SetError(helper.StatusMissingParams, "Cannot find project user", "Cannot find project user", claims.UserID)
		return
	}

	if defaultProjectId == params.ProjectID {
		m.SetError(helper.StatusMissingParams, "Cannot delete default project", "Cannot delete default project", claims.UserID)
		return
	}

	var count int
	err = o.Raw("select count(bi.id) as count from bill_invoice bi where bi.project_id in (?)", params.ProjectID).QueryRow(&count)
	if err != nil {
		m.SetError(helper.StatusMissingParams, "Error querying bill invoice", "Error querying bill invoice", claims.UserID)
		return
	}
	if count > 0 {
		m.SetError(helper.StatusMissingParams, fmt.Sprintf("Project has bill invoices %v", count), fmt.Sprintf("Project has bill invoices %v", count), claims.UserID)
		return
	}

	// passed everything now delete the project, delete everything within the project
	success, err := project.DeleteProject(project.DeleteProjectStruct{
		ProjectID: params.ProjectID,
		Email:     claims.Email,
	})

	if err != nil {
		m.SetError(helper.StatusMissingParams, "Error deleting project: "+err.Error(), "Error deleting project: "+err.Error(), claims.UserID)
		return
	}

	m.SetBody(map[string]interface{}{
		"project_id": params.ProjectID,
		"success":    success,
	})
}

// Action's ifinance stack ...
// @Title Actions
// @Description hint Actions
// @Param   id  string true "id"
// @Param   callback_url    string true "callback_url"
// @Param   allow_ip    string true "allow_ip"
// @Param   flavor_id   string true "flavor_id"
// @Failure 403
// @router /action [post]
func (m *IFinanceController) Action() {
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
		InstanceID string `json:"id" bind:"required"`
		Action     string `json:"action" bind:"required"`
		ProjectID  string `json:"project_id" bind:"required"`
		Tag        string `json:"tag" bind:"required"`
	}

	params := RequestedParams{}
	if m.BindJSON(&params) != nil {
		return
	}

	// login to new project
	project.SetCurrentStack(claims.Email, params.ProjectID, params.Tag, claims.OsUserID, "", "")
	stack := project.GetCurrentStack(claims.Email)

	if stack.IsICS {
		m.SetError(helper.StatusError, "Failed login to new project", "Failed login to new project", claims.UserID)
		fmt.Println("Failed login to new project. ")
		return
	}
	fmt.Println("✅✅✅ Successfully logged into the new project: ", stack.ProjectID, stack.Tag)

	if params.Action == "start" {
		m.Start(params.InstanceID, params.Tag)
	} else if params.Action == "stop" {
		m.Stop(params.InstanceID, params.Tag)
	} else if params.Action == "restart" {
		m.Reboot(params.InstanceID, params.Tag)
	}
	m.SetBody(params)
}

func (m *IFinanceController) Start(instanceID, tag string) {
	claims := m.Claim()
	clientIP := m.GetClientIP()

	errStart := compute.GetCompute(claims.Username).StartServer(instanceID, tag)

	server, errServer := compute.GetCompute(claims.Username).GetServer(instanceID, tag)
	flavor, err := compute.GetCompute(claims.Username).GetFlavor(server.Flavor["id"].(string), tag)
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

func (m *IFinanceController) Stop(instanceID, tag string) {
	claims := m.Claim()
	myCache := utils.GetUCache()
	clientIP := m.GetClientIP()
	err := compute.GetCompute(claims.Username).StopServer(instanceID, tag)
	if err != nil {
		m.SetError(helper.StatusBadRequest, err.Error(), err.Error(), claims.UserID)
		return
	}
	server, _ := compute.GetCompute(claims.Username).GetServer(instanceID, tag)
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

func (m *IFinanceController) Reboot(instanceID, tag string) {
	claims := m.Claim()

	server, errServer := compute.GetCompute(claims.Username).GetServer(instanceID, tag)
	if errServer != nil {
		m.SetError(helper.StatusBadRequest, errServer.Error(), errServer.Error(), claims.UserID)
	}
	errStart := compute.GetCompute(claims.Username).RebootServer(instanceID, "hard", tag)
	service.CreateLogAction(instanceID, "Instance", server.Name, "Reboot", claims.UserID, errStart)
	if errStart != nil {
		m.SetError(helper.StatusBadRequest, errStart.Error(), errStart.Error(), claims.UserID)
		return
	}
	m.SetBody(nil)
}

//Code below will update existing Project

// Code to check if data exists in database
// func (m *IFinanceController) Update() {
//    claims := m.Claim()

//    defer func() {
//        if r := recover(); r != nil {
//            m.RespondPanic(r)
//        } else {
//            m.Respond()
//        }
//    }()

//    params := project.UpdateProjectStruct{}
//    if m.BindJSON(&params) != nil {
//        return
//    }
//    updatedProject, err := project.UpdateProject(project.UpdateProjectStruct{
//        Email:       claims.Email,
//        OsUserID:    claims.OsUserID,
//        Purpose:     14,
//        Register:    "",
//        Description: claims.Email,
//    })
//    if err != nil {
//        m.SetError(helper.StatusError, "Failed Updating Project", "Failed Updating Project", "")
//    }
//    m.SetBody(updatedProject)

// }

// func (m* IFinanceController) Update {

// // }
func (m *IFinanceController) Images() {
	claims := m.Claim()
	var mainRes []images.Image
	defer func() {
		if r := recover(); r != nil {
			m.RespondPanic(r)
		} else {
			m.Respond()
		}
	}()

	listImages, err := compute.GetCompute(claims.Username).ListImage()
	if err != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), err.Error(), claims.UserID)
		return
	}
	for _, z := range listImages {
		if strings.Contains(z.Name, "admin") {
			mainRes = append(mainRes, z)
		}

	}
	m.SetBody(mainRes)
}
