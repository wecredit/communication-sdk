package handlers

import (
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

type QueryCampaignHandler struct {
	Service *services.QueryCampaignService
}

func NewQueryCampaignHandler(s *services.QueryCampaignService) *QueryCampaignHandler {
	return &QueryCampaignHandler{Service: s}
}

func (h *QueryCampaignHandler) scope(c *gin.Context) (unrestricted bool, scopedClient string) {
	scope, ok := middleware.GetCommAdminScope(c)
	if !ok {
		return false, ""
	}
	if !scope.Unrestricted && len(scope.AllowedClients) > 0 {
		scopedClient = scope.AllowedClients[0]
	}
	return scope.Unrestricted, scopedClient
}

func (h *QueryCampaignHandler) List(c *gin.Context) {
	page, pageSize, err := parseTemplatePagination(c)
	if err != nil {
		writeQCError(c, http.StatusBadRequest, "INVALID_PAGINATION", err.Error())
		return
	}
	unrestricted, scoped := h.scope(c)
	clientFilter := strings.TrimSpace(c.Query("client"))
	if !unrestricted {
		clientFilter = scoped
	}
	items, total, err := h.Service.List(apiModels.QueryCampaignListParams{
		Client:  clientFilter,
		Channel:      strings.TrimSpace(c.Query("channel")),
		Status:       strings.TrimSpace(c.Query("status")),
		Unrestricted: unrestricted,
		ScopedClient: scoped,
		Page:         page,
		PageSize:     pageSize,
	})
	
	if err != nil {
		writeQCServiceError(c, err)
		return
	}
	totalPages := calculateTotalPages(total, pageSize)
	writeQCSuccess(c, http.StatusOK, items, "Query campaigns retrieved successfully", &apiModels.TemplatePaginationMeta{
		Page: page, PageSize: pageSize, TotalItems: total, TotalPages: totalPages,
		HasNextPage: int64(page) < totalPages, HasPreviousPage: page > 1 && total > 0,
	})
}

func (h *QueryCampaignHandler) Get(c *gin.Context) {
	id, err := parsePositiveIDParam(c)
	if err != nil {
		writeQCError(c, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}
	unrestricted, scoped := h.scope(c)
	item, err := h.Service.Get(id, unrestricted, scoped)
	if err != nil {
		writeQCServiceError(c, err)
		return
	}
	writeQCSuccess(c, http.StatusOK, item, "Query campaign retrieved successfully", nil)
}

func (h *QueryCampaignHandler) Add(c *gin.Context) {
	var req apiModels.QueryCampaignCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeQCError(c, http.StatusBadRequest, "INVALID_REQUEST_BODY", err.Error())
		return
	}
	unrestricted, scoped := h.scope(c)
	if !unrestricted {
		req.Client = scoped
	}
	actor := middleware.CommAdminUsername(c)
	item, err := h.Service.Create(req, actor)
	if err != nil {
		writeQCServiceError(c, err)
		return
	}
	writeQCSuccess(c, http.StatusCreated, item, "Query campaign created successfully", nil)
}

func (h *QueryCampaignHandler) Update(c *gin.Context) {
	id, err := parsePositiveIDParam(c)
	if err != nil {
		writeQCError(c, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}
	var req apiModels.QueryCampaignUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeQCError(c, http.StatusBadRequest, "INVALID_REQUEST_BODY", err.Error())
		return
	}
	unrestricted, scoped := h.scope(c)
	actor := middleware.CommAdminUsername(c)
	item, err := h.Service.Update(id, req, actor, unrestricted, scoped)
	if err != nil {
		writeQCServiceError(c, err)
		return
	}
	writeQCSuccess(c, http.StatusOK, item, "Query campaign updated successfully", nil)
}

func (h *QueryCampaignHandler) Preview(c *gin.Context) {
	id, err := parsePositiveIDParam(c)
	if err != nil {
		writeQCError(c, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}
	unrestricted, scoped := h.scope(c)
	actor := middleware.CommAdminUsername(c)
	item, err := h.Service.PreviewAsync(id, actor, unrestricted, scoped)
	if err != nil {
		writeQCServiceError(c, err)
		return
	}
	writeQCSuccess(c, http.StatusAccepted, item, "Preview started; poll get-query-campaign until preview.status is done or failed", nil)
}

func (h *QueryCampaignHandler) Approve(c *gin.Context) {
	id, err := parsePositiveIDParam(c)
	if err != nil {
		writeQCError(c, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}
	var req apiModels.QueryCampaignApproveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeQCError(c, http.StatusBadRequest, "INVALID_REQUEST_BODY", err.Error())
		return
	}
	unrestricted, scoped := h.scope(c)
	actor := middleware.CommAdminUsername(c)
	item, err := h.Service.Approve(id, req, actor, unrestricted, scoped)
	if err != nil {
		writeQCServiceError(c, err)
		return
	}
	writeQCSuccess(c, http.StatusOK, item, "Query campaign approved successfully", nil)
}

func (h *QueryCampaignHandler) Enable(c *gin.Context) {
	id, err := parsePositiveIDParam(c)
	if err != nil {
		writeQCError(c, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}
	unrestricted, scoped := h.scope(c)
	actor := middleware.CommAdminUsername(c)
	item, err := h.Service.Enable(id, actor, unrestricted, scoped)
	if err != nil {
		writeQCServiceError(c, err)
		return
	}
	writeQCSuccess(c, http.StatusOK, item, "Query campaign enabled successfully", nil)
}

func (h *QueryCampaignHandler) Pause(c *gin.Context) {
	id, err := parsePositiveIDParam(c)
	if err != nil {
		writeQCError(c, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}
	unrestricted, scoped := h.scope(c)
	actor := middleware.CommAdminUsername(c)
	item, err := h.Service.Pause(id, actor, unrestricted, scoped)
	if err != nil {
		writeQCServiceError(c, err)
		return
	}
	writeQCSuccess(c, http.StatusOK, item, "Query campaign paused successfully", nil)
}

func (h *QueryCampaignHandler) ListRuns(c *gin.Context) {
	id, err := parsePositiveIDParam(c)
	if err != nil {
		writeQCError(c, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}
	page, pageSize, err := parseTemplatePagination(c)
	if err != nil {
		writeQCError(c, http.StatusBadRequest, "INVALID_PAGINATION", err.Error())
		return
	}
	unrestricted, scoped := h.scope(c)
	items, total, err := h.Service.ListRuns(id, unrestricted, scoped, page, pageSize)
	if err != nil {
		writeQCServiceError(c, err)
		return
	}
	totalPages := calculateTotalPages(total, pageSize)
	writeQCSuccess(c, http.StatusOK, items, "Query campaign runs retrieved successfully", &apiModels.TemplatePaginationMeta{
		Page: page, PageSize: pageSize, TotalItems: total, TotalPages: totalPages,
		HasNextPage: int64(page) < totalPages, HasPreviousPage: page > 1 && total > 0,
	})
}

func (h *QueryCampaignHandler) GetRun(c *gin.Context) {
	id, err := parsePositiveIDParam(c)
	if err != nil {
		writeQCError(c, http.StatusBadRequest, "INVALID_ID", err.Error())
		return
	}
	unrestricted, scoped := h.scope(c)
	item, err := h.Service.GetRun(id, unrestricted, scoped)
	if err != nil {
		writeQCServiceError(c, err)
		return
	}
	writeQCSuccess(c, http.StatusOK, item, "Query campaign run retrieved successfully", nil)
}

func parsePositiveIDParam(c *gin.Context) (int64, error) {
	raw := strings.TrimSpace(c.Param("id"))
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("id must be a positive integer")
	}
	return id, nil
}

func writeQCSuccess(c *gin.Context, status int, data interface{}, message string, meta *apiModels.TemplatePaginationMeta) {
	c.JSON(status, apiModels.TemplateAPIResponse{Success: true, Data: data, Message: message, Meta: meta})
}

func writeQCError(c *gin.Context, status int, code, message string) {
	c.JSON(status, apiModels.TemplateAPIResponse{
		Success: false,
		Error:   &apiModels.TemplateAPIError{Code: code, Message: message},
	})
}

func writeQCServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrQueryCampaignNotFound):
		writeQCError(c, http.StatusNotFound, "QUERY_CAMPAIGN_NOT_FOUND", "query campaign was not found")
	case errors.Is(err, services.ErrQueryCampaignConflict):
		writeQCError(c, http.StatusConflict, "QUERY_CAMPAIGN_CONFLICT", strings.TrimPrefix(err.Error(), services.ErrQueryCampaignConflict.Error()+": "))
	case errors.Is(err, services.ErrQueryCampaignValidation):
		writeQCError(c, http.StatusBadRequest, "QUERY_CAMPAIGN_VALIDATION_FAILED", strings.TrimPrefix(err.Error(), services.ErrQueryCampaignValidation.Error()+": "))
	default:
		utils.Error(fmt.Errorf("query campaign API failed: %w", err))
		writeQCError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "an internal error occurred")
	}
}
