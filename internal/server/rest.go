package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/wricardo/ai-http-bin/internal/store"
)

type restAPI struct {
	store   *store.Store
	baseURL string
}

func (a *restAPI) register(r *gin.RouterGroup) {
	r.POST("/tokens", a.createToken)
	r.GET("/tokens", a.listTokens)
	r.GET("/tokens/:id", a.getToken)
	r.PUT("/tokens/:id", a.updateToken)
	r.DELETE("/tokens/:id", a.deleteToken)
	r.PUT("/tokens/:id/script", a.setScript)
	r.GET("/tokens/:id/requests", a.listRequests)
	r.DELETE("/tokens/:id/requests", a.clearRequests)
	r.GET("/requests/:id", a.getRequest)
	r.DELETE("/requests/:id", a.deleteRequest)
	r.POST("/claim/:id", a.claimToken)
	r.GET("/vars", a.listGlobalVars)
	r.PUT("/vars/:key", a.setGlobalVar)
	r.DELETE("/vars/:key", a.deleteGlobalVar)
}

func (a *restAPI) agentID(c *gin.Context) string {
	return c.GetHeader("X-Agent-Id")
}

type createTokenRequest struct {
	DefaultStatus      *int    `json:"default_status"`
	DefaultContent     *string `json:"default_content"`
	DefaultContentType *string `json:"default_content_type"`
	Timeout            *int    `json:"timeout"`
	Cors               *bool   `json:"cors"`
	Script             *string `json:"script"`
}

func (a *restAPI) createToken(c *gin.Context) {
	var req createTokenRequest
	// Allow empty body
	_ = c.ShouldBindJSON(&req)

	t := a.store.CreateToken(c.ClientIP(), c.Request.UserAgent(), a.agentID(c))
	if req.DefaultStatus != nil {
		t.DefaultStatus = *req.DefaultStatus
	}
	if req.DefaultContent != nil {
		t.DefaultContent = *req.DefaultContent
	}
	if req.DefaultContentType != nil {
		t.DefaultContentType = *req.DefaultContentType
	}
	if req.Timeout != nil {
		t.Timeout = *req.Timeout
	}
	if req.Cors != nil {
		t.Cors = *req.Cors
	}
	if req.Script != nil {
		t.Script = *req.Script
	}

	c.JSON(http.StatusCreated, tokenJSON(a.store, a.baseURL, t))
}

func (a *restAPI) listTokens(c *gin.Context) {
	agentID := a.agentID(c)
	var tokens []*store.Token
	if agentID != "" {
		tokens = a.store.ListTokensByAgent(agentID)
	} else {
		tokens = a.store.ListTokens()
	}
	out := make([]gin.H, len(tokens))
	for i, t := range tokens {
		out[i] = tokenJSON(a.store, a.baseURL, t)
	}
	c.JSON(http.StatusOK, out)
}

func (a *restAPI) getToken(c *gin.Context) {
	t, ok := a.store.GetToken(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
		return
	}
	c.JSON(http.StatusOK, tokenJSON(a.store, a.baseURL, t))
}

func (a *restAPI) updateToken(c *gin.Context) {
	t, ok := a.store.GetToken(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
		return
	}
	var req createTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.DefaultStatus != nil {
		t.DefaultStatus = *req.DefaultStatus
	}
	if req.DefaultContent != nil {
		t.DefaultContent = *req.DefaultContent
	}
	if req.DefaultContentType != nil {
		t.DefaultContentType = *req.DefaultContentType
	}
	if req.Timeout != nil {
		t.Timeout = *req.Timeout
	}
	if req.Cors != nil {
		t.Cors = *req.Cors
	}
	if req.Script != nil {
		t.Script = *req.Script
	}
	a.store.UpdateToken(t.ID, t.DefaultContent, t.DefaultContentType, t.DefaultStatus, t.Timeout, t.Cors)
	c.JSON(http.StatusOK, tokenJSON(a.store, a.baseURL, t))
}

func (a *restAPI) deleteToken(c *gin.Context) {
	if a.store.DeleteToken(c.Param("id")) {
		c.JSON(http.StatusOK, gin.H{"deleted": true})
	} else {
		c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
	}
}

