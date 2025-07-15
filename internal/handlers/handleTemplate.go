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

	c.JSON(http.StatusOK, templates)
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

	c.JSON(http.StatusOK, template)
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

func (h *TemplateHandler) UpdateTemplateByNameAndChannel(c *gin.Context) {
	process := c.Param("process")
	stage := c.Param("stage")
	channel := c.Param("channel")
	vendor := c.Param("vendor")
	client := c.Param("client")

	stageFloat, _ := strconv.ParseFloat(stage, 64)

	var template apiModels.Templatedetails

	if err := c.ShouldBindJSON(&template); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid JSON body: %v", err)})
		return
	}

	if process != template.Process || stageFloat != template.Stage || channel != template.Channel || vendor != template.Vendor || client != template.Client {
		c.JSON(http.StatusBadRequest, gin.H{"error": "process, stage, channel, vendor or client in URL does not match with the template data"})
		return
	}

	// Fill name & channel from URL
	template.Process = process
	template.Channel = channel
	template.Vendor = vendor
	template.Client = client

	if err := h.Service.UpdateTemplateByNameAndChannel(process, stage, channel, vendor, client, template); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Template %s updated successfully for channel %s", template.TemplateName, template.Channel)})
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
