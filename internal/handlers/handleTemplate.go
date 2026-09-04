package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wecredit/communication-sdk/internal/middleware"
	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	services "github.com/wecredit/communication-sdk/internal/services/apiServices"
	"github.com/wecredit/communication-sdk/sdk/utils"
	"gorm.io/gorm"
)

const (
	defaultTemplatePageSize = 25
	maxTemplatePageSize     = 100
	maxTemplateSearchLength = 100
)

var canonicalStagePattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,7})(\.[0-9]{1,2})?$`)

type TemplateHandler struct {
	Service *services.TemplateService
}

func NewTemplateHandler(s *services.TemplateService) *TemplateHandler {
	return &TemplateHandler{Service: s}
}

func (h *TemplateHandler) GetTemplates(c *gin.Context) {
	page, pageSize, err := parseTemplatePagination(c)
	if err != nil {
		writeTemplateError(c, http.StatusBadRequest, "INVALID_PAGINATION", err.Error())
		return
	}

	stage := strings.TrimSpace(c.Query("stage"))
	if stage != "" && !canonicalStagePattern.MatchString(stage) {
		writeTemplateError(c, http.StatusBadRequest, "INVALID_STAGE", "stage must be a non-negative decimal with at most two decimal places")
		return
	}

	search := strings.TrimSpace(c.Query("search"))
	if len(search) > maxTemplateSearchLength {
		writeTemplateError(c, http.StatusBadRequest, "INVALID_SEARCH", fmt.Sprintf("search must be at most %d characters", maxTemplateSearchLength))
		return
	}

	client, err := middleware.ApplyClientListFilter(c, strings.TrimSpace(c.Query("client")))
	if err != nil {
		writeTemplateError(c, http.StatusForbidden, "FORBIDDEN", "access denied for this client")
		return
	}

	scope, _ := middleware.GetCommAdminScope(c)
	sortBy, sortDir := parseListSortQuery(c)
	result, err := h.Service.GetTemplates(apiModels.TemplateListParams{
		Process:      strings.ToUpper(strings.TrimSpace(c.Query("process"))),
		Stage:        stage,
		Channel:      strings.ToUpper(strings.TrimSpace(c.Query("channel"))),
		Vendor:       strings.ToUpper(strings.TrimSpace(c.Query("vendor"))),
		Client:       client,
		Search:       search,
		SortBy:       sortBy,
		SortDir:      sortDir,
		Unrestricted: scope.Unrestricted,
		Page:         page,
		PageSize:     pageSize,
	})
	if err != nil {
		writeTemplateServiceError(c, err)
		return
	}

	totalPages := calculateTotalPages(result.TotalItems, pageSize)
	writeTemplateSuccess(c, http.StatusOK, result.Items, "Templates retrieved successfully", &apiModels.TemplatePaginationMeta{
		Page:            page,
		PageSize:        pageSize,
		TotalItems:      result.TotalItems,
		TotalPages:      totalPages,
		HasNextPage:     int64(page) < totalPages,
		HasPreviousPage: page > 1 && result.TotalItems > 0,
	})
}

func (h *TemplateHandler) ListTemplateProcesses(c *gin.Context) {
	client, err := middleware.ApplyClientListFilter(c, strings.TrimSpace(c.Query("client")))
	if err != nil {
		writeTemplateError(c, http.StatusForbidden, "FORBIDDEN", "access denied for this client")
		return
	}

	processes, err := h.Service.ListDistinctProcesses(client)
	if err != nil {
		writeTemplateServiceError(c, err)
		return
	}

	writeTemplateSuccess(c, http.StatusOK, processes, "Processes retrieved successfully", nil)
}

func (h *TemplateHandler) GetTemplateByID(c *gin.Context) {
	id, err := parseTemplateID(c.Param("id"))
	if err != nil {
		writeTemplateError(c, http.StatusBadRequest, "INVALID_TEMPLATE_ID", err.Error())
		return
	}

	template, err := h.Service.GetTemplateByID(uint(id))
	if err != nil {
		writeTemplateServiceError(c, err)
		return
	}

	if err := middleware.EnforceClientAccess(c, template.Client); err != nil {
		writeTemplateError(c, http.StatusForbidden, "FORBIDDEN", "access denied for this client")
		return
	}

	writeTemplateSuccess(c, http.StatusOK, apiModels.NewTemplateDetailsResponse(template), "Template details retrieved successfully", nil)
}

func (h *TemplateHandler) AddTemplate(c *gin.Context) {
	var request apiModels.TemplateCreateRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeTemplateError(c, http.StatusBadRequest, "INVALID_REQUEST_BODY", fmt.Sprintf("request body is invalid: %v", err))
		return
	}

	if err := ensureJSONBodyEnded(decoder); err != nil {
		writeTemplateError(c, http.StatusBadRequest, "INVALID_REQUEST_BODY", err.Error())
		return
	}

	template := request.Template()
	if err := middleware.EnforceClientAccess(c, template.Client); err != nil {
		writeTemplateError(c, http.StatusForbidden, "FORBIDDEN", "access denied for this client")
		return
	}

	if err := h.Service.AddTemplate(&template, middleware.CommAdminUsername(c)); err != nil {
		writeTemplateServiceError(c, err)
		return
	}

	writeTemplateSuccess(c, http.StatusCreated, gin.H{"id": template.Id}, "Template created successfully", nil)
}

func (h *TemplateHandler) UpdateTemplateById(c *gin.Context) {
	id, err := parseTemplateID(c.Param("id"))
	if err != nil {
		writeTemplateError(c, http.StatusBadRequest, "INVALID_TEMPLATE_ID", err.Error())
		return
	}

	var updates apiModels.TemplateUpdateRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&updates); err != nil {
		writeTemplateError(c, http.StatusBadRequest, "INVALID_REQUEST_BODY", fmt.Sprintf("request body is invalid: %v", err))
		return
	}

	if err := ensureJSONBodyEnded(decoder); err != nil {
		writeTemplateError(c, http.StatusBadRequest, "INVALID_REQUEST_BODY", err.Error())
		return
	}

	existing, err := h.Service.GetTemplateByID(uint(id))
	if err != nil {
		writeTemplateServiceError(c, err)
		return
	}

	if err := middleware.EnforceClientAccess(c, existing.Client); err != nil {
		writeTemplateError(c, http.StatusForbidden, "FORBIDDEN", "access denied for this client")
		return
	}

	if updates.Client != nil {
		if err := middleware.EnforceClientAccess(c, *updates.Client); err != nil {
			writeTemplateError(c, http.StatusForbidden, "FORBIDDEN", "access denied for this client")
			return
		}
	}

	template, err := h.Service.UpdateTemplateById(id, updates, middleware.CommAdminUsername(c))
	if err != nil {
		writeTemplateServiceError(c, err)
		return
	}

	writeTemplateSuccess(c, http.StatusOK, apiModels.NewTemplateDetailsResponse(template), "Template updated successfully", nil)
}

func (h *TemplateHandler) DeleteTemplate(c *gin.Context) {
	id, err := parseTemplateID(c.Param("id"))
	if err != nil {
		writeTemplateError(c, http.StatusBadRequest, "INVALID_TEMPLATE_ID", err.Error())
		return
	}

	existing, err := h.Service.GetTemplateByID(uint(id))
	if err != nil {
		writeTemplateServiceError(c, err)
		return
	}

	if err := middleware.EnforceClientAccess(c, existing.Client); err != nil {
		writeTemplateError(c, http.StatusForbidden, "FORBIDDEN", "access denied for this client")
		return
	}

	if err := h.Service.DeleteTemplate(id); err != nil {
		writeTemplateServiceError(c, err)
		return
	}

	writeTemplateSuccess(c, http.StatusOK, gin.H{"id": id}, "Template deleted successfully", nil)
}

func parseTemplatePagination(c *gin.Context) (int, int, error) {
	page, err := parsePositiveQueryInt(c.Query("page"), 1)
	if err != nil {
		return 0, 0, fmt.Errorf("page must be a positive integer")
	}

	pageSize, err := parsePositiveQueryInt(c.Query("pageSize"), defaultTemplatePageSize)
	if err != nil || pageSize > maxTemplatePageSize {
		return 0, 0, fmt.Errorf("pageSize must be between 1 and %d", maxTemplatePageSize)
	}

	if page > int(^uint(0)>>1)/pageSize {
		return 0, 0, errors.New("page is too large")
	}

	return page, pageSize, nil
}

// parseListSortQuery reads sortBy/sort_by and sortDir/sort_order/sortOrder (camelCase preferred).
func parseListSortQuery(c *gin.Context) (sortBy, sortDir string) {
	sortBy = strings.TrimSpace(c.Query("sortBy"))
	if sortBy == "" {
		sortBy = strings.TrimSpace(c.Query("sort_by"))
	}

	sortDir = strings.TrimSpace(c.Query("sortDir"))
	if sortDir == "" {
		sortDir = strings.TrimSpace(c.Query("sort_order"))
	}

	if sortDir == "" {
		sortDir = strings.TrimSpace(c.Query("sortOrder"))
	}

	return sortBy, sortDir
}

func parsePositiveQueryInt(raw string, defaultValue int) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return defaultValue, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, errors.New("value must be a positive integer")
	}

	return value, nil
}

func parseTemplateID(raw string) (int, error) {
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return 0, errors.New("template ID must be a positive integer")
	}
	return id, nil
}

func calculateTotalPages(totalItems int64, pageSize int) int64 {
	if totalItems == 0 {
		return 0
	}
	return (totalItems + int64(pageSize) - 1) / int64(pageSize)
}

func ensureJSONBodyEnded(decoder *json.Decoder) error {
	var extra interface{}
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain exactly one JSON object")
		}

		return fmt.Errorf("request body is invalid: %w", err)
	}
	return nil
}

func writeTemplateSuccess(c *gin.Context, status int, data interface{}, message string, meta *apiModels.TemplatePaginationMeta) {
	c.JSON(status, apiModels.TemplateAPIResponse{
		Success: true,
		Data:    data,
		Message: message,
		Meta:    meta,
	})
}

func writeTemplateError(c *gin.Context, status int, code, message string) {
	c.JSON(status, apiModels.TemplateAPIResponse{
		Success: false,
		Error: &apiModels.TemplateAPIError{
			Code:    code,
			Message: message,
		},
	})
}

// writeTemplateServiceError writes an error to the response based on the Error Type.
func writeTemplateServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrTemplateNotFound), errors.Is(err, gorm.ErrRecordNotFound):
		writeTemplateError(c, http.StatusNotFound, "TEMPLATE_NOT_FOUND", "template was not found")

	case errors.Is(err, services.ErrTemplateConflict):
		writeTemplateError(c, http.StatusConflict, "TEMPLATE_CONFLICT", err.Error())

	case errors.Is(err, services.ErrTemplateDuplicate):
		writeTemplateError(c, http.StatusConflict, "TEMPLATE_DUPLICATE", err.Error())

	case errors.Is(err, services.ErrTemplateValidation):
		message := strings.TrimPrefix(err.Error(), services.ErrTemplateValidation.Error()+": ")
		writeTemplateError(c, http.StatusBadRequest, "TEMPLATE_VALIDATION_FAILED", message)

	case errors.Is(err, services.ErrInvalidSort):
		writeTemplateError(c, http.StatusBadRequest, "INVALID_SORT", err.Error())

	case errors.Is(err, services.ErrTemplateBusy), errors.Is(err, services.ErrConfigurationBusy):
		writeTemplateError(c, http.StatusServiceUnavailable, "TEMPLATE_BUSY", "template configuration is busy; retry the request")

	case errors.Is(err, services.ErrTemplateStale):
		writeTemplateError(c, http.StatusConflict, "TEMPLATE_STALE", "template changed while the request was being processed; retry the request")

	default:
		utils.Error(fmt.Errorf("template API request failed: %w", err))
		writeTemplateError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "an internal error occurred")
	}
}
