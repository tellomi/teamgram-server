package aliyun

import (
	"context"
	"fmt"
	"net/http"
)

// DirectMailClient calls DirectMail SingleSendMail on dm.aliyuncs.com.
// Region is cn-hangzhou, which is where the DirectMail API is served.
type DirectMailClient struct {
	AccessKeyID     string
	AccessKeySecret string
	AccountName     string
	FromAlias       string
	Endpoint        string
	HTTP            *http.Client
}

func (c *DirectMailClient) SendCode(ctx context.Context, to, code string) error {
	if c.AccountName == "" {
		return fmt.Errorf("directmail account name is not configured")
	}
	endpoint := c.Endpoint
	if endpoint == "" {
		endpoint = "https://dm.aliyuncs.com/"
	}
	alias := c.FromAlias
	if alias == "" {
		alias = "Tellomi"
	}
	text := fmt.Sprintf("您的验证码为：%s，请勿泄露于他人！", code)
	return postRPC(ctx, c.HTTP, endpoint, c.AccessKeyID, c.AccessKeySecret, map[string]string{
		"Action":         "SingleSendMail",
		"Version":        "2015-11-23",
		"RegionId":       "cn-hangzhou",
		"AccountName":    c.AccountName,
		"AddressType":    "1",
		"ReplyToAddress": "false",
		"ToAddress":      to,
		"FromAlias":      alias,
		"Subject":        "Tellomi 验证码",
		"TextBody":       text,
	})
}
