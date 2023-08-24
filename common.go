package shared

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/ioutil"
	"time"

	"github.com/gophercloud/utils/gnocchi/metric/v1/measures"
	"gitlab.com/ics-project/back-thirdparty/integration/networking"
)

// CommonController ...
type CommonController struct {
	BaseController
}

// TimeRange struct
type TimeRange struct {
	Start time.Time
	End   time.Time
	Value float64
}

// Event ...
type Event struct {
	EventType string `json:"eventType"`
	Data      struct {
		InvoiceNumber string `json:"invoiceNumber"`
		Description   string `json:"description"`
		Status        int    `json:"status"`
		Amount        int    `json:"amount"`
		TrackingData  string `json:"trackingData"`
		CreatedAt     string `json:"createdAt"`
		ExpireDate    string `json:"expireDate"`
		PaidDate      string `json:"paidDate"`
	} `json:"data"`
	Signature string `json:"signature,omitempty"`
}

// VerifySignature ...
func VerifySignature(pub *rsa.PublicKey, data []byte) (*Event, bool) {
	parsedData := new(Event)
	if err := json.Unmarshal(data, &parsedData); err != nil {
		return nil, false
	}

	signatureBytes, err := base64.StdEncoding.DecodeString(parsedData.Signature)
	if err != nil {
		return nil, false
	}

	parsedData.Signature = ""
	toVerify, err := json.Marshal(parsedData)
	if err != nil {
		return nil, false
	}

	hash := sha256.Sum256(toVerify)
	return parsedData, rsa.VerifyPKCS1v15(pub, crypto.SHA256, hash[:], signatureBytes) == nil
}

// CalculateNetworkByte ...
func CalculateNetworkByte(networkByte map[int](map[string]float64)) float64 {
	var amount float64
	amount = 0

	//loop except cpu_util measure
	for i := 0; i <= len(networkByte); i++ {
		for _, v := range networkByte[i] {
			amount += v
		}
	}

	return amount
}

// RetrieveDataFromBody ...
func RetrieveDataFromBody(body io.ReadCloser) (result map[string]interface{}) {
	var bodyBytes []byte
	if body != nil {
		bodyBytes, _ = ioutil.ReadAll(body)
	}
	json.Unmarshal(bodyBytes, &result)
	return result
}

// CheckBodyResult check request body variables
func CheckBodyResult(bodyResult map[string]interface{}, names []string) (statusCode int, ErrMsg string) {
	for _, n := range names {
		if bodyResult[n] == nil {
			statusCode = 100
			ErrMsg = n + ","
		}
	}

	if len(ErrMsg) != 0 {
		statusCode = 100
		ErrMsg = ErrMsg[:len(ErrMsg)-1]
		ErrMsg += " [value was missed]"
		return
	}
	statusCode = 0
	return
}

// FindSubnetFromIPAddress ...
func FindSubnetFromIPAddress(ports []networking.PortWithBinding, IPAddress string) string {
	for _, port := range ports {
		for _, fixedIPs := range port.FixedIPs {
			if IPAddress == fixedIPs.IPAddress {
				return fixedIPs.SubnetID
			}
		}
	}
	return ""
}

// InTimeSpan ...
func InTimeSpan(start, end, check time.Time) bool {
	if check.After(start) && check.Before(end) {
		return true
	}
	if start == check || end == check {
		return true
	}
	return false
}

// CalculateTimeRange ...
func CalculateTimeRange(measure []measures.Measure) []TimeRange {
	var result []TimeRange
	// var startVal float64
	startCheck := false
	if len(measure) != 0 {
		var timerange TimeRange
		for _, m := range measure {
			if startCheck == false {
				timerange.Start = m.Timestamp
				timerange.Value = m.Value
				// startVal = m.Value
				startCheck = true

			} else {
				if m.Value == timerange.Value {
					timerange.End = m.Timestamp
				} else {
					if timerange.End.IsZero() {
						timerange.End = timerange.Start
					}
					if timerange.End.Minute() == 0 {
						timerange.End = timerange.End.Add(time.Minute * time.Duration(59))
					}
					result = append(result, timerange)
					timerange.Start = m.Timestamp
					timerange.Value = m.Value
				}

			}
		}
		if timerange.End.IsZero() {
			timerange.End = timerange.Start
		}
		if timerange.End.Minute() == 0 {
			timerange.End = timerange.End.Add(time.Minute * time.Duration(59))
		}
		result = append(result, timerange)
	}
	return result
}
