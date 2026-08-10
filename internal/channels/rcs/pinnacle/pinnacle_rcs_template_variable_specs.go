package pinnacleRcs

// Pinnacle RCS variable keys are resolved from SDK / consumer payload fields (see resolvePinnacleRcsVariableValue).
// This map defines, per Pinnacle templateId (same value as TemplateDetails.TemplateName for PINNACLE RCS rows):
//   - which keys appear in API "variables" vs "smsVariables"
//   - whether SMS fallback is required for that template
//
// If a templateId is not listed here, RCS keys come from TemplateDetails.TemplateVariables (comma-separated),
// and smsVariables (when IsSmsFallbackRequired is set from DB or SDK) reuse those same keys.
//
// SDK → payload flow:
// User calls sdk.Send(&CommApiRequestBody{ ..., channel:"RCS", customerName:"...", ... }) → SNS/SQS →
// consumer builds CommApiRequestBody → SendRcsByProcess copies fields into extapimodels.RcsRequestBody →
// PopulateRcsFields merges TemplateDetails (TemplateVariables, TemplateCategory, optional SMS fallback flag) →
// HitPinnacleRcsApi builds JSON using specs below + resolvePinnacleRcsVariableValue for each key.
type pinnacleRcsTemplateVarSpec struct {
	RcsKeys               []string
	SmsKeys               []string // when len==0 and IsSmsFallbackRequired, RcsKeys are reused for smsVariables
	IsSmsFallbackRequired bool
}

// pinnacleRCSVariableSpecs keyed by Pinnacle templateId (Mongo-style id).
var pinnacleRCSVariableSpecs = map[string]pinnacleRcsTemplateVarSpec{
	"69f091e43ffe9a03eec459b7": {
		RcsKeys:               []string{"Customer_Name"},
		SmsKeys:               []string{"Customer_Name"},
		IsSmsFallbackRequired: true,
	},
}

func lookupPinnacleRCSVarSpec(templateID string) (pinnacleRcsTemplateVarSpec, bool) {
	if templateID == "" {
		return pinnacleRcsTemplateVarSpec{}, false
	}
	spec, ok := pinnacleRCSVariableSpecs[templateID]
	return spec, ok
}
