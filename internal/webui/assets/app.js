"use strict";

/** 管理正文编辑、图片预览和请求状态。 */
(function () {
  var token = window.location.hash.slice(1);
  var text = document.getElementById("text");
  var size = document.getElementById("size");
  var level = document.getElementById("level");
  var image = document.getElementById("image");
  var status = document.getElementById("status");
  var file = document.getElementById("file");
  var save = document.getElementById("save");
  var generated = "";
  var previewURL = "";
  var busy = false;
  var exited = false;
  var decodedOriginal = null;

  /** textarea 会将 CRLF 规范为 LF，未编辑的识别结果使用原文。
   * @returns {string} 用于编码和字节计数的正文。
   */
  function currentText() {
    return decodedOriginal !== null ? decodedOriginal : text.value;
  }

  /** 以纯文本显示状态。
   * @param {string} value 状态或错误信息。
   * @param {boolean} error true 使用错误样式；false 使用普通状态样式。
   */
  function message(value, error) {
    status.textContent = value;
    status.className = error ? "error" : "";
  }

  /** 更新 UTF-8 字节计数，并禁用过期图片的保存按钮。 */
  function invalidate() {
    generated = "";
    save.disabled = true;
    document.getElementById("count").textContent = new Blob([currentText()]).size + " 个 UTF-8 字节";
    document.getElementById("detail").textContent = "请重新生成后保存图片。";
  }

  /** 请求期间锁定控件，使响应与提交的正文和参数对应。
   * @param {boolean} value true 锁定操作；false 按退出状态和生成结果恢复控件。
   */
  function setBusy(value) {
    busy = value;
    document.querySelectorAll("button, input, select").forEach(function (element) {
      element.disabled = value || exited;
    });
    text.readOnly = value || exited;
    save.disabled = value || exited || !generated;
  }

  /** 向本机服务发送带访问凭据的 POST 请求。
   * @param {string} path API 路径。
   * @param {string|File} body JSON 文本、图片文件或空正文。
   * @param {string} contentType 请求正文的媒体类型。
   * @returns {Promise<Object>} API 的 JSON 结果；网络、HTTP 或响应解析失败时拒绝。
   */
  function request(path, body, contentType) {
    return fetch(path, {
      method: "POST",
      headers: { "X-QR-Token": token, "Content-Type": contentType },
      body: body
    }).then(function (response) {
      return response.text().then(function (raw) {
        var data;
        try { data = JSON.parse(raw); } catch (error) { throw new Error(raw || "本地服务返回了无效响应"); }
        if (!response.ok) { throw new Error(data.error || "请求失败"); }
        return data;
      });
    });
  }

  /** 替换预览并释放上一张上传图片的 Blob URL。
   * @param {string} source PNG 数据地址或上传图片的 Blob URL。
   */
  function showImage(source) {
    if (previewURL) { URL.revokeObjectURL(previewURL); previewURL = ""; }
    image.src = source;
    image.hidden = false;
    document.getElementById("empty").hidden = true;
  }

  text.addEventListener("input", function () { decodedOriginal = null; invalidate(); });
  size.addEventListener("input", invalidate);
  level.addEventListener("change", invalidate);

  document.getElementById("generate").addEventListener("click", function () {
    if (busy || exited) { return; }
    invalidate();
    if (!currentText() || !size.checkValidity()) { message("请填写正文和 1—4096 之间的整数尺寸。", true); return; }
    setBusy(true);
    message("正在生成…", false);
    request("/api/encode", JSON.stringify({ text: currentText(), size: Number(size.value), level: level.value }), "application/json")
      .then(function (data) {
        generated = "data:image/png;base64," + data.png;
        showImage(generated);
        var scale = Math.floor(Number(size.value) / data.modules);
        document.getElementById("detail").textContent = "Version " + data.version + " · " + size.value + " × " + size.value + " 像素 · 每模块 " + scale + " 像素";
        message(scale < 4 ? "已生成。二维码较密集，建议增大图片尺寸。" : "已生成，可保存 PNG。", false);
      })
      .catch(function (error) { message(error.message, true); })
      .then(function () { setBusy(false); });
  });

  document.getElementById("upload").addEventListener("click", function () { file.click(); });
  file.addEventListener("change", function () {
    var selected = file.files[0];
    if (!selected || busy || exited) { return; }
    if (selected.size > 16 * 1024 * 1024) { message("图片文件不能超过 16 MiB。", true); file.value = ""; return; }
    invalidate();
    setBusy(true);
    message("正在识别…", false);
    request("/api/decode", selected, "application/octet-stream")
      .then(function (data) {
        text.value = data.text;
        decodedOriginal = data.text;
        invalidate();
        var source = URL.createObjectURL(selected);
        showImage(source);
        previewURL = source;
        document.getElementById("detail").textContent = "已打开图片。点击生成可保存二维码 PNG。";
        message("识别完成，正文已显示在内容区域。", false);
      })
      .catch(function (error) { message(error.message, true); })
      .then(function () { file.value = ""; setBusy(false); });
  });

  save.addEventListener("click", function () {
    if (!generated || busy || exited) { return; }
    var link = document.createElement("a");
    link.href = generated;
    link.download = "qrplain.png";
    document.body.appendChild(link);
    link.click();
    link.remove();
  });

  document.getElementById("quit").addEventListener("click", function () {
    if (busy || exited) { return; }
    setBusy(true);
    request("/api/quit", "", "text/plain")
      .then(function (data) { exited = true; message(data.message, false); })
      .catch(function (error) { message("退出请求失败：" + error.message, true); })
      .then(function () { setBusy(false); });
  });

  window.addEventListener("beforeunload", function () { if (previewURL) { URL.revokeObjectURL(previewURL); } });
  if (!/^[a-f0-9]{64}$/.test(token)) {
    exited = true;
    setBusy(false);
    message("访问凭据缺失，请用程序显示的完整地址打开页面。", true);
  }
}());
