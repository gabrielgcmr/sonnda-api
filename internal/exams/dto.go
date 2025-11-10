package exam

import "mime/multipart"

type UploadExamInput struct {
	PatientID uint                  `form:"patient_id" binding:"required"`
	File      *multipart.FileHeader `form:"file" binding:"required"`
	Name      string                `form:"name" binding:"required"`
	ExamDate  string                `form:"exam_date" binding:"required,datetime=2006-01-02"`
	Note      string                `form:"note"`
}

type ExamOutput struct {
	ID        uint   `json:"id"`
	Name      string `json:"name"`
	ExamDate  string `json:"exam_date"`
	FileURL   string `json:"file_url"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`
}
