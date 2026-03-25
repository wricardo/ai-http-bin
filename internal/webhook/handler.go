package webhook

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wricardo/ai-http-bin/internal/store"
)

var statusCodePattern = regexp.MustCompile(`^[1-5][0-9][0-9]$`)

var corsHeaders = map[string]string{
	"Access-Control-Allow-Origin":  "*",
	"Access-Control-Allow-Methods": "*",
	"Access-Control-Allow-Headers": "*",
}

type Handler struct {
	store *store.Store
}

func NewHandler(s *store.Store) *Handler {
	return &Handler{store: s}
}

func (h *Handler) Register(r *gin.Engine) {
	r.Any("/:token", h.capture)
	r.Any("/:token/*path", h.capture)
}

func (h *Handler) capture(c *gin.Context) {
	tokenID := c.Param("token")
	token, ok := h.store.GetToken(tokenID)
	if !ok {
		c.Status(http.StatusGone)
		return
	}

	if h.store.IsOverQuota(tokenID) {
		c.String(http.StatusGone, "Too many requests, please create a new URL/token")
		return
	}

	if token.Timeout > 0 {
		time.Sleep(time.Duration(token.Timeout) * time.Second)
	}

	body, _ := io.ReadAll(c.Request.Body)
	_ = c.Request.Body.Close()
	c.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	headers := make(map[string]string, len(c.Request.Header))
	for k, v := range c.Request.Header {
		headers[k] = strings.Join(v, ", ")
	}
	headersJSON, _ := json.Marshal(headers)

	queryMap := make(map[string]string)
	for k, v := range c.Request.URL.Query() {
		queryMap[k] = strings.Join(v, ", ")
	}
	queryJSON, _ := json.Marshal(queryMap)

	formDataJSON := []byte("null")
	if !strings.Contains(c.ContentType(), "application/json") {
		_ = c.Request.ParseForm()
		if len(c.Request.PostForm) > 0 {
			fm := make(map[string]string)
			for k, v := range c.Request.PostForm {
				fm[k] = strings.Join(v, ", ")
			}
			formDataJSON, _ = json.Marshal(fm)
		}
	}

	path := c.Param("path")
	if path == "" {
		path = "/"
	}

	req := &store.Request{
		TokenID:   tokenID,
		Method:    c.Request.Method,
		URL:       c.Request.RequestURI,
		Hostname:  c.Request.Host,
		Path:      path,
		Headers:   string(headersJSON),
		Query:     string(queryJSON),
		Body:      string(body),
		FormData:  string(formDataJSON),
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
	}

	h.store.AddRequest(req)

	// Determine response status: path segment overrides token default
	status := token.DefaultStatus
	if seg := strings.TrimPrefix(path, "/"); statusCodePattern.MatchString(seg) {
		var code int
		for _, b := range seg {
			code = code*10 + int(b-'0')
		}
		status = code
	}

	c.Header("X-Request-Id", req.ID)
	c.Header("X-Token-Id", tokenID)
	if token.Cors {
		for k, v := range corsHeaders {
			c.Header(k, v)
		}
	}

	c.Data(status, token.DefaultContentType, []byte(token.DefaultContent))
}
