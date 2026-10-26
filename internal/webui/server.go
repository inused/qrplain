// Package webui 提供二维码浏览器界面和本机 HTTP 服务。
package webui

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"qrplain/internal/qr"
)

// assets 内嵌页面、样式和脚本。
//
//go:embed assets/*
var assets embed.FS

// handler 保存监听地址 host、随机凭据 token、退出回调 stop 和图片任务锁 work。
type handler struct {
	host  string
	token string
	stop  func()
	work  sync.Mutex // 图片处理串行执行，限制并发图片展开造成的峰值内存。
}

// Run 启动随机端口的本机服务并打开浏览器，直到上下文取消或页面请求退出。
//
//	@param ctx context.Context 进程退出信号；取消后关闭 HTTP 服务。
//	@param diagnostics io.Writer 本地地址、启动失败与退出诊断的输出位置。
//	@return error 监听、随机数、服务运行或关闭失败的原因；正常退出为 nil。
func Run(ctx context.Context, diagnostics io.Writer) error {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("启动本地服务失败：%w", err)
	}
	defer listener.Close()
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("创建本地访问凭据失败：%w", err)
	}
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	h := &handler{host: listener.Addr().String(), token: hex.EncodeToString(secret), stop: stop}
	server := &http.Server{
		Handler: h, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second,
		IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10,
	}
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	// 凭据仅放在 URL 片段中，浏览器不会把片段作为 HTTP 请求或 Referer 发送。
	address := "http://" + h.host + "/#" + h.token
	if _, err = fmt.Fprintln(diagnostics, "QRPlain 本地二维码：", address, "\n退出：点击页面“退出程序”或在终端按 Ctrl+C。关闭标签页会保留进程。"); err != nil {
		server.Close()
		return err
	}
	if err := openBrowser(ctx, address); err != nil {
		fmt.Fprintln(diagnostics, "打开浏览器失败，请手动访问上面的地址：", err)
	}
	select {
	case err = <-finished:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			server.Close()
			return fmt.Errorf("关闭本地服务失败：%w", err)
		}
		return nil
	}
}

// ServeHTTP 校验 Host、访问凭据与来源，分派静态资源和本机 API。
func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data: blob:; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	if r.Host != h.host {
		http.Error(w, "访问地址无效，请使用程序显示的本机地址", http.StatusForbidden)
		return
	}
	if r.Method == http.MethodGet {
		files := map[string]string{"/": "index.html", "/app.js": "app.js", "/style.css": "style.css"}
		name, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		data, err := assets.ReadFile("assets/" + name)
		if err != nil {
			http.Error(w, "读取内嵌页面失败", http.StatusInternalServerError)
			return
		}
		types := map[string]string{"index.html": "text/html; charset=utf-8", "app.js": "text/javascript; charset=utf-8", "style.css": "text/css; charset=utf-8"}
		w.Header().Set("Content-Type", types[name])
		w.Write(data)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "不支持的请求方法", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("Origin") != "http://"+h.host || subtle.ConstantTimeCompare([]byte(r.Header.Get("X-QR-Token")), []byte(h.token)) != 1 {
		http.Error(w, "访问凭据或来源无效，请用程序显示的完整地址打开页面", http.StatusForbidden)
		return
	}
	if r.URL.Path == "/api/quit" {
		writeJSON(w, http.StatusOK, map[string]string{"message": "程序已退出，可以关闭此页面。"})
		h.stop()
		return
	}
	// 同时处理一个图片任务，以限制解码时的峰值内存。
	if !h.work.TryLock() {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "正在处理另一项任务，请稍后重试"})
		return
	}
	defer h.work.Unlock()
	switch r.URL.Path {
	case "/api/encode":
		h.encode(w, r)
	case "/api/decode":
		r.Body = http.MaxBytesReader(w, r.Body, qr.MaxImageBytes)
		text, err := qr.Decode(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"text": text})
	default:
		http.NotFound(w, r)
	}
}

// encode 接受 text 正文、size 像素边长、level 纠错等级，返回 PNG 的 Base64 与模块信息。
func (h *handler) encode(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Text  string `json:"text"`
		Size  int    `json:"size"`
		Level string `json:"level"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "生成参数无效或正文过大"})
		return
	}
	if err := decoder.Decode(new(interface{})); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求只能包含一组生成参数"})
		return
	}
	code, err := qr.New(request.Text, qr.NormalizeLevel(request.Level))
	var data []byte
	if err == nil {
		data, err = code.PNG(request.Size)
	}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	// png 是 PNG 字节的 JSON Base64；modules 含留白，version 为标准二维码版本。
	writeJSON(w, http.StatusOK, map[string]interface{}{"png": data, "modules": len(code.Modules), "version": code.Version})
}

// writeJSON 发送 JSON 响应，并将发送错误写入标准错误。
func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		fmt.Fprintln(os.Stderr, "发送页面响应失败：", err)
	}
}
