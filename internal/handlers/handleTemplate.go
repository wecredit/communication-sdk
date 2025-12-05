package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	services "github.com/wecredit/communication-sdk/internal/services/apiServices"

	"gorm.io/gorm"
)

type TemplateHandler struct {
	Service *services.TemplateService
}

// TemplateResponse ensures all fields are always present in API response
// Without omitempty, all fields will be included even if empty
type TemplateResponse struct {
	Id                int     `json:"id"`
	Client            string  `json:"client"`
	Channel           string  `json:"channel"`
	Process           string  `json:"process"`
	Stage             float64 `json:"stage"`
	Vendor            string  `json:"vendor"`
	TemplateName      string  `json:"templateName"`
	ImageId           string  `json:"imageId"`
	ImageUrl          string  `json:"imageUrl"`
	DltTemplateId     int64   `json:"dltTemplateId"`
	IsActive          bool    `json:"isActive"`
	TemplateText      string  `json:"templateText"`
	Link              string  `json:"link"`
	CreatedOn         string  `json:"createdOn"`
	UpdatedOn         *string `json:"updatedOn"`
	TemplateCategory  int64   `json:"templateCategory"`
	TemplateVariables string  `json:"templateVariables"`
	Subject           string  `json:"subject"`
	FromEmail         string  `json:"fromEmail"`
}

func NewTemplateHandler(s *services.TemplateService) *TemplateHandler {
	return &TemplateHandler{Service: s}
}

func (h *TemplateHandler) GetTemplates(c *gin.Context) {
	process := c.Query("process")
	stage := c.Query("stage")
	channel := c.Query("channel")
	vendor := c.Query("vendor")
	client := c.Query("client")

	templates, err := h.Service.GetTemplates(process, stage, client, channel, vendor)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(templates) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "No Templates found"})
		return
	}

	// Convert all templates to response format ensuring all fields present
	response := make([]TemplateResponse, len(templates))
	for i := range templates {
		response[i] = toTemplateResponse(&templates[i])
	}
	c.JSON(http.StatusOK, response)
}

func (h *TemplateHandler) GetTemplateByID(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ID"})
		return
	}

	template, err := h.Service.GetTemplateByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Template not found"})
		return
	}

	// Convert to response format to ensure all fields are present (even if empty)
	response := toTemplateResponse(template)
	c.JSON(http.StatusOK, response)
}

func (h *TemplateHandler) AddTemplate(c *gin.Context) {
	var template apiModels.Templatedetails
	if c.Request.Method != "POST" {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "METHOD NOT ALLOWED "})
		return
	}

	if err := c.ShouldBindJSON(&template); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input: " + err.Error()})
		return
	}

	if err := h.Service.AddTemplate(&template); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, template)
}

func (h *TemplateHandler) UpdateTemplateById(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ID"})
		return
	}

	// Bind into a map so we only get fields that are present in JSON
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid JSON body: %v", err)})
		return
	}

	if err := h.Service.UpdateTemplateById(id, updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Template %d updated successfully", id)})
}

func (h *TemplateHandler) DeleteTemplate(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ID"})
		return
	}

	err = h.Service.DeleteTemplate(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Template not found with id: %d", id)})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Template deleted successfully"})
}

// toTemplateResponse converts DB model to API response struct
// Ensures all fields are always present in consistent order (struct field order)
// No omitempty tags = all fields included even if empty
func toTemplateResponse(template *apiModels.Templatedetails) TemplateResponse {
	var updatedOn *string
	if template.UpdatedOn != nil {
		formattedTime := template.UpdatedOn.Format("2006-01-02T15:04:05Z07:00")
		updatedOn = &formattedTime
	}

	return TemplateResponse{
		Id:                template.Id,
		Client:            template.Client,
		Channel:           template.Channel,
		Process:           template.Process,
		Stage:             template.Stage,
		Vendor:            template.Vendor,
		TemplateName:      template.TemplateName,
		ImageId:           template.ImageId,
		ImageUrl:          template.ImageUrl,
		DltTemplateId:     template.DltTemplateId,
		IsActive:          template.IsActive,
		TemplateText:      template.TemplateText,
		Link:              template.Link,
		TemplateCategory:  template.TemplateCategory,
		TemplateVariables: template.TemplateVariables,
		Subject:           template.Subject,
		FromEmail:         template.FromEmail,
		CreatedOn:         template.CreatedOn.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedOn:         updatedOn,
	}
}
