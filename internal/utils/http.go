// Package utils provides HTTP response utilities for consistent API responses
// with structured logging and standardized error handling.
package utils

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
)

// HTTPError represents a structured HTTP error response
type HTTPError struct {
	Status  int    `json:"status"`
	Message string `json:"error"`
	Code    string `json:"code,omitempty"`
}

// ResponseBuilder provides modern HTTP response handling with structured logging
type ResponseBuilder struct {
	logger *slog.Logger
}

// NewResponseBuilder creates a new response builder with structured logging
func NewResponseBuilder() *ResponseBuilder {
	return &ResponseBuilder{
		logger: slog.Default(),
	}
}

// Error sends a structured error response with automatic logging
func (rb *ResponseBuilder) Error(c *gin.Context, status int, message string, code ...string) {
	response := HTTPError{
		Status:  status,
		Message: message,
	}

	if len(code) > 0 {
		response.Code = code[0]
	}

	// Structured logging for errors
	rb.logger.Error("HTTP error response",
		slog.Int("status", status),
		slog.String("message", message),
		slog.String("path", c.Request.URL.Path),
		slog.String("method", c.Request.Method),
		slog.String("user_agent", c.Request.UserAgent()),
		slog.String("remote_addr", c.ClientIP()),
	)

	c.JSON(status, response)
}

// BadRequest responds with HTTP 400 Bad Request
func (rb *ResponseBuilder) BadRequest(c *gin.Context, message string) {
	rb.Error(c, http.StatusBadRequest, message, "BAD_REQUEST")
}

// NotFound responds with HTTP 404 Not Found
func (rb *ResponseBuilder) NotFound(c *gin.Context, message string) {
	rb.Error(c, http.StatusNotFound, message, "NOT_FOUND")
}

// Unauthorized responds with HTTP 401 Unauthorized
func (rb *ResponseBuilder) Unauthorized(c *gin.Context) {
	rb.Error(c, http.StatusUnauthorized, "Unauthorized", "UNAUTHORIZED")
}

// InternalError responds with HTTP 500 Internal Server Error
func (rb *ResponseBuilder) InternalError(c *gin.Context, message string) {
	rb.Error(c, http.StatusInternalServerError, message, "INTERNAL_ERROR")
}

// Global response builder instance for convenience
var HTTPResponder = NewResponseBuilder()
