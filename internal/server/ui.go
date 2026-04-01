package server

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed static/ui.html
var uiHTML []byte

//go:embed static/ui-a.html
var uiAHTML []byte

//go:embed static/ui-b.html
var uiBHTML []byte

//go:embed static/ui-c.html
var uiCHTML []byte

func registerUI(r *gin.Engine) {
	r.GET("/ui", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", uiAHTML)
	})
	r.GET("/ui-a", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", uiAHTML)
	})
	r.GET("/ui-b", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", uiBHTML)
	})
	r.GET("/ui-c", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", uiCHTML)
	})
}
