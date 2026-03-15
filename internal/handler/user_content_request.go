package handler

import (
	"errors"
	"net/http"
	"strconv"
)

// GetContentsRequest กำหนดโครงสร้างข้อมูลที่ API นี้ต้องการ
type GetContentsRequest struct {
	FeatureID int
	Page      int
	Limit     int
}

// ValidateContentsRequest ดึงค่าจาก Request, กำหนด Default, และ Validate
func (req *GetContentsRequest) ValidateContentsRequest(r *http.Request) error {
	featureIDStr := r.URL.Query().Get("feature_id")
	if featureIDStr == "" {
		return errors.New("feature_id parameter is required")
	}

	var err error
	req.FeatureID, err = strconv.Atoi(featureIDStr)
	if err != nil || req.FeatureID <= 0 {
		return errors.New("invalid feature_id")
	}

	// ตั้งค่า Defaults
	req.Page = 1
	req.Limit = 20

	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			req.Page = p
		}
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			req.Limit = l
		}
	}

	return nil
}
