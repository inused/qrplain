/** 使用 Chromium 和 Node.js 的 CDP 接口验证浏览器交互。 */
import assert from "node:assert/strict";
import { spawn, execFileSync } from "node:child_process";
import { mkdir, mkdtemp, readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const cacheRoot = path.join(root, ".cache", "browser-test");
const browserPath = process.env.QR_TEST_BROWSER;
if (!browserPath) {
  throw new Error("请通过 QR_TEST_BROWSER 指定 Chromium/Chrome 可执行文件。");
}
await mkdir(cacheRoot, { recursive: true });
// 独立目录可避免读取上次运行的下载文件。
const cache = await mkdtemp(path.join(cacheRoot, "run-"));

/** 等待异步检查返回真值，超时则失败。
 * @param {Function} check 返回期望值或假值的异步检查。
 * @returns {Promise<*>} 首个真值；10 秒内未完成则拒绝。
 */
async function until(check) {
  const deadline = Date.now() + 10000;
  while (Date.now() < deadline) {
    const result = await check();
    if (result) return result;
    await new Promise((resolve) => setTimeout(resolve, 50));
  }
  throw new Error("等待浏览器操作超时");
}

const app = spawn(path.join(root, "dist", "qrplain-linux-amd64"), ["ui"], {
  cwd: root,
  // PATH 为空，用于验证浏览器启动器不可用时的手动访问路径。
  env: { ...process.env, PATH: "" },
  stdio: ["ignore", "ignore", "pipe"],
});
let appLog = "";
app.stderr.on("data", (data) => { appLog += data; });
let browser;
let socket;
try {
  const address = await until(() => appLog.match(/http:\/\/127\.0\.0\.1:\d+\/#[a-f0-9]{64}/)?.[0]);
  browser = spawn(browserPath, [
    "--headless", "--no-sandbox", "--disable-dev-shm-usage", "--no-first-run",
    "--no-default-browser-check", "--remote-debugging-port=0",
    "--user-data-dir=" + path.join(cache, "profile"), "about:blank",
  ], { stdio: ["ignore", "ignore", "pipe"] });
  let browserLog = "";
  browser.stderr.on("data", (data) => { browserLog += data; });
  const endpoint = await until(() => browserLog.match(/DevTools listening on (ws:\/\/[^\s]+)/)?.[1]);
  socket = new WebSocket(endpoint);
  await new Promise((resolve, reject) => { socket.onopen = resolve; socket.onerror = reject; });
  let nextID = 0;
  let pageSession;
  const pending = new Map();
  const exceptions = [];
  socket.addEventListener("message", (event) => {
    const value = JSON.parse(event.data);
    if (value.method === "Runtime.exceptionThrown") exceptions.push(value.params);
    if (!value.id) return;
    const waiter = pending.get(value.id);
    if (!waiter) return;
    pending.delete(value.id);
    clearTimeout(waiter.timer);
    if (value.error) waiter.reject(new Error(JSON.stringify(value.error)));
    else waiter.resolve(value.result);
  });

  /** 调用公开 CDP 方法；浏览器根方法传 null，页面方法默认使用当前页面会话。
   * @param {string} method CDP 方法名。
   * @param {object} params 对应协议参数。
   * @param {string|null} session 页面会话标识；null 表示浏览器级调用。
   * @returns {Promise<object>} 协议结果；错误或超时则拒绝。
   */
  function call(method, params = {}, session = pageSession) {
    const id = ++nextID;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => { pending.delete(id); reject(new Error(method + " 超时")); }, 10000);
      pending.set(id, { resolve, reject, timer });
      socket.send(JSON.stringify({ id, method, params, ...(session ? { sessionId: session } : {}) }));
    });
  }

  /** 在页面执行验证表达式；页面异常直接使测试失败。
   * @param {string} expression JavaScript 表达式，允许返回 Promise。
   * @returns {Promise<*>} 页面序列化结果。
   */
  async function evaluate(expression) {
    const response = await call("Runtime.evaluate", { expression, returnByValue: true, awaitPromise: true });
    if (response.exceptionDetails) throw new Error(JSON.stringify(response.exceptionDetails));
    return response.result.value;
  }

  const target = await call("Target.createTarget", { url: "about:blank" }, null);
  const attached = await call("Target.attachToTarget", { targetId: target.targetId, flatten: true }, null);
  pageSession = attached.sessionId;
  await call("Page.enable");
  await call("Runtime.enable");
  await call("Emulation.setDeviceMetricsOverride", { width: 1200, height: 1000, deviceScaleFactor: 1, mobile: false });
  await call("Browser.setDownloadBehavior", { behavior: "allow", downloadPath: cache }, null);
  await call("Page.navigate", { url: address });
  await until(() => evaluate('document.getElementById("status") && document.getElementById("status").textContent === "就绪"'));
  assert.equal(await evaluate('document.title'), "QRPlain · 本地二维码");
  const content = " 浏览器中文🙂\n\n<script>不能执行</script> ";
  await evaluate(`document.getElementById("text").value = ${JSON.stringify(content)}; document.getElementById("text").dispatchEvent(new Event("input")); document.getElementById("generate").click()`);
  await until(() => evaluate('!document.getElementById("save").disabled'));
  assert.equal(await evaluate('document.getElementById("image").naturalWidth'), 512);
  assert.match(await evaluate('document.getElementById("status").textContent'), /已生成/);

  const source = await evaluate('document.getElementById("image").src');
  const originalPNG = Buffer.from(source.split(",")[1], "base64");
  const fixture = path.join(cache, "upload.png");
  await writeFile(fixture, originalPNG);
  await evaluate('document.getElementById("save").click()');
  await until(async () => { try { return (await readFile(path.join(cache, "qrplain.png"))).equals(originalPNG); } catch { return false; } });

  await evaluate('document.getElementById("text").value = "已修改"; document.getElementById("text").dispatchEvent(new Event("input"))');
  assert.equal(await evaluate('document.getElementById("save").disabled'), true);

  const dom = await call("DOM.getDocument");
  const input = await call("DOM.querySelector", { nodeId: dom.root.nodeId, selector: "#file" });
  await call("DOM.setFileInputFiles", { nodeId: input.nodeId, files: [fixture] });
  await until(() => evaluate('document.getElementById("status").textContent.indexOf("识别完成") === 0'));
  assert.equal(await evaluate('document.getElementById("text").value'), content);
  assert.equal(await evaluate('document.getElementById("save").disabled'), true);
  assert.equal(await evaluate('document.querySelectorAll("textarea script").length'), 0);

  await evaluate('document.getElementById("generate").click()');
  await until(() => evaluate('!document.getElementById("save").disabled'));

  // textarea 会将 CRLF 规范为 LF；重新生成未编辑的识别结果时使用原文。
  const crlfText = "中文\r\n\r\n末尾\r\n";
  const crlfPNG = await evaluate(`fetch("/api/encode", {method:"POST",headers:{"X-QR-Token":location.hash.slice(1),"Content-Type":"application/json"},body:JSON.stringify({text:${JSON.stringify(crlfText)},size:512,level:"M"})}).then(function(r){return r.json()}).then(function(r){return r.png})`);
  const crlfFixture = path.join(cache, "crlf.png");
  await writeFile(crlfFixture, Buffer.from(crlfPNG, "base64"));
  await call("DOM.setFileInputFiles", { nodeId: input.nodeId, files: [crlfFixture] });
  await until(() => evaluate('document.getElementById("status").textContent.indexOf("识别完成") === 0'));
  await evaluate('document.getElementById("generate").click()');
  await until(() => evaluate('!document.getElementById("save").disabled'));
  const preservedSource = await evaluate('document.getElementById("image").src');
  const preservedFile = path.join(cache, "preserved.png");
  await writeFile(preservedFile, Buffer.from(preservedSource.split(",")[1], "base64"));
  assert.equal(execFileSync(path.join(root, "dist", "qrplain-linux-amd64"), ["decode", "--image", preservedFile], { encoding: "utf8" }), crlfText + "\n");

  await evaluate('document.getElementById("size").value = "768"; document.getElementById("size").dispatchEvent(new Event("input"))');
  assert.equal(await evaluate('document.getElementById("save").disabled'), true);
  await evaluate('document.getElementById("generate").click()');
  await until(() => evaluate('!document.getElementById("save").disabled'));
  assert.equal(await evaluate('document.getElementById("image").naturalWidth'), 768);

  const screenshot = await call("Page.captureScreenshot", { format: "png" });
  await writeFile(path.join(cache, "desktop.png"), Buffer.from(screenshot.data, "base64"));
  await call("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 1, mobile: false });
  assert.equal(await evaluate('document.documentElement.scrollWidth <= window.innerWidth'), true);

  await evaluate('document.getElementById("size").value = "0"; document.getElementById("size").dispatchEvent(new Event("input")); document.getElementById("generate").click()');
  assert.match(await evaluate('document.getElementById("status").textContent'), /整数尺寸/);
  assert.equal(await evaluate('document.getElementById("save").disabled'), true);
  assert.deepEqual(exceptions, []);

  await evaluate('document.getElementById("quit").click()');
  await until(() => evaluate('document.getElementById("status").textContent.indexOf("程序已退出") === 0'));
  await until(() => app.exitCode === 0);
  process.stdout.write("浏览器端到端验证通过：生成、下载、上传识别、旧图失效、尺寸、窄屏布局、退出。\n");
} finally {
  if (socket) socket.close();
  if (browser) browser.kill();
  if (app.exitCode === null) app.kill();
}
