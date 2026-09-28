package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wecredit/communication-sdk/internal/middleware"
	"github.com/wecredit/communication-sdk/internal/models/apiModels"
	services "github.com/wecredit/communication-sdk/internal/services/apiServices"
	"github.com/wecredit/communication-sdk/sdk/utils"
)

type StageConfigurationHandler struct {
	Service *services.StageConfigurationService
}

func NewStageConfigurationHandler(service *services.StageConfigurationService) *StageConfigurationHandler {
	return &StageConfigurationHandler{Service: service}
}

func (h *StageConfigurationHandler) Create(c *gin.Context) {
	request, ok := decodeStageConfigurationRequest(c)
	if !ok {
		return
	}

	if err := middleware.EnforceClientAccess(c, request.LenderName); err != nil {
		writeTemplateError(c, http.StatusForbidden, "FORBIDDEN", "access denied for this client")
		return
	}

	result, err := h.Service.Create(request, middleware.CommAdminUsername(c))
	if err != nil {
		writeStageConfigurationServiceError(c, err)
		return
	}

	writeTemplateSuccess(c, http.StatusCreated, result, "Stage configuration created successfully", nil)
}

func (h *StageConfigurationHandler) Update(c *gin.Context) {
	id, err := parseConfigurationID(c.Param("id"))
	if err != nil {
		writeTemplateError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	request, ok := decodeStageConfigurationRequest(c)
	if !ok {
		return
	}

	existing, err := h.Service.GetLenderSchedule(id)
	if err != nil {
		writeStageConfigurationServiceError(c, err)
		return
	}

	if existing.LenderSchedule != nil {
		if err := middleware.EnforceClientAccess(c, existing.LenderSchedule.LenderName); err != nil {
			writeTemplateError(c, http.StatusForbidden, "FORBIDDEN", "access denied for this client")
			return
		}
	}

	if err := middleware.EnforceClientAccess(c, request.LenderName); err != nil {
		writeTemplateError(c, http.StatusForbidden, "FORBIDDEN", "access denied for this client")
		return
	}

	result, err := h.Service.Update(id, request, middleware.CommAdminUsername(c))
	if err != nil {
		writeStageConfigurationServiceError(c, err)
		return
	}

	writeTemplateSuccess(c, http.StatusOK, result, "Stage configuration updated successfully", nil)
}

func decodeStageConfigurationRequest(c *gin.Context) (apiModels.StageConfigurationRequest, bool) {
	var request apiModels.StageConfigurationRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeTemplateError(c, http.StatusBadRequest, "INVALID_REQUEST", fmt.Sprintf("request body is invalid: %v", err))
		return request, false
	}

	if err := ensureJSONBodyEnded(decoder); err != nil {
		writeTemplateError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return request, false
	}

	return request, true
}

func (h *StageConfigurationHandler) ListLenderSchedules(c *gin.Context) {
	params, ok := parseStageConfigurationListParams(c, false)
	if !ok {
		return
	}

	lenderName, err := middleware.ApplyClientListFilter(c, params.LenderName)
	if err != nil {
		writeTemplateError(c, http.StatusForbidden, "FORBIDDEN", "access denied for this client")
		return
	}
	params.LenderName = lenderName

	result, err := h.Service.GetLenderSchedules(params)
	if err != nil {
		writeStageConfigurationServiceError(c, err)
		return
	}

	writeTemplateSuccess(c, http.StatusOK, result.Items, "Lender schedules retrieved successfully", paginationMeta(result.TotalItems, params.Page, params.PageSize))
}

