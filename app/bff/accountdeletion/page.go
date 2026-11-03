package accountdeletion

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
)

// The page is black and white. The confirm word is the only red text.
// Style and script are inlined so the CSP can pin them with sha256 hashes.
// There is no external asset.

const pageStyle = `
  html { color-scheme: light; }
  body { margin: 0; background: #fff; color: #111; font: 16px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
  main { max-width: 28rem; margin: 0 auto; padding: 2.5rem 1.25rem 4rem; }
  .brand { margin: 0 0 1.25rem; font-weight: 700; letter-spacing: 0.08em; }
  h1 { font-size: 1.5rem; font-weight: 650; margin: 0 0 0.75rem; }
  p { margin: 0 0 1rem; }
  label { display: block; margin: 1rem 0 0.35rem; }
  input { width: 100%; box-sizing: border-box; border: 1px solid #111; background: #fff; color: #111; padding: 0.6rem 0.7rem; font: inherit; }
  button { margin-top: 1.25rem; background: #111; color: #fff; border: 1px solid #111; padding: 0.7rem 1.1rem; font: inherit; cursor: pointer; }
  button:disabled { background: #888; border-color: #888; color: #fff; cursor: default; }
  button:focus, input:focus, button:focus-visible, input:focus-visible { outline: 2px solid #111; outline-offset: 1px; }
  input, button { accent-color: #111; }
  .lang, .tabs { display: flex; gap: 0; }
  .lang { margin: 0 0 1.25rem; }
  .lang button, .tabs button { flex: 1; margin: 0; background: #fff; color: #111; }
  .lang button + button, .tabs button + button { border-left: 0; }
  .lang button.on, .tabs button.on { background: #111; color: #fff; }
  .tabs { margin-top: 0.25rem; }
  .err { color: #111; border: 1px solid #111; padding: 0.6rem 0.7rem; margin: 1rem 0; }
  a { color: #111; }
  .word { color: #c00; font-weight: 700; }
`

