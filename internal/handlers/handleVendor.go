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

type VendorHandler struct {
	Service *services.VendorService
}

func NewVendorHandler(s *services.VendorService) *VendorHandler {
	return &VendorHandler{Service: s}
}

func (h *VendorHandler) GetVendors(c *gin.Context) {
	channel := c.Query("channel")
	name := c.Query("name")
	client := c.Query("client")

	vendors, err := h.Service.GetVendors(channel, name, client)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(vendors) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "No vendors found"})
		return
	}

	c.JSON(http.StatusOK, vendors)
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

func (h *VendorHandler) AddVendor(c *gin.Context) {
	var vendor apiModels.Vendor
	if c.Request.Method != "POST" {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "Method NOT ALLOWED "})
		return
	}

	if err := c.ShouldBindJSON(&vendor); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input: " + err.Error()})
		return
	}

	if err := h.Service.AddVendor(&vendor); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, vendor)
}

func (h *VendorHandler) UpdateVendorByNameAndChannel(c *gin.Context) {
	name := c.Param("name")
	channel := c.Param("channel")
	client := c.Query("client")

	var vendor apiModels.Vendor

	if err := c.ShouldBindJSON(&vendor); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}

	if name != vendor.Name || channel != vendor.Channel {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name and Channel doesn't match from URL"})
		return
	}

	// Fill name & channel from URL
	vendor.Name = name
	vendor.Channel = channel
	if client != "" {
		vendor.Client = client
	}

	if client != "" {
		vendor.Client = client
	}

	if err := h.Service.UpdateVendorByNameAndChannel(name, channel, vendor); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("vendor %s updated successfully for channel %s", vendor.Name, vendor.Channel)})
}

func (h *VendorHandler) DeleteVendor(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ID"})
		return
	}

	err = h.Service.DeleteVendor(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("vendor not found with id: %d", id)})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "vendor deleted successfully"})
}
