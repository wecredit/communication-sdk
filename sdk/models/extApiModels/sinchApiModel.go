package extapimodels

type SinchAPIModel struct {
	Mobile       string
	TemplateName string
	ImageID      string
	Process      string
	ButtonLink   string
	AccessToken  string
}

type SinchTokenResponse struct {
	AccessToken      string `json:"access_token"`
	ExpiresIn        int    `json:"expires_in"`
	RefreshToken     string `json:"refresh_token"`
	RefreshExpiresIn int    `json:"refresh_expires_in"`
	TokenType        string `json:"token_type"`
}

type SinchRcsPayload struct {
	AppID     string `json:"app_id"`
	Recipient struct {
		IdentifiedBy struct {
			ChannelIdentities []struct {
				Channel  string `json:"channel"`
				Identity string `json:"identity"`
			} `json:"channel_identities"`
		} `json:"identified_by"`
	} `json:"recipient"`
	Message struct {
		TemplateMessage struct {
			ChannelTemplate struct {
				RCS struct {
					TemplateId   string `json:"template_id"`
					LanguageCode string `json:"language_code"`
				} `json:"RCS"`
			} `json:"channel_template"`
		} `json:"template_message"`
	} `json:"message"`
}

type SinchSmsPayload struct {
	Process       string
	Stage         int
	DltTemplateId int64
	TemplateText  string
	Mobile        string
	CommId        string
}
