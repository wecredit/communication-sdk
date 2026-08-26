package templatevars_test

import (
	"testing"

	"github.com/wecredit/communication-sdk/internal/channels/sms/templatevars"
	extapimodels "github.com/wecredit/communication-sdk/internal/models/extApiModels"
)

func TestApplyTemplateVariablesReplacesPaymentLink(t *testing.T) {
	text, err := templatevars.ApplyTemplateVariables(extapimodels.SmsRequestBody{
		TemplateText:      "Apply here {#var#} WeCredit",
		TemplateVariables: "PaymentLink",
		PaymentLink:       "https://example.com/apply",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "Apply here https://example.com/apply WeCredit" {
		t.Fatalf("text = %q", text)
	}
}

func TestApplyTemplateVariablesMapsDynamicNamedValuesOntoVarPlaceholders(t *testing.T) {
	text, err := templatevars.ApplyTemplateVariables(extapimodels.SmsRequestBody{
		TemplateText:           "Chhoti zaroorat, sahi support. Salaried users ke liye Credittplus se Rs 35000 tak loan. Abhi explore karein {#var#} WeCredit",
		TemplateVariables:      "urg",
		TemplateVariableValues: "https://loan.credittnow.com/",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "Chhoti zaroorat, sahi support. Salaried users ke liye Credittplus se Rs 35000 tak loan. Abhi explore karein https://loan.credittnow.com/ WeCredit"
	if text != want {
		t.Fatalf("text = %q, want %q", text, want)
	}
}

func TestApplyTemplateVariablesAllowsBlankDynamicUrg(t *testing.T) {
	text, err := templatevars.ApplyTemplateVariables(extapimodels.SmsRequestBody{
		TemplateText:      "Aaj hi explore karein {#var#} WeCredit",
		TemplateVariables: "urg",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "Aaj hi explore karein  WeCredit" {
		t.Fatalf("text = %q", text)
	}
}

func TestApplyTemplateVariablesMapsMultipleDynamicValuesInOrder(t *testing.T) {
	text, err := templatevars.ApplyTemplateVariables(extapimodels.SmsRequestBody{
		TemplateText:           "Hi {#var#} open {#var#}",
		TemplateVariables:      "args,urg",
		TemplateVariableValues: "Asha, https://loan.credittnow.com/",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "Hi Asha open https://loan.credittnow.com/" {
		t.Fatalf("text = %q", text)
	}
}

func TestApplyTemplateVariablesUsesPositionalCsv(t *testing.T) {
	text, err := templatevars.ApplyTemplateVariables(extapimodels.SmsRequestBody{
		TemplateText:           "Hi {#var#} pay {#var#}",
		TemplateVariables:      "CustomerName,PaymentLink",
		TemplateVariableValues: "Asha, https://pay.example/1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "Hi Asha pay https://pay.example/1" {
		t.Fatalf("text = %q", text)
	}
}

func TestApplyTemplateVariablesNamedFieldsWinWhenCsvEmpty(t *testing.T) {
	text, err := templatevars.ApplyTemplateVariables(extapimodels.SmsRequestBody{
		TemplateText:      "Hi {#var#}",
		TemplateVariables: "CustomerName",
		CustomerName:      "ZapCash User",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "Hi ZapCash User" {
		t.Fatalf("text = %q", text)
	}
}

func TestApplyTemplateVariablesRejectsPlaceholderCountMismatch(t *testing.T) {
	_, err := templatevars.ApplyTemplateVariables(extapimodels.SmsRequestBody{
		TemplateText:      "Hi {#var#} then {#var#}",
		TemplateVariables: "CustomerName",
		CustomerName:      "Asha",
	})
	if err == nil {
		t.Fatal("expected placeholder/key count mismatch")
	}
}

func TestApplyTemplateVariablesKeepsLoanIdAndApplicationNumberDistinct(t *testing.T) {
	text, err := templatevars.ApplyTemplateVariables(extapimodels.SmsRequestBody{
		TemplateText:           "loan {#var#} app {#var#}",
		TemplateVariables:      "LoanId,ApplicationNumber",
		TemplateVariableValues: "L-1,A-9",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "loan L-1 app A-9" {
		t.Fatalf("text = %q", text)
	}
}

func TestApplyTemplateVariablesNoOpWithoutPlaceholders(t *testing.T) {
	text, err := templatevars.ApplyTemplateVariables(extapimodels.SmsRequestBody{
		TemplateText: "plain text",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "plain text" {
		t.Fatalf("text = %q", text)
	}
}
