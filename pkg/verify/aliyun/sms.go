package aliyun

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// SMSClient calls Dysmsapi SendSms.
type SMSClient struct {
	AccessKeyID     string
	AccessKeySecret string
	Region          string
	SignName        string
	TemplateCode    string
	Endpoint        string
	HTTP            *http.Client
}

// smsTemplateParam is a struct, not a number, so encoding/json quotes the
// code. {"code":012345} is illegal JSON and a numeric code drops a leading zero.
type smsTemplateParam struct {
	Code string `json:"code"`
}

func (c *SMSClient) SendCode(ctx context.Context, phone, code string) error {
	param, err := json.Marshal(smsTemplateParam{Code: code})
	if err != nil {
		return err
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = "https://dysmsapi.aliyuncs.com/"
	}
	region := c.Region
	if region == "" {
		region = "cn-hangzhou"
	}
	return postRPC(ctx, c.HTTP, endpoint, c.AccessKeyID, c.AccessKeySecret, map[string]string{
		"Action":        "SendSms",
		"Version":       "2017-05-25",
		"RegionId":      region,
		"PhoneNumbers":  phone,
		"SignName":      c.SignName,
		"TemplateCode":  c.TemplateCode,
		"TemplateParam": string(param),
	})
}

// EndpointHost is the host of the configured endpoint, used by tests.
func (c *SMSClient) EndpointHost() string {
	return strings.TrimRight(c.Endpoint, "/")
}
