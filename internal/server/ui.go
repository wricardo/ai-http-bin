package server

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed static/ui.html
var uiHTML []byte

func registerUI(r *gin.Engine) {
	r.GET("/ui", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", uiHTML)
	})
}
