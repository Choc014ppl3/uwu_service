package response

import (
	"encoding/json"
	"net/http"
)

// Response represents a standard API response.
type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorBody  `json:"error,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
}

// ErrorBody represents an error in the response.
type ErrorBody struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// Meta contains metadata about the response.
type Meta struct {
	Page       int `json:"page,omitempty"`
	PerPage    int `json:"per_page,omitempty"`
	Total      int `json:"total,omitempty"`
	TotalPages int `json:"total_pages,omitempty"`
}

// JSON writes a JSON response.
func JSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := Response{
		Success: status >= 200 && status < 300,
		Data:    data,
	}

	json.NewEncoder(w).Encode(resp)
}

// JSONWithMeta writes a JSON response with metadata.
func JSONWithMeta(w http.ResponseWriter, status int, data interface{}, meta *Meta) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	resp := Response{
		Success: status >= 200 && status < 300,
		Data:    data,
		Meta:    meta,
	}

	json.NewEncoder(w).Encode(resp)
}

// Error writes an error response.
func Error(w http.ResponseWriter, status int, err interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	var errBody *ErrorBody

	switch e := err.(type) {
	case *ErrorBody:
		errBody = e
	case interface{ Error() string }:
		errBody = &ErrorBody{
			Code:    "ERROR",
			Message: e.Error(),
		}
	case string:
		errBody = &ErrorBody{
			Code:    "ERROR",
			Message: e,
		}
	default:
		errBody = &ErrorBody{
			Code:    "UNKNOWN_ERROR",
			Message: "An unknown error occurred",
		}
	}

	resp := Response{
		Success: false,
		Error:   errBody,
	}

	json.NewEncoder(w).Encode(resp)
}

// Created writes a 201 Created response.
func Created(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusCreated, data)
}

// NoContent writes a 204 No Content response.
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// NotFound writes a 404 Not Found response.
func NotFound(w http.ResponseWriter, message string) {
	Error(w, http.StatusNotFound, &ErrorBody{
		Code:    "NOT_FOUND",
		Message: message,
	})
}

// BadRequest writes a 400 Bad Request response.
func BadRequest(w http.ResponseWriter, message string) {
	Error(w, http.StatusBadRequest, &ErrorBody{
		Code:    "BAD_REQUEST",
		Message: message,
	})
}

// Unauthorized writes a 401 Unauthorized response.
func Unauthorized(w http.ResponseWriter, message string) {
	Error(w, http.StatusUnauthorized, &ErrorBody{
		Code:    "UNAUTHORIZED",
		Message: message,
	})
}

// Forbidden writes a 403 Forbidden response.
func Forbidden(w http.ResponseWriter, message string) {
	Error(w, http.StatusForbidden, &ErrorBody{
		Code:    "FORBIDDEN",
		Message: message,
	})
}

// InternalError writes a 500 Internal Server Error response.
func InternalError(w http.ResponseWriter, message string) {
	Error(w, http.StatusInternalServerError, &ErrorBody{
		Code:    "INTERNAL_ERROR",
		Message: message,
	})
}
