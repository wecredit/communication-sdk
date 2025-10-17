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

type ClientHandler struct {
	Service *services.ClientService
}

func NewClientHandler(s *services.ClientService) *ClientHandler {
	return &ClientHandler{Service: s}
}

func (h *ClientHandler) GetClients(c *gin.Context) {
	channel := c.Query("channel")
	name := c.Query("name")

	clients, err := h.Service.GetClients(channel, name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(clients) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "No Clients found"})
		return
	}

	c.JSON(http.StatusOK, clients)
}

func (h *ClientHandler) GetClientByID(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ID"})
		return
	}

	client, err := h.Service.GetClientByID(uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Client not found"})
		return
	}

	c.JSON(http.StatusOK, client)
}

func (h *ClientHandler) AddClient(c *gin.Context) {
	var client apiModels.Clients
	if c.Request.Method != "POST" {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "METHOD NOT ALLOWED "})
		return
	}

	if err := c.ShouldBindJSON(&client); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input: " + err.Error()})
		return
	}

	if client.Name == "" || client.Channel == "" || client.Status == 0 || client.RateLimitPerMinute == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, channel, status and rateLimitPerMinute should not be blank in request body"})
		return
	}

	if err := h.Service.AddClient(&client); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, client)
}

func (h *ClientHandler) UpdateClientByNameAndChannel(c *gin.Context) {
	name := c.Param("name")
	channel := c.Param("channel")

	var client apiModels.Clients

	if err := c.ShouldBindJSON(&client); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid JSON body: %v", err)})
		return
	}

	// if name != client.Name || channel != client.Channel {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": "Name and Channel doesn't match with URL Params"})
	// 	return
	// }

	if name == "" || channel == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and channel should not be blank in URL Params"})
		return
	}

	// // Fill name & channel from URL
	// client.Name = name
	// client.Channel = channel

	if err := h.Service.UpdateClientByNameAndChannel(name, channel, client); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Client %s updated successfully for channel %s", name, channel)})
}

func (h *ClientHandler) DeleteClient(c *gin.Context) {
	idParam := c.Param("id")
	id, err := strconv.Atoi(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ID"})
		return
	}

	err = h.Service.DeleteClient(id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Client not found with id: %d", id)})
		return
	} else if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Client deleted successfully"})
}

func (h *ClientHandler) ValidateClient(c *gin.Context) {
	var userInput apiModels.Userbasicauth
	if err := c.ShouldBindJSON(&userInput); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid input: " + err.Error()})
		return
	}

	// Extract the "Channel" header
	channel := c.GetHeader("Channel")
	if channel == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing Channel header"})
		return
	}

	user, channel, topicArn, err := h.Service.ValidateCredentials(userInput.Username, userInput.Password, channel)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  "authentication successful",
		"user":     user,
		"channel":  channel,
		"topicArn": topicArn,
	})
}
