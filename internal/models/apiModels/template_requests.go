package apiModels

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// NullableFloat64 distinguishes an omitted stage from an explicit JSON null.
// This is temporary compatibility for the legacy endpoint; the versioned API
// will expose stages as canonical decimal strings.
type NullableFloat64 struct {
	Present bool
	Value   *float64
}

func (v *NullableFloat64) UnmarshalJSON(data []byte) error {
	v.Present = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		v.Value = nil
		return nil
	}

	var value float64
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("stage must be a number or null: %w", err)
	}
	v.Value = &value
	return nil
}

// TemplateCreateRequest is the complete allowlist for template creation.
// Database identity and audit fields are intentionally absent.
type TemplateCreateRequest struct {
	Client               string   `json:"client"`
	Channel              string   `json:"channel"`
	Process              string   `json:"process"`
	Stage                *float64 `json:"stage"`
	Vendor               string   `json:"vendor"`
	TemplateName         string   `json:"templateName"`
	ImageId              string   `json:"imageId"`
	ImageUrl             string   `json:"imageUrl"`
	DltTemplateId        int64    `json:"dltTemplateId"`
	TemplateEntityId     int64    `json:"templateEntityId"`
	TemplateHeader       string   `json:"templateHeader"`
	IsActive             bool     `json:"isActive"`
	TemplateText         string   `json:"templateText"`
	Link                 string   `json:"link"`
	TemplateCategory     int64    `json:"templateCategory"`
	TemplateVariables    string   `json:"templateVariables"`
	SmsFallbackVariables string   `json:"smsFallbackVariables"`
	Subject              string   `json:"subject"`
	FromEmail            string   `json:"fromEmail"`
}


// Template converts the TemplateCreateRequest to a Templatedetails object
func (r TemplateCreateRequest) Template() Templatedetails {
	return Templatedetails{
		Client:               r.Client,
		Channel:              r.Channel,
		Process:              r.Process,
		Stage:                r.Stage,
		Vendor:               r.Vendor,
		TemplateName:         r.TemplateName,
		ImageId:              r.ImageId,
		ImageUrl:             r.ImageUrl,
		DltTemplateId:        r.DltTemplateId,
		TemplateEntityId:     r.TemplateEntityId,
		TemplateHeader:       r.TemplateHeader,
		IsActive:             r.IsActive,
		TemplateText:         r.TemplateText,
		Link:                 r.Link,
		TemplateCategory:     r.TemplateCategory,
		TemplateVariables:    r.TemplateVariables,
		SmsFallbackVariables: r.SmsFallbackVariables,
		Subject:              r.Subject,
		FromEmail:            r.FromEmail,
	}
}

// TemplateUpdateRequest is the complete allowlist for the legacy template PUT.
// Lifecycle, identity, and audit columns are intentionally absent.
type TemplateUpdateRequest struct {
	Client               *string         `json:"client"`
	Channel              *string         `json:"channel"`
	Process              *string         `json:"process"`
	Stage                NullableFloat64 `json:"stage"`
	Vendor               *string         `json:"vendor"`
	TemplateName         *string         `json:"templateName"`
	ImageId              *string         `json:"imageId"`
	ImageUrl             *string         `json:"imageUrl"`
	DltTemplateId        *int64          `json:"dltTemplateId"`
	TemplateEntityId     *int64          `json:"templateEntityId"`
	TemplateHeader       *string         `json:"templateHeader"`
	IsActive             *bool           `json:"isActive"`
	TemplateText         *string         `json:"templateText"`
	Link                 *string         `json:"link"`
	TemplateCategory     *int64          `json:"templateCategory"`
	TemplateVariables    *string         `json:"templateVariables"`
	SmsFallbackVariables *string         `json:"smsFallbackVariables"`
	Subject              *string         `json:"subject"`
	FromEmail            *string         `json:"fromEmail"`
}

func (r TemplateUpdateRequest) Apply(template *Templatedetails) {
	if r.Client != nil {
		template.Client = *r.Client
	}
	if r.Channel != nil {
		template.Channel = *r.Channel
	}
	if r.Process != nil {
		template.Process = *r.Process
	}
	if r.Stage.Present {
		template.Stage = r.Stage.Value
	}
	if r.Vendor != nil {
		template.Vendor = *r.Vendor
	}
	if r.TemplateName != nil {
		template.TemplateName = *r.TemplateName
	}
	if r.ImageId != nil {
		template.ImageId = *r.ImageId
	}
	if r.ImageUrl != nil {
		template.ImageUrl = *r.ImageUrl
	}
	if r.DltTemplateId != nil {
		template.DltTemplateId = *r.DltTemplateId
	}
	if r.TemplateEntityId != nil {
		template.TemplateEntityId = *r.TemplateEntityId
	}
	if r.TemplateHeader != nil {
		template.TemplateHeader = *r.TemplateHeader
	}
	if r.IsActive != nil {
		template.IsActive = *r.IsActive
	}
	if r.TemplateText != nil {
		template.TemplateText = *r.TemplateText
	}
	if r.Link != nil {
		template.Link = *r.Link
	}
	if r.TemplateCategory != nil {
		template.TemplateCategory = *r.TemplateCategory
	}
	if r.TemplateVariables != nil {
		template.TemplateVariables = *r.TemplateVariables
	}
	if r.SmsFallbackVariables != nil {
		template.SmsFallbackVariables = *r.SmsFallbackVariables
	}
	if r.Subject != nil {
		template.Subject = *r.Subject
	}
	if r.FromEmail != nil {
		template.FromEmail = *r.FromEmail
	}
}
