package domain

import "testing"

func TestClassifyModelBindingFailure(t *testing.T) {
	cases := []struct {
		Status         int
		Retryable      bool
		ProviderWide   bool
		ExpClass       ModelBindingFailureClass
		ExpDisposition ModelBindingFailureDisposition
		ExpScope       string
	}{
		{400, false, false, ModelFailureInvalidRequest, ModelFailureTryNextBinding, "binding"},
		{401, false, false, ModelFailureUnauthorized, ModelFailureDisableBinding, "binding"},
		{404, false, false, ModelFailureNotFound, ModelFailureUnavailable, "binding"},
		{429, true, false, ModelFailureRateLimited, ModelFailureCooldownBinding, "binding"},
		{429, true, true, ModelFailureRateLimited, ModelFailureCooldownProvider, "provider"},
		{503, true, false, ModelFailureServer, ModelFailureTryNextBinding, "binding"},
		{0, true, false, ModelFailureTransport, ModelFailureTryNextBinding, "binding"},
		{418, true, false, ModelFailureUnknown, ModelFailureRetryBinding, "binding"},
		{418, false, false, ModelFailureUnknown, ModelFailureFailOperation, "binding"},
	}

	for _, c := range cases {
		got := ClassifyModelBindingFailure(c.Status, c.Retryable, c.ProviderWide)
		if got.Class != c.ExpClass || got.Disposition != c.ExpDisposition || got.Scope != c.ExpScope {
			t.Errorf("Classify(%d, %v, %v) = %+v, want %s / %s / %s", c.Status, c.Retryable, c.ProviderWide, got, c.ExpClass, c.ExpDisposition, c.ExpScope)
		}
	}
}
