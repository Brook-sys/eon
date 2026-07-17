package domain

// ModelBindingFailureDisposition is an authority-free routing consequence of a
// provider attempt. It changes only binding eligibility/retry behavior; it never
// authorizes a capability or mutates canonical knowledge.
type ModelBindingFailureDisposition string

const (
	ModelFailureRetryBinding     ModelBindingFailureDisposition = "RETRY_BINDING"
	ModelFailureTryNextBinding   ModelBindingFailureDisposition = "TRY_NEXT_BINDING"
	ModelFailureCooldownBinding  ModelBindingFailureDisposition = "COOLDOWN_BINDING"
	ModelFailureCooldownProvider ModelBindingFailureDisposition = "COOLDOWN_PROVIDER"
	ModelFailureDisableBinding   ModelBindingFailureDisposition = "DISABLE_BINDING"
	ModelFailureUnavailable      ModelBindingFailureDisposition = "MARK_UNAVAILABLE"
	ModelFailureFailOperation    ModelBindingFailureDisposition = "FAIL_OPERATION"
)

// ModelBindingFailureClass is a bounded, non-secret taxonomy suitable for
// audit events and deterministic recovery decisions.
type ModelBindingFailureClass string

const (
	ModelFailureInvalidRequest ModelBindingFailureClass = "INVALID_REQUEST"
	ModelFailureUnauthorized   ModelBindingFailureClass = "UNAUTHORIZED"
	ModelFailureForbidden      ModelBindingFailureClass = "FORBIDDEN"
	ModelFailureNotFound       ModelBindingFailureClass = "NOT_FOUND"
	ModelFailureRateLimited    ModelBindingFailureClass = "RATE_LIMITED"
	ModelFailureServer         ModelBindingFailureClass = "SERVER"
	ModelFailureTransport      ModelBindingFailureClass = "TRANSPORT"
	ModelFailureUnknown        ModelBindingFailureClass = "UNKNOWN"
)

// ModelBindingFailureDecision is pure policy output. Scope identifies whether
// durable cooldown should be reported against the binding or the provider.
type ModelBindingFailureDecision struct {
	Class       ModelBindingFailureClass       `json:"class"`
	Disposition ModelBindingFailureDisposition `json:"disposition"`
	Scope       string                         `json:"scope"`
	Reason      string                         `json:"reason"`
}

// ClassifyModelBindingFailure maps an adapter-neutral HTTP projection to a
// deterministic fallback/circuit consequence. Provider-wide 429 is selected
// explicitly by the caller for services whose quota is account/global.
func ClassifyModelBindingFailure(status int, retryable, providerWideRateLimit bool) ModelBindingFailureDecision {
	switch status {
	case 400, 409, 413, 422:
		return ModelBindingFailureDecision{ModelFailureInvalidRequest, ModelFailureTryNextBinding, "binding", "request_incompatible"}
	case 401:
		return ModelBindingFailureDecision{ModelFailureUnauthorized, ModelFailureDisableBinding, "binding", "credential_rejected"}
	case 403:
		return ModelBindingFailureDecision{ModelFailureForbidden, ModelFailureDisableBinding, "binding", "access_forbidden"}
	case 404:
		return ModelBindingFailureDecision{ModelFailureNotFound, ModelFailureUnavailable, "binding", "model_unavailable"}
	case 429:
		if providerWideRateLimit {
			return ModelBindingFailureDecision{ModelFailureRateLimited, ModelFailureCooldownProvider, "provider", "provider_rate_limited"}
		}
		return ModelBindingFailureDecision{ModelFailureRateLimited, ModelFailureCooldownBinding, "binding", "binding_rate_limited"}
	}
	if status >= 500 && status <= 599 {
		return ModelBindingFailureDecision{ModelFailureServer, ModelFailureTryNextBinding, "binding", "provider_server_error"}
	}
	if status == 0 && retryable {
		return ModelBindingFailureDecision{ModelFailureTransport, ModelFailureTryNextBinding, "binding", "transport_retryable"}
	}
	if retryable {
		return ModelBindingFailureDecision{ModelFailureUnknown, ModelFailureRetryBinding, "binding", "retryable_provider_error"}
	}
	return ModelBindingFailureDecision{ModelFailureUnknown, ModelFailureFailOperation, "binding", "non_retryable_provider_error"}
}
