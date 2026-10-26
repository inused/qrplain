package webui

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"qrplain/internal/qr"
)

// testRequest 构造带有效 Host、Origin 和凭据的本机 API 请求。
func testRequest(h *handler, path string, data []byte) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "http://"+h.host+path, bytes.NewReader(data))
	r.Header.Set("Origin", "http://"+h.host)
	r.Header.Set("X-QR-Token", h.token)
	return r
}

// TestBrowserAPIRoundTrip 检查 API 生成与识别的正文一致性，以及退出回调。
func TestBrowserAPIRoundTrip(t *testing.T) {
	stopped := false
	h := &handler{host: "127.0.0.1:12345", token: strings.Repeat("a", 64), stop: func() { stopped = true }}
	content := "<script>原文🙂</script>\r\n\n"
	request, _ := json.Marshal(map[string]interface{}{"text": content, "size": 512, "level": "M"})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, testRequest(h, "/api/encode", request))
	if w.Code != http.StatusOK {
		t.Fatalf("生成失败：%s", w.Body)
	}
	var encoded struct {
		PNG     string `json:"png"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &encoded); err != nil {
		t.Fatal(err)
	}
	image, err := base64.StdEncoding.DecodeString(encoded.PNG)
	if err != nil {
		t.Fatal(err)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, testRequest(h, "/api/decode", image))
	var decoded map[string]string
	json.Unmarshal(w.Body.Bytes(), &decoded)
	if w.Code != http.StatusOK || decoded["text"] != content {
		t.Fatalf("识别正文不一致：%s", w.Body)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, testRequest(h, "/api/quit", nil))
	if w.Code != http.StatusOK || !stopped {
		t.Fatal("退出回调未执行")
	}
}

// TestAccessBoundary 覆盖 Host 重绑定、跨来源、缺凭据与错误方法，外部请求不得触发退出。
func TestAccessBoundary(t *testing.T) {
	stopped := false
	h := &handler{host: "127.0.0.1:12345", token: "secret", stop: func() { stopped = true }}
	for _, change := range []func(*http.Request){
		func(r *http.Request) { r.Host = "evil.test" },
		func(r *http.Request) { r.Header.Set("Origin", "https://evil.test") },
		func(r *http.Request) { r.Header.Del("Origin") },
		func(r *http.Request) { r.Header.Del("X-QR-Token") },
		func(r *http.Request) { r.Header.Set("X-QR-Token", "wrong") },
	} {
		r := testRequest(h, "/api/quit", nil)
		change(r)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusForbidden || stopped {
			t.Fatal("不可信请求穿过访问边界")
		}
	}
	w := httptest.NewRecorder()
	r := testRequest(h, "/api/quit", nil)
	r.Method = http.MethodDelete
	h.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed || stopped {
		t.Fatal("错误方法未拒绝")
	}
}

// TestInvalidAPIInput 验证畸形 JSON、过大正文、非法尺寸、损坏图片及并发任务拒绝。
func TestInvalidAPIInput(t *testing.T) {
	h := &handler{host: "127.0.0.1:12345", token: "secret", stop: func() {}}
	for _, body := range []string{
		"{", `{"text":"","size":512,"level":"M"}`,
		`{"text":"x","size":0,"level":"M"}`,
		`{"text":"x","size":4097,"level":"M"}`,
		`{"text":"x","size":512,"level":"M","path":"/etc/passwd"}`,
		`{"text":"x","size":512,"level":"M"} {}`,
		strings.Repeat(" ", 65537),
	} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, testRequest(h, "/api/encode", []byte(body)))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("无效参数未拒绝：%s", body)
		}
	}
	for _, data := range [][]byte{[]byte("bad image"), make([]byte, qr.MaxImageBytes+1)} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, testRequest(h, "/api/decode", data))
		if w.Code != http.StatusBadRequest {
			t.Fatal("无效图片未拒绝")
		}
	}
	h.work.Lock()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, testRequest(h, "/api/decode", nil))
	h.work.Unlock()
	if w.Code != http.StatusTooManyRequests {
		t.Fatal("重复图片任务未拒绝")
	}
}

// TestStaticResources 检查内嵌资源、凭据隔离和页面响应头。
func TestStaticResources(t *testing.T) {
	h := &handler{host: "127.0.0.1:12345", token: "test-secret-not-in-page", stop: func() {}}
	for _, path := range []string{"/", "/app.js", "/style.css"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://"+h.host+path, nil))
		if w.Code != 200 || w.Body.Len() == 0 || strings.Contains(w.Body.String(), h.token) {
			t.Fatalf("静态资源异常：%s", path)
		}
		if w.Header().Get("Cache-Control") != "no-store" || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") {
			t.Fatal("页面响应缺少访问边界配置")
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "http://"+h.host+"/api/quit", nil))
	if w.Code != http.StatusNotFound {
		t.Fatal("GET 请求触发了 API")
	}
}
