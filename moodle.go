package thirtdparty

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/astaxie/beego/orm"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"gitlab.com/ics-project/back-thirdparty/controllers/shared"
	"gitlab.com/ics-project/back-thirdparty/helper"
	"gitlab.com/ics-project/back-thirdparty/integration/compute"
	"gitlab.com/ics-project/back-thirdparty/models"
	"gitlab.com/ics-project/back-thirdparty/service"
	"gitlab.com/ics-project/back-thirdparty/utils"
)

// MoodleController struct
type MoodleController struct {
	shared.BaseController
}

// URLMapping URL mapping
func (m *MoodleController) URLMapping() {
	m.Mapping("Create", m.Create)
}

type MoodleCreateStruct struct {
	MoodleUser      string        `json:"md_user" bind:"required"`
	MoodlePass      string        `json:"md_pwd" bind:"required"`
	SgIDs           []interface{} `json:"sg_ids" bind:"required"`
	MoodleFullname  string        `json:"moodle_full_name" bind:"required"`
	MoodleShortname string        `json:"moodle_short_name" bind:"required"`
	DomainName      string        `json:"domain_name" bind:"required"`
}

// Create Moodle stack ...
// @Title Create
// @Description hint Create
// @Param	md_user	string	false	"md_user"
// @Param	md_pwd	string	false	"md_pwd"
// @Param	instance_name	string	false	"instance_name"
// @Param	sg_ids	[]string	false	"sg_ids"
// @Failure 403
// @router /create [post]
func (m *MoodleController) Create() {
	claims := m.Claim()

	defer func() {
		if r := recover(); r != nil {
			m.RespondPanic(r)
		} else {
			m.Respond()
		}
	}()
	params := MoodleCreateStruct{}
	if m.BindJSON(&params) != nil {
		return
	}

	clientIP := m.Ctx.Request.Header.Get("ClientIP")
	dbUser := "fibo"
	dbPwd := utils.RandSeq(10)
	dbName := "lmsDB"
	osUserID := claims.UserID
	flavorID := "7868fc2f-88b4-4aa9-96a0-d56519e2b910" // ics4  - 8gb -ram 4 vcpu
	diskSize := 50
	networkID := os.Getenv("public.network.id")
	imageID := "89b21a98-0ba6-46a3-8bac-bb210e289652"
	sysUserID := claims.SysUserID

	domainName := params.DomainName + ".ics.itools.mn"

	//region [DOMAIN CHECK, CREATE]
	isExist, errDomain := Check(domainName)
	if isExist {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusAlready), "", "")
		return
	}

	if errDomain != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), errDomain.Error(), "moodle")
		return
	}

	// өөрсдийн table дээрээс шалгах
	count, errDomainDB := models.CheckDomainName(domainName)
	if errDomainDB != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), errDomainDB.Error(), "moodle")
		return
	}

	if count > 0 {

		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusAlready), "", "")
		return
	}

	domainID, domainErr := models.CreateDomain(osUserID, domainName)
	if domainErr != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), domainErr.Error(), "moodle")
		return
	}

	errDb := models.CreateMoodleConfg(claims.UserID, dbUser, dbPwd, dbName, params.MoodleUser, params.MoodlePass, params.MoodleFullname, params.MoodleShortname, uint32(domainID))
	if errDb != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), errDb.Error(), "moodle")
		return
	}
	//endregion

	// var scripted []byte

	secGroup := make([]string, len(params.SgIDs))
	for i, v := range params.SgIDs {
		secGroup[i] = fmt.Sprint(v)
	}

	userData, err := ioutil.ReadFile("files/moodle/base.yml")
	if err != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), err.Error(), "moodle")
		return
	}

	lines := strings.Split(string(userData), "\n")
	for i, line := range lines {
		if strings.Contains(line, "dbUser") {
			lines[i] = strings.Replace(lines[i], "dbUser", dbUser, -1)
		}

		if strings.Contains(line, "dbPwd") {
			lines[i] = strings.Replace(lines[i], "dbPwd", dbPwd, -1)
		}

		if strings.Contains(line, "dbName") {
			lines[i] = strings.Replace(lines[i], "dbName", dbName, -1)
		}

		if strings.Contains(line, "mdUser") {
			lines[i] = strings.Replace(lines[i], "mdUser", params.MoodleUser, -1)
		}

		if strings.Contains(line, "mdPwd") {
			lines[i] = strings.Replace(lines[i], "mdPwd", params.MoodlePass, -1)
		}
		if strings.Contains(line, "email") {
			lines[i] = strings.Replace(lines[i], "email", claims.Email, -1)
		}
		if strings.Contains(line, "moodle_full_name") {
			lines[i] = strings.Replace(lines[i], "moodle_full_name", params.MoodleFullname, -1)
		}
		if strings.Contains(line, "moodle_short_name") {
			lines[i] = strings.Replace(lines[i], "moodle_short_name", params.MoodleShortname, -1)
		}
		if strings.Contains(line, "MOODLE_DOMAIN_NAME") {
			lines[i] = strings.Replace(lines[i], "MOODLE_DOMAIN_NAME", "https://"+domainName, -1)
		}
		if strings.Contains(line, "[DMN]") {
			lines[i] = strings.Replace(lines[i], "[DMN]", domainName, -1)
		}
	}

	output := strings.Join(lines, "\n")
	fmt.Println(output)

	scripted := []byte(output)

	server, errServer := compute.GetCompute(claims.Username).CreateServerForVolume(imageID, domainName, flavorID, "", int(diskSize), secGroup, scripted, "nova", true, networkID)
	if errServer != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), err.Error(), fmt.Sprintf("moodle-%v", claims.UserID))
		return
	}

	logID := service.CreateLogAction(server.ID, "Moodle", domainName, "Create", claims.UserID, err)
	flavorRes, errGetFlavor := compute.GetCompute(claims.Username).GetFlavor(flavorID, "CLOUD.MN")
	if errGetFlavor != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), errServer.Error(), fmt.Sprintf("moodle-%v", claims.UserID))
	}

	go func(serverid string, username string, logID int64) {
		provider := service.GetProvider(username)
		for {
			server, _ := servers.Get(service.GetClientCompute(provider), serverid).Extract()
			if server.Status == "ACTIVE" {
				service.CreateUsageAction(uint32(sysUserID), osUserID, "Instance", domainName, server.ID, server.ID, flavorRes.Name, "ACTIVE", clientIP, logID, time.Now(), time.Time{}, 0, flavorRes.VCPUs, flavorRes.RAM, true)

				for _, l := range server.Addresses["public1"].([]interface{}) {
					o := orm.NewOrm()
					tmp := l.(map[string]interface{})

					o.QueryTable("domains").Filter("id", domainID).Update(orm.Params{
						"ip": tmp["addr"].(string),
					})

					service.CreateUsageAction(uint32(sysUserID), osUserID, "IP", tmp["addr"].(string), "", server.ID, "", "ACTIVE", tmp["addr"].(string), logID, time.Now(), time.Time{}, 0, 0, 0, true)
				}

				for _, volume := range server.AttachedVolumes {
					service.CreateUsageAction(uint32(sysUserID), osUserID, "Volume", volume.ID, volume.ID, server.ID, "", "ACTIVE", clientIP, logID, time.Now(), time.Time{}, int(diskSize), 0, 0, true)
				}

				fmt.Println(server.AttachedVolumes)
				return
			}
			if server.Status == "ERROR" {
				return
			}
			time.Sleep(3 * time.Second)
		}
	}(server.ID, claims.Username, logID)

	m.SetBody(server)
}
