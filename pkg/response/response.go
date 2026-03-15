package response

import (
	"encoding/json"
	"net/http"
)

// -------------------------------------------------------------------------
// 1. Data Structures
// -------------------------------------------------------------------------

type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorBody  `json:"error,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
}

type ErrorBody struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

type Meta struct {
	Page       int `json:"page,omitempty"`
	PerPage    int `json:"per_page,omitempty"`
	Total      int `json:"total,omitempty"`
	TotalPages int `json:"total_pages,omitempty"`
}

// AppError Interface ที่หน้าตาตรงกับ getter ใน errors.go เป๊ะๆ
type AppError interface {
	error
	GetCode() string
	GetMessage() string
	GetDetails() map[string]interface{}
}

// -------------------------------------------------------------------------
// 2. Base Writers
// -------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, resp Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

// -------------------------------------------------------------------------
// 3. Success Responses
// -------------------------------------------------------------------------

func JSON(w http.ResponseWriter, status int, data interface{}) {
	writeJSON(w, status, Response{Success: true, Data: data})
}

func JSONWithMeta(w http.ResponseWriter, status int, data interface{}, meta *Meta) {
	writeJSON(w, status, Response{Success: true, Data: data, Meta: meta})
}

func OK(w http.ResponseWriter, data interface{})       { JSON(w, http.StatusOK, data) }
func Created(w http.ResponseWriter, data interface{})  { JSON(w, http.StatusCreated, data) }
func Accepted(w http.ResponseWriter, data interface{}) { JSON(w, http.StatusAccepted, data) }
func NoContent(w http.ResponseWriter)                  { w.WriteHeader(http.StatusNoContent) }

// -------------------------------------------------------------------------
// 4. Error Responses & Central Error Handler
// -------------------------------------------------------------------------

// Error เป็น Base Writer สำหรับ Error
func Error(w http.ResponseWriter, status int, errBody *ErrorBody) {
	writeJSON(w, status, Response{
		Success: false,
		Error:   errBody,
	})
}

// HandleError รับจบทุก Error ของระบบ (เรียกใช้ตัวนี้ใน Handler เป็นหลัก)
func HandleError(w http.ResponseWriter, err error) {
	// 1. ตรวจสอบว่าเป็น AppError ของเราหรือไม่
	if appErr, ok := err.(AppError); ok {
		status := mapErrorCodeToHTTPStatus(appErr.GetCode())

		Error(w, status, &ErrorBody{
			Code:    appErr.GetCode(),
			Message: appErr.GetMessage(), // ใช้ GetMessage() เพื่อไม่ให้ leak SQL error ออกไปหา User
			Details: appErr.GetDetails(),
		})
		return
	}

	// 2. ถ้าเป็น Error ธรรมดาที่ไม่ได้จับคู่ไว้ (เช่น standard error) ให้ตอบ 500
	Error(w, http.StatusInternalServerError, &ErrorBody{
		Code:    "INTERNAL_ERROR",
		Message: "An unexpected internal server error occurred",
	})
}

// mapErrorCodeToHTTPStatus ผูกความสัมพันธ์ระหว่าง Domain Error กับ HTTP Status
func mapErrorCodeToHTTPStatus(code string) int {
	switch code {
	case "VALIDATION_ERROR":
		return http.StatusBadRequest
	case "UNAUTHORIZED":
		return http.StatusUnauthorized
	case "FORBIDDEN":
		return http.StatusForbidden
	case "NOT_FOUND":
		return http.StatusNotFound
	case "CONFLICT":
		return http.StatusConflict
	case "RATE_LIMIT_EXCEEDED":
		return http.StatusTooManyRequests
	case "TIMEOUT_ERROR":
		return http.StatusGatewayTimeout
	default:
		// คลุมพวก INTERNAL_ERROR, DATABASE_ERROR, AI_SERVICE_ERROR
		return http.StatusInternalServerError
	}
}
