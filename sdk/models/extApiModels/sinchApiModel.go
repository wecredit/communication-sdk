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
		CardMessage struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Media       struct {
				URL string `json:"url"`
			} `json:"media_message"`
			Height  string `json:"height"`
			Choices []struct {
				UrlMessage struct {
					Url string `json:"url"`
				} `json:"url_message"`
				PostbackData string `json:"postback_data"`
			} `json:"choices"`
		} `json:"card_message"`
	} `json:"message"`
}