func (a *restAPI) listRequests(c *gin.Context) {
	tokenID := c.Param("id")
	if _, ok := a.store.GetToken(tokenID); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "50"))
	newest := c.DefaultQuery("sorting", "oldest") == "newest"

	reqs, total := a.store.ListRequests(tokenID, page, perPage, newest)
	out := make([]gin.H, len(reqs))
	for i, r := range reqs {
		out[i] = requestJSON(r)
	}
	c.JSON(http.StatusOK, gin.H{
		"data":  out,
		"total": total,
		"page":  page,
	})
}

func (a *restAPI) clearRequests(c *gin.Context) {
	if a.store.ClearRequests(c.Param("id")) {
		c.JSON(http.StatusOK, gin.H{"cleared": true})
	} else {
		c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
	}
}

func (a *restAPI) getRequest(c *gin.Context) {
	r, ok := a.store.GetRequest(c.Param("id"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
		return
	}
	c.JSON(http.StatusOK, requestJSON(r))
}

func (a *restAPI) deleteRequest(c *gin.Context) {
	if a.store.DeleteRequest(c.Param("id")) {
		c.JSON(http.StatusOK, gin.H{"deleted": true})
	} else {
		c.JSON(http.StatusNotFound, gin.H{"error": "request not found"})
	}
}

func (a *restAPI) claimToken(c *gin.Context) {
	agentID := a.agentID(c)
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "X-Agent-Id header required"})
		return
	}
	t, ok := a.store.ClaimToken(c.Param("id"), agentID)
	if !ok {
		c.JSON(http.StatusConflict, gin.H{"error": "token not found or already claimed"})
		return
	}
	c.JSON(http.StatusOK, tokenJSON(a.store, a.baseURL, t))
}

func (a *restAPI) setScript(c *gin.Context) {
	var body struct {
		Script string `json:"script"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !a.store.SetScript(c.Param("id"), body.Script) {
		c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
		return
	}
	t, _ := a.store.GetToken(c.Param("id"))
	c.JSON(http.StatusOK, tokenJSON(a.store, a.baseURL, t))
}

func (a *restAPI) listGlobalVars(c *gin.Context) {
	vars := a.store.ListGlobalVars()
	out := make([]gin.H, 0, len(vars))
	for k, v := range vars {
		out = append(out, gin.H{"key": k, "value": v})
	}
	c.JSON(http.StatusOK, out)
}

func (a *restAPI) setGlobalVar(c *gin.Context) {
	var body struct {
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	a.store.SetGlobalVar(c.Param("key"), body.Value)
	c.JSON(http.StatusOK, gin.H{"key": c.Param("key"), "value": body.Value})
}

func (a *restAPI) deleteGlobalVar(c *gin.Context) {
	a.store.DeleteGlobalVar(c.Param("key"))
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func tokenJSON(s *store.Store, baseURL string, t *store.Token) gin.H {
	h := gin.H{
		"id":                   t.ID,
		"url":                  s.TokenURL(baseURL, t.ID),
		"ip":                   t.IP,
		"user_agent":           t.UserAgent,
		"created_at":           t.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		"request_count":        s.RequestCount(t.ID),
		"default_status":       t.DefaultStatus,
		"default_content":      t.DefaultContent,
		"default_content_type": t.DefaultContentType,
		"timeout":              t.Timeout,
		"cors":                 t.Cors,
		"script":               t.Script,
	}
	if !t.ExpiresAt.IsZero() {
		h["expires_at"] = t.ExpiresAt.Format("2006-01-02T15:04:05Z07:00")
	}
	if t.AgentID != "" {
		h["agent_id"] = t.AgentID
	}
	return h
}

func requestJSON(r *store.Request) gin.H {
	return gin.H{
		"id":         r.ID,
		"token_id":   r.TokenID,
		"method":     r.Method,
		"url":        r.URL,
		"hostname":   r.Hostname,
		"path":       r.Path,
		"headers":    r.Headers,
		"query":      r.Query,
		"body":       r.Body,
		"form_data":  r.FormData,
		"ip":         r.IP,
		"user_agent": r.UserAgent,
		"created_at": r.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
