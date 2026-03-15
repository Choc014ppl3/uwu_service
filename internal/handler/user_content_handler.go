package handler

import (
	"net/http"

	"github.com/windfall/uwu_service/internal/service"
	"github.com/windfall/uwu_service/pkg/response"
)

type UserContentHandler struct {
	service *service.UserContentService
}

func NewUserContentHandler(service *service.UserContentService) *UserContentHandler {
	return &UserContentHandler{service: service}
}

func (h *UserContentHandler) GetContentsByFeature(w http.ResponseWriter, r *http.Request) {
	// 1. ประกาศ Struct ที่เราเขียนไว้ในไฟล์ _request.go (เรียกใช้ได้เลยเพราะอยู่ package เดียวกัน)
	var req GetContentsRequest

	// 2. ให้ Request จัดการ Parse และ Validate ตัวมันเอง
	if err := req.ValidateContentsRequest(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// 3. ส่งข้อมูลที่สะอาดแล้ว (Clean Data) ให้ Service ไปจัดการ Business Logic
	items, total, err := h.service.GetContentsByFeature(
		r.Context(),
		req.FeatureID,
		req.Page,
		req.Limit,
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// 4. จัดเตรียมข้อมูลส่งกลับ
	result := map[string]interface{}{
		"data":  items,
		"total": total,
		"page":  req.Page,
		"limit": req.Limit,
	}

	// ส่ง Response ตอบกลับ (เปลี่ยนจาก Created เป็น OK หรือฟังก์ชันที่คุณมี)
	response.OK(w, result)
}
