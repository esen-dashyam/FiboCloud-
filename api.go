package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
)

// SmsRequest ...
type SmsRequest struct {
	MobileNum string `json:"mobile_num"`
	SmsTxt    string `json:"sms_txt"`
}

// SMSResponse ...
type SMSResponse struct {
	StatusCode int         `json:"status_code"`
	ErrorMsg   string      `json:"error_msg"`
	Body       interface{} `json:"body"`
}

// SendSMS ...
func SendSMS(mobileNum, smsTxt string) (SMSResponse, error) {
	var request SmsRequest
	var response SMSResponse
	url := os.Getenv("sms.url") + "send"
	fmt.Print(url)
	method := "POST"

	request.MobileNum = mobileNum
	request.SmsTxt = smsTxt

	// payload := strings.NewReader("{\n	\"mobile_num\" : \""+mobileNum+"\",\n	\"sms_txt\" : \""+smsTxt+"\"\n}")

	requestByte, _ := json.Marshal(request)
	requestReader := bytes.NewReader(requestByte)

	client := &http.Client{}
	req, err := http.NewRequest(method, url, requestReader)

	if err != nil {
		fmt.Println(err)
	}

	req.Header.Add("Content-Type", "application/json")

	res, resErr := client.Do(req)

	if res.StatusCode != 200 {
		return response, errors.New(res.Status)
	}

	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	json.Unmarshal(body, &response)

	if response.StatusCode != int(0) {
		return response, errors.New(response.ErrorMsg)
	}

	return response, resErr
}