const pageScript = `
(function () {
  var lang = "zh";
  var mode = "phone";
  var session = "";
  var copy = {
    zh: {
      title: "删除 Tellomi 账号",
      intro: "选择手机号或邮箱。我们会发送验证码。如果账号开了两步验证，还需要填写密码。确认后，删除方式与 App 内删除账号相同。",
      alt: "也可以在 App 内删除，或发邮件到",
      altEnd: "。",
      tabPhone: "手机号",
      tabEmail: "邮箱",
      phone: "手机号",
      email: "邮箱",
      phonePh: "+86",
      emailPh: "name@example.com",
      send: "发送验证码",
      codePhone: "短信验证码",
      codeEmail: "邮箱验证码",
      password: "两步验证密码",
      hint: "提示：",
      submit: "删除账号",
      done: "账号已删除。",
      fail: "请求失败，请稍后再试。",
      needCode: "请先发送验证码。",
      needConfirm: "请准确输入删除。",
      needPassword: "请填写两步验证密码，并输入删除。",
      confirm: ["输入 ", "删除", " 以确认删除"],
      word: "删除",
      rate: "发送太频繁，请稍后再试。",
      badPhone: "请输入有效的手机号。",
      badEmail: "请输入有效的邮箱。",
      badCode: "验证码不正确。",
      badPassword: "密码不正确。",
      unfinished: "没有完成。"
    },
    en: {
      title: "Delete your Tellomi account",
      intro: "Choose phone or email. We send a verification code. If the account has two-step verification, the password is required too. After you confirm, the account is deleted the same way as in the app.",
      alt: "You can also delete the account in the app, or email",
      altEnd: ".",
      tabPhone: "Phone",
      tabEmail: "Email",
      phone: "Phone number",
      email: "Email",
      phonePh: "+86",
      emailPh: "name@example.com",
      send: "Send code",
      codePhone: "SMS code",
      codeEmail: "Email code",
      password: "Two-step verification password",
      hint: "Hint: ",
      submit: "Delete account",
      done: "The account has been deleted.",
      fail: "The request failed. Try again later.",
      needCode: "Send a code first.",
      needConfirm: "Type DELETE exactly.",
      needPassword: "Enter the two-step verification password, and type DELETE.",
      confirm: ["Type ", "DELETE", " to confirm"],
      word: "DELETE",
      rate: "Too many attempts. Try again later.",
      badPhone: "Enter a valid phone number.",
      badEmail: "Enter a valid email address.",
      badCode: "The code is not valid.",
      badPassword: "The password is not valid.",
      unfinished: "Could not finish."
    }
  };

  function t() { return copy[lang]; }
  function expected() { return t().word; }

  function el(id) { return document.getElementById(id); }

  function renderConfirm() {
    var label = el("confirmLabel");
    var parts = t().confirm;
    label.textContent = "";
    label.appendChild(document.createTextNode(parts[0]));
    var word = document.createElement("span");
    word.className = "word";
    word.textContent = parts[1];
    label.appendChild(word);
    label.appendChild(document.createTextNode(parts[2]));
  }

  function syncConfirm() {
    el("go").disabled = el("confirm").value !== expected();
  }

  function render() {
    var c = t();
    document.documentElement.lang = lang === "en" ? "en" : "zh-CN";
    document.title = c.title;
    el("title").textContent = c.title;
    el("intro").textContent = c.intro;
    el("alt").textContent = c.alt;
    el("altEnd").textContent = c.altEnd;
    el("tabPhone").textContent = c.tabPhone;
    el("tabEmail").textContent = c.tabEmail;
    el("zh").className = lang === "zh" ? "on" : "";
    el("en").className = lang === "en" ? "on" : "";
    el("tabPhone").className = mode === "phone" ? "on" : "";
    el("tabEmail").className = mode === "email" ? "on" : "";
    el("tabPhone").setAttribute("aria-selected", mode === "phone" ? "true" : "false");
    el("tabEmail").setAttribute("aria-selected", mode === "email" ? "true" : "false");
    if (mode === "email") {
      el("idLabel").textContent = c.email;
      el("ident").placeholder = c.emailPh;
      el("ident").inputMode = "email";
      el("ident").autocomplete = "email";
    } else {
      el("idLabel").textContent = c.phone;
      el("ident").placeholder = c.phonePh;
      el("ident").inputMode = "tel";
      el("ident").autocomplete = "tel";
    }
    el("send").textContent = c.send;
    el("codeLabel").textContent = mode === "email" ? c.codeEmail : c.codePhone;
    el("pwLabel").textContent = c.password;
    el("go").textContent = c.submit;
    renderConfirm();
    syncConfirm();
  }

  function show(text) {
    var msg = el("msg");
    msg.hidden = !text;
    msg.textContent = text || "";
    if (text) { msg.scrollIntoView({ block: "nearest" }); }
  }

  function mapErr(code) {
    var c = t();
    if (code === "rate_limited") { return c.rate; }
    if (code === "phone_invalid") { return c.badPhone; }
    if (code === "email_invalid") { return c.badEmail; }
    if (code === "code_invalid") { return c.badCode; }
    if (code === "password_invalid") { return c.badPassword; }
    if (code === "confirm_required") { return c.needConfirm; }
    return c.unfinished;
  }

  function post(url, body) {
    return fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body)
    }).then(function (res) {
      return res.json().then(function (data) {
        return { status: res.status, data: data };
      });
    });
  }

  function resetSession() {
    session = "";
    el("step2").hidden = true;
    el("pw").hidden = true;
    el("hint").textContent = "";
    el("code").value = "";
  }

  function setLang(next) {
    lang = next;
    render();
  }

  function setMode(next) {
    if (mode === next) { return; }
    mode = next;
    el("ident").value = "";
    el("confirm").value = "";
    el("password").value = "";
    resetSession();
    render();
  }

  el("zh").addEventListener("click", function () { setLang("zh"); });
  el("en").addEventListener("click", function () { setLang("en"); });
  el("tabPhone").addEventListener("click", function () { setMode("phone"); });
  el("tabEmail").addEventListener("click", function () { setMode("email"); });
  el("ident").addEventListener("input", resetSession);
  el("confirm").addEventListener("input", syncConfirm);
  el("ident").addEventListener("keydown", function (ev) {
    if (ev.key === "Enter") {
      ev.preventDefault();
      el("send").click();
    }
  });

  el("send").addEventListener("click", function () {
    show("");
    var btn = el("send");
    btn.disabled = true;
    var ident = el("ident").value;
    var payload = mode === "email" ? { email: ident } : { phone: ident };
    post("/v1/verification/session", payload).then(function (res) {
      if (!res.data || res.data.error) {
        show(mapErr(res.data && res.data.error));
        return;
      }
      session = res.data.session_id;
      el("step2").hidden = false;
      el("confirmLabel").scrollIntoView({ block: "nearest" });
    }).catch(function () {
      show(t().fail);
    }).then(function () { btn.disabled = false; });
  });

  el("f").addEventListener("submit", function (ev) {
    ev.preventDefault();
    show("");
    if (!session) {
      show(t().needCode);
      return;
    }
    var typed = el("confirm").value;
    if (typed !== expected()) {
      show(t().needConfirm);
      syncConfirm();
      return;
    }
    var btn = el("go");
    btn.disabled = true;
    post("/v1/tellomi/account-deletion", {
      session_id: session,
      code: el("code").value,
      password: el("password").value,
      confirm: typed
    }).then(function (res) {
      if (res.data && res.data.error === "password_required") {
        el("pw").hidden = false;
        el("hint").textContent = res.data.hint ? t().hint + res.data.hint : "";
        show(t().needPassword);
        return;
      }
      if (res.data && res.data.deleted) {
        el("f").hidden = true;
        show(t().done);
        return;
      }
      show(mapErr(res.data && res.data.error));
    }).catch(function () {
      show(t().fail);
    }).then(function () { syncConfirm(); });
  });

  render();
})();
`

