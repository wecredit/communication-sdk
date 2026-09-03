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

// SendRequest is the FCM HTTP v1 send request. Message deliberately exposes
// only token and data so comm-sdk cannot accidentally emit a notification
// block with different background or killed-state behavior.
type SendRequest struct {
	Message Message `json:"message"`
}

type Message struct {
	Token string            `json:"token"`
	Data  map[string]string `json:"data"`
}

// BuildDataOnlyRequest creates the platform-neutral data payload rendered by
// the ZapCash app. Reserved fields always override navigationData entries.
func BuildDataOnlyRequest(token, title, body string, request sdkModels.CommApiRequestBody) (SendRequest, error) {
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

	return SendRequest{Message: Message{Token: token, Data: data}}, nil
}

func setDataValue(data map[string]string, key, value string) {
	if value = strings.TrimSpace(value); value != "" {
		data[key] = value
	} else {
		delete(data, key)
	}
}
