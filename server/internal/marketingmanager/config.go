package marketingmanager

import (
	"net/http"
	"os"
	"strconv"
	"strings"
)

// NewFromEnv builds the Service from the environment:
//
//	OPSAPI_BASE_URL             the ring's OpsAPI (shared with the CMS)
//	OPSAPI_MARKETING_API_KEY    a key scoped for opsapi_marketing_* and crm — never the CMS key
//	OPSAPI_MARKETING_NAMESPACE  its namespace (default OPSAPI_NAMESPACE)
//	MARKETING_TEST_RECIPIENTS   comma list of confirmed test addresses
//	MARKETING_LIVE_SEND         "true" lets sends leave a workspace not in test mode (off by default)
//
// Org integrations (credentials.marketing_api_key) work without any of these.
func NewFromEnv(integrations IntegrationLister) *Service {
	ns := strings.TrimSpace(os.Getenv("OPSAPI_MARKETING_NAMESPACE"))
	if ns == "" {
		ns = strings.TrimSpace(os.Getenv("OPSAPI_NAMESPACE"))
	}
	live, _ := strconv.ParseBool(strings.TrimSpace(os.Getenv("MARKETING_LIVE_SEND")))
	return &Service{
		Integrations: integrations,
		Ring: RingConfig{
			BaseURL:        os.Getenv("OPSAPI_BASE_URL"),
			Key:            os.Getenv("OPSAPI_MARKETING_API_KEY"),
			Namespace:      ns,
			TestRecipients: strings.Split(os.Getenv("MARKETING_TEST_RECIPIENTS"), ","),
		},
		AllowLiveSend: live,
		HTTP:          &http.Client{Timeout: defaultTimeout},
	}
}

// RingConfigured reports whether the ring's own workspace is set.
func (s *Service) RingConfigured() bool { return s != nil && s.Ring.complete() }