var (
	pageHTML []byte
	pageCSP  string
)

func init() {
	pageHTML = renderPage(pageStyle, pageScript)
	pageCSP = contentSecurityPolicy(pageStyle, pageScript)
}

func renderPage(style, script string) []byte {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"zh-CN\">\n<head>\n<meta charset=\"utf-8\">\n")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	b.WriteString("<title>删除 Tellomi 账号</title>\n<style>")
	b.WriteString(style)
	b.WriteString("</style>\n</head>\n<body>\n<main>\n")
	b.WriteString("<p class=\"brand\">Tellomi</p>\n")
	b.WriteString("<div class=\"lang\">\n")
	b.WriteString("<button type=\"button\" id=\"zh\" class=\"on\">中文</button>\n")
	b.WriteString("<button type=\"button\" id=\"en\">English</button>\n")
	b.WriteString("</div>\n")
	b.WriteString("<h1 id=\"title\">删除 Tellomi 账号</h1>\n")
	b.WriteString("<p id=\"intro\">选择手机号或邮箱。我们会发送验证码。如果账号开了两步验证，还需要填写密码。确认后，删除方式与 App 内删除账号相同。</p>\n")
	b.WriteString("<p><span id=\"alt\">也可以在 App 内删除，或发邮件到</span> <a href=\"mailto:privacy@tellomi.app\">privacy@tellomi.app</a><span id=\"altEnd\">。</span></p>\n")
	b.WriteString("<form id=\"f\">\n")
	b.WriteString("<div class=\"tabs\" role=\"tablist\">\n")
	b.WriteString("<button type=\"button\" id=\"tabPhone\" class=\"on\" role=\"tab\" aria-selected=\"true\">手机号</button>\n")
	b.WriteString("<button type=\"button\" id=\"tabEmail\" role=\"tab\" aria-selected=\"false\">邮箱</button>\n")
	b.WriteString("</div>\n")
	b.WriteString("<label id=\"idLabel\" for=\"ident\">手机号</label>\n")
	b.WriteString("<input id=\"ident\" name=\"ident\" inputmode=\"tel\" autocomplete=\"tel\" placeholder=\"+86\">\n")
	b.WriteString("<button id=\"send\" type=\"button\">发送验证码</button>\n")
	b.WriteString("<div id=\"step2\" hidden>\n")
	b.WriteString("<label id=\"codeLabel\" for=\"code\">短信验证码</label>\n")
	b.WriteString("<input id=\"code\" name=\"code\" inputmode=\"numeric\" autocomplete=\"one-time-code\">\n")
	b.WriteString("<div id=\"pw\" hidden>\n")
	b.WriteString("<label id=\"pwLabel\" for=\"password\">两步验证密码</label>\n")
	b.WriteString("<input id=\"password\" name=\"password\" type=\"password\" autocomplete=\"current-password\">\n")
	b.WriteString("<p id=\"hint\"></p>\n")
	b.WriteString("</div>\n")
	b.WriteString("<label id=\"confirmLabel\" for=\"confirm\">输入 <span class=\"word\">删除</span> 以确认删除</label>\n")
	b.WriteString("<input id=\"confirm\" name=\"confirm\" autocomplete=\"off\" spellcheck=\"false\">\n")
	b.WriteString("<button id=\"go\" type=\"submit\" disabled>删除账号</button>\n")
	b.WriteString("</div>\n</form>\n")
	b.WriteString("<p id=\"msg\" class=\"err\" hidden></p>\n")
	b.WriteString("</main>\n<script>")
	b.WriteString(script)
	b.WriteString("</script>\n</body>\n</html>\n")
	return []byte(b.String())
}

func contentSecurityPolicy(style, script string) string {
	return strings.Join([]string{
		"default-src 'none'",
		"base-uri 'none'",
		"form-action 'self'",
		"frame-ancestors 'none'",
		"object-src 'none'",
		"img-src 'none'",
		"font-src 'none'",
		"connect-src 'self'",
		"script-src " + cspHash(script),
		"style-src " + cspHash(style),
	}, "; ")
}

func cspHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}
