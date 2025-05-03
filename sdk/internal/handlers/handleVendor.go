package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	services "github.com/wecredit/communication-sdk/sdk/internal/services/consumerServices"
	"github.com/wecredit/communication-sdk/sdk/models/apiModels"
)

type VendorHandler struct {
	Service *services.VendorService
}

func NewVendorHandler(s *services.VendorService) *VendorHandler {
	return &VendorHandler{Service: s}
}

func (h *VendorHandler) AddVendor(c *gin.Context) {
	var v apiModels.Vendor
	if c.Request.Method != "POST" {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "Method NOT ALLOWED "})
		return
	}

	if err := c.ShouldBindJSON(&v); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input: " + err.Error()})
		return
	}

	if err := h.Service.AddVendor(&v); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, v)
}

func (h *VendorHandler) UpdateVendorByNameAndChannel(c *gin.Context) {
	name := c.Param("name")
	channel := c.Param("channel")

	var vendor apiModels.Vendor
	if err := c.ShouldBindJSON(&vendor); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}

	// Fill name & channel from URL
	vendor.Name = name
	vendor.Channel = channel

	if err := h.Service.UpdateVendorByNameAndChannel(name, channel, vendor); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "vendor updated successfully"})
}

func (h *VendorHandler) DeleteVendor(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ID"})
		return
	}

	if err := h.Service.DeleteVendor(uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "vendor deleted"})
}

func (h *VendorHandler) GetVendorByID(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ID"})
		return
	}

	vendor, err := h.Service.GetVendorByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "vendor not found"})
		return
	}

	c.JSON(http.StatusOK, vendor)
}

func (h *VendorHandler) GetVendors(c *gin.Context) {
	channel := c.Query("channel")

	vendors, err := h.Service.GetVendors(channel)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, vendors)
}
