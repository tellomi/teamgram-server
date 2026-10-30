package aliyun

import (
	"context"
	"encoding/json"
	"fmt"
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
	if c.AccessKeyID == "" || c.AccessKeySecret == "" {
		return fmt.Errorf("aliyun access key is not configured")
	}
	// Defaults in verify.Load fill these. An explicit empty value is incomplete
	// SMS config and must fail before any dial, not wait on Dysmsapi.
	if strings.TrimSpace(c.SignName) == "" || strings.TrimSpace(c.TemplateCode) == "" {
		return fmt.Errorf("aliyun sms sign or template is not configured")
	}
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
