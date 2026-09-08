package fcm

import (
	"errors"
	"strings"

	"github.com/wecredit/communication-sdk/sdk/models/sdkModels"
)

const (
	dataKeyTitle             = "title"
	dataKeyBody              = "body"
	dataKeyEventID           = "eventId"
	dataKeyNotificationEvent = "notificationEvent"
	dataKeyDeepLink          = "deepLink"
	dataKeyUserID            = "userId"
	dataKeyApplicationNumber = "applicationNumber"
)

// SendRequest is the FCM HTTP v1 send request.
// Includes notification (system-tray display, Firebase Console parity) plus
// data (deep link / event fields for the app).
type SendRequest struct {
	Message Message `json:"message"`
}

type Notification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type AndroidConfig struct {
	Priority string `json:"priority,omitempty"`
}

type Message struct {
	Token        string            `json:"token"`
	Notification *Notification     `json:"notification,omitempty"`
	Android      *AndroidConfig    `json:"android,omitempty"`
	Data         map[string]string `json:"data"`
}

// BuildSendRequest builds notification + data (Console-style display + app extras).
// Reserved data fields always override navigationData entries.
func BuildSendRequest(token, title, body string, request sdkModels.CommApiRequestBody) (SendRequest, error) {
	token = strings.TrimSpace(token)
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if token == "" {
		return SendRequest{}, errors.New("FCM device token is required")
	}

	if title == "" {
		return SendRequest{}, errors.New("FCM title is required")
	}

	if body == "" {
		return SendRequest{}, errors.New("FCM body is required")
	}

	if strings.TrimSpace(request.EventId) == "" {
		return SendRequest{}, errors.New("FCM eventId is required")
	}

	data := make(map[string]string, len(request.NavigationData)+7)
	for key, value := range request.NavigationData {
		key = strings.TrimSpace(key)
		if key != "" {
			data[key] = value
		}
	}

	data[dataKeyTitle] = title
	data[dataKeyBody] = body
	data[dataKeyEventID] = strings.TrimSpace(request.EventId)
	setDataValue(data, dataKeyNotificationEvent, request.NotificationEvent)
	setDataValue(data, dataKeyDeepLink, request.DeepLink)
	setDataValue(data, dataKeyUserID, request.UserId)
	setDataValue(data, dataKeyApplicationNumber, request.ApplicationNumber)

	return SendRequest{Message: Message{
		Token: token,
		Notification: &Notification{
			Title: title,
			Body:  body,
		},
		Android: &AndroidConfig{Priority: "HIGH"},
		Data:    data,
	}}, nil
}

// BuildDataOnlyRequest is kept as an alias for older call sites/tests during the
// notification-parity trial. Prefer BuildSendRequest.
func BuildDataOnlyRequest(token, title, body string, request sdkModels.CommApiRequestBody) (SendRequest, error) {
	return BuildSendRequest(token, title, body, request)
}

func setDataValue(data map[string]string, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		data[key] = value
	} else {
		delete(data, key)
	}
}
