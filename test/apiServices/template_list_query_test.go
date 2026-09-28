package apiServices_test

import (
	"testing"

	apiServices "github.com/wecredit/communication-sdk/internal/services/apiServices"
)

func TestTemplateListOrderClause(t *testing.T) {
	unrestricted := apiServices.TemplateListOrderClause(true)
	wantUnrestricted := "Client ASC, Process ASC, TemplateName ASC, Channel ASC, COALESCE(UpdatedOn, CreatedOn) DESC, Id DESC"
	if unrestricted != wantUnrestricted {
		t.Fatalf("unrestricted order = %q, want %q", unrestricted, wantUnrestricted)
	}

	scoped := apiServices.TemplateListOrderClause(false)
	wantScoped := "Process ASC, Channel ASC, TemplateName ASC, COALESCE(UpdatedOn, CreatedOn) DESC, Id DESC"
	if scoped != wantScoped {
		t.Fatalf("scoped order = %q, want %q", scoped, wantScoped)
	}
}

func TestEscapeLikePattern(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "otp", want: "otp"},
		{name: "percent", in: "100%", want: "100!%"},
		{name: "underscore", in: "a_b", want: "a!_b"},
		{name: "bang", in: "wow!", want: "wow!!"},
		{name: "all specials", in: "%_!", want: "!%!_!!"},
		{name: "mixed", in: "pre%mid_end!", want: "pre!%mid!_end!!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := apiServices.EscapeLikePattern(tt.in)
			if got != tt.want {
				t.Fatalf("EscapeLikePattern(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestEscapeLikePatternDoesNotTreatPercentAsMatchAllWhenEscaped(t *testing.T) {
	escaped := apiServices.EscapeLikePattern("%")
	if escaped != "!%" {
		t.Fatalf("escaped percent = %q, want %q", escaped, "!%")
	}
	pattern := "%" + escaped + "%"
	wantPattern := "%!%%"
	if pattern != wantPattern {
		t.Fatalf("bound pattern = %q, want %q", pattern, wantPattern)
	}
}
