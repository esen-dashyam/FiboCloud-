package thirtdparty

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"time"

	"gitlab.com/ics-project/back-thirdparty/helper"
	"gitlab.com/ics-project/back-thirdparty/models"

	"github.com/astaxie/beego/orm"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"gitlab.com/ics-project/back-thirdparty/controllers/shared"
	"gitlab.com/ics-project/back-thirdparty/integration/compute"
	"gitlab.com/ics-project/back-thirdparty/service"
	"gitlab.com/ics-project/back-thirdparty/utils"
)

// MeetingController struct
type MeetingController struct {
	shared.BaseController
}

// URLMapping URL mapping
func (m *MeetingController) URLMapping() {
	m.Mapping("Create", m.Create)
}

// Check ...
func Check(checkDomain string) (isExist bool, err error) {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String("ap-east-1"), //
		Credentials: credentials.NewStaticCredentials("AKIARY32T3ZFMN7F5EFL", "T+jeJmFQvNKw49zdchFb/8+SDIrJ7Sic/fvfUD0a", ""),
	})
	if err != nil {
		return false, err
	}

	svc := route53.New(sess)
	opts := route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String("Z3OE19WQV8WACN"),
	}

	outputs, err := svc.ListResourceRecordSets(&opts)
	if err != nil {
		return false, err
	}

	for _, domain := range outputs.ResourceRecordSets {
		realName := *domain.Name
		fmt.Println(realName, checkDomain)

		if realName == checkDomain+"." {
			return true, nil
		}
	}

	return false, nil
}

// MeetingCreateStruct ...

type MeetingCreateStruct struct {
	Domain        string        `json:"domain_name" bind:"required"`
	SgIDs         []interface{} `json:"sg_ids" bind:"required"`
	AdminEmail    string        `json:"admin_email" bind:"required"`
	AdminPassword string        `json:"admin_password" bind:"required"`
}

// Create Moodle stack ...
// @Title Create
// @Description hint Create
// @Param	domain_name	string	false	"domain_name"
// @Param	admin_email	string	false	"admin_email"
// @Param	admin_password	string	false	"admin_password"
// @Param	sg_ids	[]string	false	"sg_ids"
// @Failure 403
// @router /create [post]
func (m *MeetingController) Create() {
	claims := m.Claim()
	utils.SetLumlog(claims.Email)

	defer func() {
		if r := recover(); r != nil {
			m.RespondPanic(r)
		} else {
			m.Respond()
		}
	}()
	params := MeetingCreateStruct{}
	if m.BindJSON(&params) != nil {
		return
	}

	clientIP := m.Ctx.Request.Header.Get("ClientIP")
	osUserID := claims.UserID
	domainName := params.Domain + ".ics.itools.mn"
	adminEmail := params.AdminEmail
	adminPassword := params.AdminPassword
	flavorID := "146aef2b-98ba-4c5e-8bb9-33f5aa8664ec"
	diskSize := 50
	networkID := os.Getenv("public.network.id")
	imageID := "89b21a98-0ba6-46a3-8bac-bb210e289652"
	sysUserID := claims.SysUserID

	//region [DOMAIN CHECK, CREATE]
	isExist, errDomain := Check(domainName)
	if isExist {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusAlready), "", "")
		return
	}

	if errDomain != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), errDomain.Error(), "meeting")
		return
	}

	// өөрсдийн table дээрээс шалгах
	count, errDomainDB := models.CheckDomainName(domainName)
	if errDomainDB != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), errDomainDB.Error(), "meeting")
		return
	}

	if count > 0 {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusAlready), "", "")
		return
	}

	domainID, domainErr := models.CreateDomain(osUserID, domainName)
	if domainErr != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), domainErr.Error(), "meeting")
		return
	}

	errDb := models.CreateMeeting(claims.UserID, adminEmail, adminPassword, uint32(domainID))
	if errDb != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), errDb.Error(), "meeting")
		return
	}
	//endregion

	//region [CREATE]

	secGroup := make([]string, len(params.SgIDs))
	for i, v := range params.SgIDs {
		secGroup[i] = fmt.Sprint(v)
	}

	userData, err := ioutil.ReadFile("files/jitsi/base.yml")
	if err != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), err.Error(), fmt.Sprintf("meeting-%v", claims.UserID))
		return
	}

	lines := strings.Split(string(userData), "\n")
	for i, line := range lines {
		if strings.Contains(line, "[DMN]") {
			lines[i] = strings.Replace(lines[i], "[DMN]", domainName, -1)
		}
		if strings.Contains(line, "[RCK_ADMN_PWD]") {
			lines[i] = strings.Replace(lines[i], "[RCK_ADMN_PWD]", adminPassword, -1)
		}
		if strings.Contains(line, "[RCK_ADMN_EML]") {
			lines[i] = strings.Replace(lines[i], "[RCK_ADMN_EML]", adminEmail, -1)
		}
	}

	output := strings.Join(lines, "\n")
	fmt.Println(output)
	scripted := []byte(output)

	server, errServer := compute.GetCompute(claims.Username).CreateServerForVolume(imageID, domainName, flavorID, "", int(diskSize), secGroup, scripted, "nova", true, networkID)
	if errServer != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), errServer.Error(), fmt.Sprintf("meeting-%v", claims.UserID))
		return
	}

	logID := service.CreateLogAction(server.ID, "Moodle", domainName, "Create", claims.UserID, err)
	flavorRes, errGetFlavor := compute.GetCompute(claims.Username).GetFlavor(flavorID, "CLOUD.MN")
	if errGetFlavor != nil {
		m.SetError(helper.StatusMissingParams, helper.StatusText(helper.StatusError), err.Error(), fmt.Sprintf("meeting-%v", claims.UserID))
	}

	go func(serverid string, username string, logID int64) {
		provider := service.GetProvider(username)
		for {
			server, _ := servers.Get(service.GetClientCompute(provider), serverid).Extract()
			if server.Status == "ACTIVE" {
				service.CreateUsageAction(uint32(sysUserID), osUserID, "Instance", domainName, server.ID, server.ID, flavorRes.Name, "ACTIVE", clientIP, logID, time.Now(), time.Time{}, 0, flavorRes.VCPUs, flavorRes.RAM, true)

				for _, l := range server.Addresses["public1"].([]interface{}) {
					tmp := l.(map[string]interface{})
					o := orm.NewOrm()
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
	//endregion

	m.SetBody(server)
}
