package accountdeletion

// pageHTML is the deletion form. Black and white. It does not mention
// encryption and it does not offer another way to sign in.
var pageHTML = []byte(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>删除 Tellomi 账号</title>
<style>
  body { margin: 0; background: #fff; color: #111; font: 16px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
  main { max-width: 28rem; margin: 0 auto; padding: 2.5rem 1.25rem 4rem; }
  h1 { font-size: 1.5rem; font-weight: 650; margin: 0 0 0.75rem; }
  p { margin: 0 0 1rem; }
  label { display: block; margin: 1rem 0 0.35rem; }
  input { width: 100%; box-sizing: border-box; border: 1px solid #111; background: #fff; color: #111; padding: 0.6rem 0.7rem; font: inherit; }
  button { margin-top: 1.25rem; background: #111; color: #fff; border: 0; padding: 0.7rem 1.1rem; font: inherit; cursor: pointer; }
  button:disabled { background: #888; cursor: default; }
  .err { color: #111; border: 1px solid #111; padding: 0.6rem 0.7rem; margin: 1rem 0; }
  a { color: #111; }
</style>
</head>
<body>
<main>
  <h1>删除 Tellomi 账号</h1>
  <p>输入注册时使用的手机号。我们会发送短信验证码。如果账号开了两步验证，还需要填写密码。确认后，删除方式与 App 内「删除账号」相同。</p>
  <p>也可以在 App 内删除，或发邮件到 <a href="mailto:privacy@tellomi.app">privacy@tellomi.app</a>。</p>
  <form id="f">
    <label for="phone">手机号</label>
    <input id="phone" name="phone" inputmode="tel" autocomplete="tel" placeholder="+86" required>
    <label for="code">短信验证码</label>
    <input id="code" name="code" inputmode="numeric" autocomplete="one-time-code">
    <div id="pw" hidden>
      <label for="password">两步验证密码</label>
      <input id="password" name="password" type="password" autocomplete="current-password">
      <p id="hint"></p>
    </div>
    <label for="confirm">输入 DELETE 以确认删除</label>
    <input id="confirm" name="confirm" autocomplete="off">
    <button id="go" type="submit">继续</button>
  </form>
  <p id="msg" class="err" hidden></p>
</main>
<script>
(function () {
  var form = document.getElementById("f");
  var msg = document.getElementById("msg");
  var btn = document.getElementById("go");
  var session = "";
  function show(text) { msg.hidden = !text; msg.textContent = text || ""; }
  function post(url, body) {
    return fetch(url, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) })
      .then(function (res) { return res.json().then(function (data) { return { status: res.status, data: data }; }); });
  }
  form.addEventListener("submit", function (ev) {
    ev.preventDefault();
    show("");
    btn.disabled = true;
    var phone = document.getElementById("phone").value;
    var code = document.getElementById("code").value;
    var password = document.getElementById("password").value;
    var confirm = document.getElementById("confirm").value;
    var next = session ? Promise.resolve() : post("/v1/verification/session", { phone: phone }).then(function (res) {
      if (res.data.error) { throw new Error(res.data.error); }
      session = res.data.session_id;
    });
    next.then(function () {
      return post("/v1/tellomi/account-deletion", { session_id: session, code: code, password: password, confirm: confirm });
    }).then(function (res) {
      if (res.data.error === "password_required") {
        document.getElementById("pw").hidden = false;
        if (res.data.hint) { document.getElementById("hint").textContent = "提示：" + res.data.hint; }
        show("请填写两步验证密码，并输入 DELETE。");
        return;
      }
      if (res.data.deleted) {
        form.hidden = true;
        show("账号已删除。");
        return;
      }
      show(res.data.error || "没有完成");
    }).catch(function () {
      show("请求失败，请稍后再试。");
    }).then(function () { btn.disabled = false; });
  });
})();
</script>
</body>
</html>
`)
