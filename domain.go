package shared

import (
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"gitlab.com/ics-project/back-thirdparty/structs"
)

// DomainController struct
type DomainController struct {
	BaseController
}

// CheckDomainAWS ...
func CheckDomainAWS(domainName string) (response structs.ResponseBody) {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String("ap-east-1"), //
		Credentials: credentials.NewStaticCredentials("AKIARY32T3ZFMN7F5EFL", "T+jeJmFQvNKw49zdchFb/8+SDIrJ7Sic/fvfUD0a", ""),
	})
	if err != nil {
		return
	}

	svc := route53.New(sess)
	opts := route53.ListResourceRecordSetsInput{
		HostedZoneId: aws.String("Z3OE19WQV8WACN"),
	}

	outputs, err := svc.ListResourceRecordSets(&opts)

	for _, domain := range outputs.ResourceRecordSets {
		realName := *domain.Name
		if realName == domainName+".ics.itools.mn." {
			response.StatusCode = 1
			response.ErrorMsg = "Domain is already existed"
			return
		}
	}

	response.Body = outputs
	return
}

// DeleteDomainAWS ...
func DeleteDomainAWS(domainName, ipAddress string) (response structs.ResponseBody) {
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String("ap-east-1"), //
		Credentials: credentials.NewStaticCredentials("AKIARY32T3ZFMN7F5EFL", "T+jeJmFQvNKw49zdchFb/8+SDIrJ7Sic/fvfUD0a", ""),
	})
	if err != nil {
		return
	}

	svc := route53.New(sess)
	request := route53.ChangeResourceRecordSetsInput{
		ChangeBatch: &route53.ChangeBatch{
			Changes: []*route53.Change{
				{
					Action: aws.String("DELETE"),
					ResourceRecordSet: &route53.ResourceRecordSet{
						Name: aws.String(domainName),
						ResourceRecords: []*route53.ResourceRecord{
							{
								Value: aws.String(ipAddress),
							},
						},
						Type: aws.String("A"),
						TTL:  aws.Int64(300),
					},
				},
			},
		},
		HostedZoneId: aws.String("Z3OE19WQV8WACN"),
	}

	resp, err := svc.ChangeResourceRecordSets(&request)
	if err != nil {
		response.StatusCode = 1
		response.ErrorMsg = err.Error()
		fmt.Println("Unable to delete DNS Record", err)
		return
	}
	fmt.Println(resp)
	response.Body = ""
	return
}
