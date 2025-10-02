package util

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Response struct {
	Code    int         `json:"code"`
	Data    interface{} `json:"data"`
	Message string      `json:"message"`
}

func Success(c *gin.Context, data interface{}, message string) {
	c.JSON(http.StatusOK, Response{
		Code:    0,
		Data:    data,
		Message: message,
	})
}

func Error(c *gin.Context, code int, err interface{}) {
	msg := ""
	switch e := err.(type) {
	case string:
		msg = e
	case error:
		msg = e.Error()
	default:
		msg = "Internal Server Error"
	}

	zap.S().Errorf("API Error: %s", msg)

	c.JSON(code, Response{
		Code:    -1,
		Data:    nil,
		Message: msg,
	})
}
