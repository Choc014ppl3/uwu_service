package response

import (
	"encoding/json"
	"net/http"
)

// -------------------------------------------------------------------------
// 1. Data Structures (โครงสร้างข้อมูล)
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

// AppError เป็น Interface เพื่อให้ response ไม่ต้องไป import internal/errors
// โครงสร้าง Error ของคุณต้องมีฟังก์ชันเหล่านี้ถึงจะเข้าเงื่อนไข
type AppError interface {
	Error() string
	HTTPStatus() int
	Code() string
	Details() map[string]interface{}
}

// -------------------------------------------------------------------------
// 2. Base Writers (แกนกลางสำหรับเขียน HTTP)
// -------------------------------------------------------------------------

// writeJSON เป็นฟังก์ชันซ่อน (Unexported) ใช้ทำงานซ้ำซากแทนฟังก์ชันอื่น
func writeJSON(w http.ResponseWriter, status int, resp Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp) // ในระดับ Prod อาจจะจับ error ตรงนี้ไปลง Log ถ้า Encode พลาด
}

// -------------------------------------------------------------------------
// 3. Success Responses (ฟังก์ชันตอบกลับเมื่อสำเร็จ)
// -------------------------------------------------------------------------

func JSON(w http.ResponseWriter, status int, data interface{}) {
	writeJSON(w, status, Response{
		Success: true,
		Data:    data,
	})
}

func JSONWithMeta(w http.ResponseWriter, status int, data interface{}, meta *Meta) {
	writeJSON(w, status, Response{
		Success: true,
		Data:    data,
		Meta:    meta,
	})
}

func OK(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusOK, data)
}

func Created(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusCreated, data)
}

func Accepted(w http.ResponseWriter, data interface{}) {
	JSON(w, http.StatusAccepted, data)
}

// NoContent เขียน HTTP 204 (กฎคือห้ามส่ง Body กลับไปเด็ดขาด)
func NoContent(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNoContent)
}

// -------------------------------------------------------------------------
// 4. Error Responses (ฟังก์ชันตอบกลับเมื่อเกิดข้อผิดพลาด)
// -------------------------------------------------------------------------

// Error ฟังก์ชันแกนกลางสำหรับ Error
func Error(w http.ResponseWriter, status int, errBody *ErrorBody) {
	writeJSON(w, status, Response{
		Success: false,
		Error:   errBody,
	})
}

// BadRequest รองรับการส่ง Details เข้ามาได้ (มีประโยชน์มากเวลาทำ Validation)
func BadRequest(w http.ResponseWriter, message string, details ...map[string]interface{}) {
	var d map[string]interface{}
	if len(details) > 0 {
		d = details[0]
	}
	Error(w, http.StatusBadRequest, &ErrorBody{
		Code:    "BAD_REQUEST",
		Message: message,
		Details: d,
	})
}

func Unauthorized(w http.ResponseWriter, message string) {
	Error(w, http.StatusUnauthorized, &ErrorBody{
		Code:    "UNAUTHORIZED",
		Message: message,
	})
}

func Forbidden(w http.ResponseWriter, message string) {
	Error(w, http.StatusForbidden, &ErrorBody{
		Code:    "FORBIDDEN",
		Message: message,
	})
}

func NotFound(w http.ResponseWriter, message string) {
	Error(w, http.StatusNotFound, &ErrorBody{
		Code:    "NOT_FOUND",
		Message: message,
	})
}

func InternalError(w http.ResponseWriter, message string) {
	Error(w, http.StatusInternalServerError, &ErrorBody{
		Code:    "INTERNAL_ERROR",
		Message: message,
	})
}

// -------------------------------------------------------------------------
// 5. Central Error Handler (ตัวจัดการ Error อัตโนมัติ)
// -------------------------------------------------------------------------

// HandleError รับ Error จาก Service มาแยกแยะและตอบกลับอัตโนมัติ
func HandleError(w http.ResponseWriter, err error) {
	// 1. ตรวจสอบว่าเป็น AppError ของระบบเราหรือไม่ (ผ่าน Interface)
	if appErr, ok := err.(AppError); ok {
		Error(w, appErr.HTTPStatus(), &ErrorBody{
			Code:    appErr.Code(),
			Message: appErr.Error(),
			Details: appErr.Details(),
		})
		return
	}

	// 2. ถ้าเป็น Error ธรรมดาที่ไม่ได้จัดการไว้ ให้ตอบ 500
	// (ข้อควรระวัง: ใน Prod ของจริง ควรส่ง Logger เข้ามาในฟังก์ชันนี้เพื่อพิมพ์ Log ก่อนตอบ 500 ด้วย)
	InternalError(w, "internal server error")
}
