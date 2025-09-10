package utils

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RespondWithError sends a JSON error response
func RespondWithError(c *gin.Context, statusCode int, message string) {
	c.JSON(statusCode, gin.H{"error": message})
}

// RespondWithBadRequest sends a 400 Bad Request response
func RespondWithBadRequest(c *gin.Context, message string) {
	RespondWithError(c, http.StatusBadRequest, message)
}

// RespondWithNotFound sends a 404 Not Found response
func RespondWithNotFound(c *gin.Context, message string) {
	RespondWithError(c, http.StatusNotFound, message)
}

// RespondWithInternalError sends a 500 Internal Server Error response
func RespondWithInternalError(c *gin.Context, message string) {
	RespondWithError(c, http.StatusInternalServerError, message)
}

// RespondWithUnauthorized sends a 401 Unauthorized response
func RespondWithUnauthorized(c *gin.Context) {
	RespondWithError(c, http.StatusUnauthorized, "Unauthorized")
}