func (h *StageConfigurationHandler) GetLenderSchedule(c *gin.Context) {
	id, err := parseConfigurationID(c.Param("id"))
	if err != nil {
		writeTemplateError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	result, err := h.Service.GetLenderSchedule(id)
	if err != nil {
		writeStageConfigurationServiceError(c, err)
		return
	}

	if result.LenderSchedule != nil {
		if err := middleware.EnforceClientAccess(c, result.LenderSchedule.LenderName); err != nil {
			writeTemplateError(c, http.StatusForbidden, "FORBIDDEN", "access denied for this client")
			return
		}
	}

	writeTemplateSuccess(c, http.StatusOK, result, "Lender schedule retrieved successfully", nil)
}

func (h *StageConfigurationHandler) DeleteLenderSchedule(c *gin.Context) {
	id, err := parseConfigurationID(c.Param("id"))
	if err != nil {
		writeTemplateError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	existing, err := h.Service.GetLenderSchedule(id)
	if err != nil {
		writeStageConfigurationServiceError(c, err)
		return
	}

	if existing.LenderSchedule != nil {
		if err := middleware.EnforceClientAccess(c, existing.LenderSchedule.LenderName); err != nil {
			writeTemplateError(c, http.StatusForbidden, "FORBIDDEN", "access denied for this client")
			return
		}
	}

	if err := h.Service.DeleteLenderSchedule(id); err != nil {
		writeStageConfigurationServiceError(c, err)
		return
	}

	writeTemplateSuccess(c, http.StatusOK, gin.H{"id": id}, "Lender schedule deleted successfully", nil)
}

func (h *StageConfigurationHandler) ListStageMappings(c *gin.Context) {
	params, ok := parseStageConfigurationListParams(c, true)
	if !ok {
		return
	}

	lenderName, err := middleware.ApplyClientListFilter(c, params.LenderName)
	if err != nil {
		writeTemplateError(c, http.StatusForbidden, "FORBIDDEN", "access denied for this client")
		return
	}
	params.LenderName = lenderName

	result, err := h.Service.GetStageMappings(params)
	if err != nil {
		writeStageConfigurationServiceError(c, err)
		return
	}

	writeTemplateSuccess(c, http.StatusOK, result.Items, "Stage mappings retrieved successfully", paginationMeta(result.TotalItems, params.Page, params.PageSize))
}

func (h *StageConfigurationHandler) DeleteStageMapping(c *gin.Context) {
	id, err := parseConfigurationID(c.Param("id"))
	if err != nil {
		writeTemplateError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}

	existing, err := h.Service.GetStageMappingByID(id)
	if err != nil {
		writeStageConfigurationServiceError(c, err)
		return
	}

	if err := middleware.EnforceClientAccess(c, existing.LenderName); err != nil {
		writeTemplateError(c, http.StatusForbidden, "FORBIDDEN", "access denied for this client")
		return
	}

	if err := h.Service.DeleteStageMapping(id); err != nil {
		writeStageConfigurationServiceError(c, err)
		return
	}

	writeTemplateSuccess(c, http.StatusOK, gin.H{"id": id}, "Stage mapping deleted successfully", nil)
}

func parseStageConfigurationListParams(c *gin.Context, includeSubStage bool) (apiModels.StageConfigurationListParams, bool) {
	page, pageSize, err := parseTemplatePagination(c)
	if err != nil {
		writeTemplateError(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return apiModels.StageConfigurationListParams{}, false
	}

	params := apiModels.StageConfigurationListParams{LenderName: c.Query("lenderName"), CommType: c.Query("commType"), Page: page, PageSize: pageSize}
	params.SortBy, params.SortDir = parseListSortQuery(c)

	if raw := strings.TrimSpace(c.Query("stage")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 || value > 99999999 {
			writeTemplateError(c, http.StatusBadRequest, "INVALID_REQUEST", "stage must be a non-negative integer")
			return params, false
		}
		params.Stage = &value
	}

	if includeSubStage {
		if raw := strings.TrimSpace(c.Query("subStage")); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > 99 {
				writeTemplateError(c, http.StatusBadRequest, "INVALID_REQUEST", "subStage must be between 1 and 99")
				return params, false
			}
			params.SubStage = &value
		}
	}

	return params, true
}

func parseConfigurationID(raw string) (int, error) {
	id, err := strconv.Atoi(raw)
	if err != nil || id <= 0 {
		return 0, errors.New("id must be a positive integer")
	}

	return id, nil
}

func paginationMeta(total int64, page, pageSize int) *apiModels.TemplatePaginationMeta {
	totalPages := calculateTotalPages(total, pageSize)
	return &apiModels.TemplatePaginationMeta{Page: page, PageSize: pageSize, TotalItems: total, TotalPages: totalPages, HasNextPage: int64(page) < totalPages, HasPreviousPage: page > 1 && total > 0}
}

func writeStageConfigurationServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrConfigurationValidation):
		writeTemplateError(c, http.StatusBadRequest, "INVALID_REQUEST", strings.TrimPrefix(err.Error(), services.ErrConfigurationValidation.Error()+": "))

	case errors.Is(err, services.ErrInvalidSort):
		writeTemplateError(c, http.StatusBadRequest, "INVALID_SORT", err.Error())

	case errors.Is(err, services.ErrConfigurationNotFound):
		writeTemplateError(c, http.StatusNotFound, "CONFIGURATION_NOT_FOUND", "stage configuration was not found")

	case errors.Is(err, services.ErrConfigurationAlreadyExists):
		writeTemplateError(c, http.StatusConflict, "CONFIGURATION_ALREADY_EXISTS", err.Error())

	case errors.Is(err, services.ErrConfigurationStale):
		writeTemplateError(c, http.StatusConflict, "CONFIGURATION_STALE", err.Error())

	case errors.Is(err, services.ErrConfigurationInUse):
		writeTemplateError(c, http.StatusConflict, "CONFIGURATION_IN_USE", err.Error())

	case errors.Is(err, services.ErrConfigurationBusy):
		c.Header("Retry-After", "10")
		writeTemplateError(c, http.StatusServiceUnavailable, "CONFIGURATION_BUSY", "stage configuration is busy; retry the request")

	default:
		utils.Error(fmt.Errorf("stage configuration API request failed: %w", err))
		writeTemplateError(c, http.StatusInternalServerError, "CONFIGURATION_ERROR", "an internal error occurred")
	}
}
