package enforcement

import (
	"fmt"
	"strings"
	"time"
)

var beijing = time.FixedZone("UTC+8", 8*3600)

func bj(t time.Time) string {
	return t.In(beijing).Format("2006-01-02 15:04")
}

func render(template string, row *Sanction, now time.Time) string {
	end := "未设定结束时间"
	if row.End != nil {
		end = bj(*row.End)
	}
	scope := strings.Join(row.Scopes, "、")
	if scope == "" {
		scope = row.Kind
	}
	start := bj(row.Start)
	if row.Start.IsZero() {
		start = bj(now)
	}
	var body string
	switch template {
	case "T-01":
		body = fmt.Sprintf("你的账号因%s，依据《%s》，自 %s 起至 %s（北京时间）被限制以下功能：%s。其他功能不受影响，你可以继续登录使用。处置编号：%s。如有异议，可在处置页申诉。", row.ReasonCategory, row.PolicyClause, start, end, scope, row.ID)
	case "T-02":
		body = fmt.Sprintf("你的账号因%s，依据《%s》，自 %s 起至 %s（北京时间）处于只读冻结状态：可以登录和查看消息，暂不能发送消息、发布内容或修改资料。到期后将自动恢复。处置编号：%s。申诉入口：处置页。", row.ReasonCategory, row.PolicyClause, start, end, row.ID)
	case "T-03":
		body = fmt.Sprintf("你的账号因%s，依据《%s》，自 %s（北京时间）起被停用，已无法使用聊天及其他服务。你仍可通过手机号或邮箱完成身份验证后查看处置说明并提交申诉。处置编号：%s。申诉入口：处置页。", row.ReasonCategory, row.PolicyClause, start, row.ID)
	case "T-04":
		body = fmt.Sprintf("你的账号处置（编号 %s）已于 %s（北京时间）到期，相关功能已恢复。请遵守《%s》。", row.ID, end, row.PolicyClause)
	case "T-05":
		body = fmt.Sprintf("经复核，你的账号处置（编号 %s）已撤销。相关功能已恢复，你的聊天数据未受影响。若此前已退出登录，请重新登录。给你带来不便，我们深表歉意。", row.ID)
	case "T-06":
		body = fmt.Sprintf("我们已收到你针对处置编号 %s 的申诉，正在处理。如需补充材料，我们会通过本通知和处置页联系你。", row.ID)
	case "T-07":
		body = fmt.Sprintf("你针对处置编号 %s 的申诉已处理完毕。请在处置页查看该项结果。", row.ID)
	default:
		body = "处置编号：" + row.ID
	}
	return body + "\nTellomi 不会通过任何渠道向你索要验证码或密码。请勿将验证码告知任何人。"
}
