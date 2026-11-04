package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// ---------------------------------------------------------------------------
// Application API v2: https://api.nexmo.com/v2/applications
// ---------------------------------------------------------------------------

type wireWebhook struct {
	Address        string `json:"address"`
	HTTPMethod     string `json:"http_method,omitempty"`
	ConnectTimeout *int   `json:"connect_timeout,omitempty"`
	SocketTimeout  *int   `json:"socket_timeout,omitempty"`
}

type wireVoice struct {
	Webhooks *struct {
		AnswerURL         *wireWebhook `json:"answer_url,omitempty"`
		FallbackAnswerURL *wireWebhook `json:"fallback_answer_url,omitempty"`
		EventURL          *wireWebhook `json:"event_url,omitempty"`
	} `json:"webhooks,omitempty"`
	SignedCallbacks    *bool   `json:"signed_callbacks,omitempty"`
	ConversationsTTL   *int    `json:"conversations_ttl,omitempty"`
	LegPersistenceTime *int    `json:"leg_persistence_time,omitempty"`
	Region             *string `json:"region,omitempty"`
}

type wireMessages struct {
	Webhooks *struct {
		InboundURL *wireWebhook `json:"inbound_url,omitempty"`
		StatusURL  *wireWebhook `json:"status_url,omitempty"`
	} `json:"webhooks,omitempty"`
	Version                  *string `json:"version,omitempty"`
	AuthenticateInboundMedia *bool   `json:"authenticate_inbound_media,omitempty"`
}

type wireRTC struct {
	Webhooks *struct {
		EventURL *wireWebhook `json:"event_url,omitempty"`
	} `json:"webhooks,omitempty"`
	SignedCallbacks *bool `json:"signed_callbacks,omitempty"`
}

type wireVerify struct {
	Webhooks *struct {
		StatusURL *wireWebhook `json:"status_url,omitempty"`
	} `json:"webhooks,omitempty"`
	Version *string `json:"version,omitempty"`
}

type wireMeetings struct {
	Webhooks *struct {
		RecordingChanged *wireWebhook `json:"recording_changed,omitempty"`
		RoomChanged      *wireWebhook `json:"room_changed,omitempty"`
		SessionChanged   *wireWebhook `json:"session_changed,omitempty"`
	} `json:"webhooks,omitempty"`
}

type wireCapabilities struct {
	Voice    *wireVoice    `json:"voice,omitempty"`
	Messages *wireMessages `json:"messages,omitempty"`
	RTC      *wireRTC      `json:"rtc,omitempty"`
	// VBC is always an empty object; a non-nil pointer marshals to {}.
	VBC      *struct{}     `json:"vbc,omitempty"`
	Verify   *wireVerify   `json:"verify,omitempty"`
	Meetings *wireMeetings `json:"meetings,omitempty"`

	// Extra holds capabilities this provider does not model (for example video
	// and network_apis). They are captured on read and replayed on every PUT so a
	// full-replace update never strips a capability configured outside Pulumi.
	Extra map[string]json.RawMessage `json:"-"`
}

type wireCapabilitiesAlias wireCapabilities

// UnmarshalJSON decodes the modelled capabilities and keeps every other key in Extra.
func (c *wireCapabilities) UnmarshalJSON(b []byte) error {
	var known wireCapabilitiesAlias
	if err := json.Unmarshal(b, &known); err != nil {
		return err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		return err
	}
	for _, k := range []string{"voice", "messages", "rtc", "vbc", "verify", "meetings"} {
		delete(all, k)
	}
	*c = wireCapabilities(known)
	if len(all) > 0 {
		c.Extra = all
	}
	return nil
}

// MarshalJSON encodes the modelled capabilities plus any preserved extras.
func (c wireCapabilities) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(wireCapabilitiesAlias(c))
	if err != nil || len(c.Extra) == 0 {
		return b, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	for k, v := range c.Extra {
		if _, ok := m[k]; !ok {
			m[k] = v
		}
	}
	return json.Marshal(m)
}

type wirePrivacy struct {
	ImproveAI *bool `json:"improve_ai,omitempty"`
}

type wireKeys struct {
	PublicKey  string `json:"public_key,omitempty"`
	PrivateKey string `json:"private_key,omitempty"`
}

// wireApplication is both the request body (create/update) and the response.
type wireApplication struct {
	ID           string            `json:"id,omitempty"`
	Name         string            `json:"name"`
	Capabilities *wireCapabilities `json:"capabilities,omitempty"`
	Privacy      *wirePrivacy      `json:"privacy,omitempty"`
	Keys         *wireKeys         `json:"keys,omitempty"`
}

type wireApplicationList struct {
	Page       int `json:"page"`
	TotalPages int `json:"total_pages"`
	Embedded   struct {
		Applications []wireApplication `json:"applications"`
	} `json:"_embedded"`
}

func (c *Client) CreateApplication(ctx context.Context, app *wireApplication) (*wireApplication, error) {
	var out wireApplication
	if err := c.apiJSON(ctx, http.MethodPost, "/v2/applications", nil, app, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetApplication returns nil, nil when the application does not exist.
func (c *Client) GetApplication(ctx context.Context, id string) (*wireApplication, error) {
	var out wireApplication
	err := c.apiJSON(ctx, http.MethodGet, "/v2/applications/"+url.PathEscape(id), nil, nil, &out)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateApplication replaces the application configuration (HTTP PUT).
func (c *Client) UpdateApplication(ctx context.Context, id string, app *wireApplication) (*wireApplication, error) {
	var out wireApplication
	if err := c.apiJSON(ctx, http.MethodPut, "/v2/applications/"+url.PathEscape(id), nil, app, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteApplication deletes an application; a missing application is not an error.
func (c *Client) DeleteApplication(ctx context.Context, id string) error {
	err := c.apiJSON(ctx, http.MethodDelete, "/v2/applications/"+url.PathEscape(id), nil, nil, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

// ListApplications walks every page of /v2/applications.
func (c *Client) ListApplications(ctx context.Context) ([]wireApplication, error) {
	var all []wireApplication
	for page := 1; page <= 1000; page++ {
		q := url.Values{"page_size": {"100"}, "page": {strconv.Itoa(page)}}
		var out wireApplicationList
		if err := c.apiJSON(ctx, http.MethodGet, "/v2/applications", q, nil, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Embedded.Applications...)
		if out.TotalPages == 0 || page >= out.TotalPages {
			break
		}
	}
	return all, nil
}

// ---------------------------------------------------------------------------
// Numbers API: https://rest.nexmo.com/number/* and /account/numbers
// ---------------------------------------------------------------------------

type wireOwnedNumber struct {
	Country               string   `json:"country"`
	MSISDN                string   `json:"msisdn"`
	MoHTTPURL             string   `json:"moHttpUrl"`
	Type                  string   `json:"type"`
	Features              []string `json:"features"`
	MessagesCallbackType  string   `json:"messagesCallbackType"`
	MessagesCallbackValue string   `json:"messagesCallbackValue"`
	VoiceCallbackType     string   `json:"voiceCallbackType"`
	VoiceCallbackValue    string   `json:"voiceCallbackValue"`
	AppID                 string   `json:"app_id"`
	// VoiceStatusCallback is a pointer so an absent field (not reported by the
	// list endpoint) is distinguishable from an explicitly empty one.
	VoiceStatusCallback *string `json:"voiceStatusCallback,omitempty"`
}

type wireOwnedNumbers struct {
	Count   int               `json:"count"`
	Numbers []wireOwnedNumber `json:"numbers"`
}

// numberStatus is the legacy envelope returned by buy/cancel/update.
type numberStatus struct {
	ErrorCode      string `json:"error-code"`
	ErrorCodeLabel string `json:"error-code-label"`
}

func (s *numberStatus) check(op string) error {
	if s.ErrorCode != "200" {
		return &numberStatusError{Op: op, Code: s.ErrorCode, Label: s.ErrorCodeLabel}
	}
	return nil
}

// numberStatusError is a definitive refusal reported by the legacy number API in a
// 2xx response body: Vonage understood the request and did not apply it.
type numberStatusError struct {
	Op, Code, Label string
}

func (e *numberStatusError) Error() string {
	return fmt.Sprintf("vonage: number %s failed: error-code %s (%s)", e.Op, e.Code, e.Label)
}

// FindOwnedNumber returns the owned number with exactly this msisdn, or nil.
func (c *Client) FindOwnedNumber(ctx context.Context, country, msisdn string) (*wireOwnedNumber, error) {
	for index := 1; index <= 100; index++ {
		q := url.Values{
			"country":        {country},
			"pattern":        {msisdn},
			"search_pattern": {"1"},
			"size":           {"100"},
			"index":          {strconv.Itoa(index)},
		}
		var out wireOwnedNumbers
		if err := c.restForm(ctx, http.MethodGet, "/account/numbers", q, nil, &out); err != nil {
			return nil, err
		}
		for i := range out.Numbers {
			if out.Numbers[i].MSISDN == msisdn {
				n := out.Numbers[i]
				return &n, nil
			}
		}
		if out.Count <= index*100 {
			break
		}
	}
	return nil, nil
}

// BuyNumber purchases a number. This is a billed, non-idempotent operation.
func (c *Client) BuyNumber(ctx context.Context, country, msisdn string) error {
	var st numberStatus
	form := url.Values{"country": {country}, "msisdn": {msisdn}}
	if err := c.restForm(ctx, http.MethodPost, "/number/buy", nil, form, &st); err != nil {
		return err
	}
	return st.check("buy")
}

// CancelNumber releases a number permanently.
func (c *Client) CancelNumber(ctx context.Context, country, msisdn string) error {
	var st numberStatus
	form := url.Values{"country": {country}, "msisdn": {msisdn}}
	if err := c.restForm(ctx, http.MethodPost, "/number/cancel", nil, form, &st); err != nil {
		return err
	}
	return st.check("cancel")
}

// numberUpdate carries the form fields of POST /number/update. A nil pointer
// leaves the field out of the request; a pointer to "" sends an empty value.
type numberUpdate struct {
	AppID               *string
	MoHTTPURL           *string
	MoSmppSysType       *string
	VoiceCallbackType   *string
	VoiceCallbackValue  *string
	VoiceStatusCallback *string
}

func (c *Client) UpdateNumber(ctx context.Context, country, msisdn string, u numberUpdate) error {
	form := url.Values{"country": {country}, "msisdn": {msisdn}}
	set := func(k string, v *string) {
		if v != nil {
			form.Set(k, *v)
		}
	}
	set("app_id", u.AppID)
	set("moHttpUrl", u.MoHTTPURL)
	set("moSmppSysType", u.MoSmppSysType)
	set("voiceCallbackType", u.VoiceCallbackType)
	set("voiceCallbackValue", u.VoiceCallbackValue)
	set("voiceStatusCallback", u.VoiceStatusCallback)
	var st numberStatus
	if err := c.restForm(ctx, http.MethodPost, "/number/update", nil, form, &st); err != nil {
		return err
	}
	return st.check("update")
}

// ---------------------------------------------------------------------------
// Subaccounts API: https://api.nexmo.com/accounts/{api_key}/subaccounts
// ---------------------------------------------------------------------------

type wireSubaccount struct {
	APIKey                   string   `json:"api_key"`
	Name                     string   `json:"name"`
	PrimaryAccountAPIKey     string   `json:"primary_account_api_key"`
	UsePrimaryAccountBalance bool     `json:"use_primary_account_balance"`
	CreatedAt                string   `json:"created_at"`
	Suspended                bool     `json:"suspended"`
	Balance                  *float64 `json:"balance"`
	CreditLimit              *float64 `json:"credit_limit"`
	// Secret is only present in the create response.
	Secret string `json:"secret,omitempty"`
}

type subaccountCreate struct {
	Name                     string  `json:"name"`
	Secret                   *string `json:"secret,omitempty"`
	UsePrimaryAccountBalance *bool   `json:"use_primary_account_balance,omitempty"`
}

type subaccountPatch struct {
	Name                     *string `json:"name,omitempty"`
	Suspended                *bool   `json:"suspended,omitempty"`
	UsePrimaryAccountBalance *bool   `json:"use_primary_account_balance,omitempty"`
}

func (c *Client) subPath(suffix string) string {
	return "/accounts/" + url.PathEscape(c.apiKey) + "/subaccounts" + suffix
}

func (c *Client) CreateSubaccount(ctx context.Context, in subaccountCreate) (*wireSubaccount, error) {
	var out wireSubaccount
	if err := c.apiJSON(ctx, http.MethodPost, c.subPath(""), nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetSubaccount returns nil, nil when the subaccount does not exist.
func (c *Client) GetSubaccount(ctx context.Context, key string) (*wireSubaccount, error) {
	var out wireSubaccount
	err := c.apiJSON(ctx, http.MethodGet, c.subPath("/"+url.PathEscape(key)), nil, nil, &out)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PatchSubaccount(ctx context.Context, key string, in subaccountPatch) (*wireSubaccount, error) {
	var out wireSubaccount
	if err := c.apiJSON(ctx, http.MethodPatch, c.subPath("/"+url.PathEscape(key)), nil, in, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ---------------------------------------------------------------------------
// API secrets: https://api.nexmo.com/accounts/{api_key}/secrets
// ---------------------------------------------------------------------------

type wireSecret struct {
	ID        string `json:"id"`
	CreatedAt string `json:"created_at"`
}

func secretsPath(accountKey, suffix string) string {
	return "/accounts/" + url.PathEscape(accountKey) + "/secrets" + suffix
}

func (c *Client) CreateSecret(ctx context.Context, accountKey, secret string) (*wireSecret, error) {
	var out wireSecret
	body := map[string]string{"secret": secret}
	if err := c.apiJSON(ctx, http.MethodPost, secretsPath(accountKey, ""), nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type wireSecretList struct {
	Embedded struct {
		Secrets []wireSecret `json:"secrets"`
	} `json:"_embedded"`
}

// ListSecrets returns the secrets currently held by an account.
func (c *Client) ListSecrets(ctx context.Context, accountKey string) ([]wireSecret, error) {
	var out wireSecretList
	if err := c.apiJSON(ctx, http.MethodGet, secretsPath(accountKey, ""), nil, nil, &out); err != nil {
		return nil, err
	}
	return out.Embedded.Secrets, nil
}

// GetSecret returns nil, nil when the secret does not exist.
func (c *Client) GetSecret(ctx context.Context, accountKey, id string) (*wireSecret, error) {
	var out wireSecret
	err := c.apiJSON(ctx, http.MethodGet, secretsPath(accountKey, "/"+url.PathEscape(id)), nil, nil, &out)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// RevokeSecret revokes an API secret; a missing secret is not an error.
func (c *Client) RevokeSecret(ctx context.Context, accountKey, id string) error {
	err := c.apiJSON(ctx, http.MethodDelete, secretsPath(accountKey, "/"+url.PathEscape(id)), nil, nil, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

// validAPISecret mirrors the documented API secret policy: 8-25 characters
// with at least one lowercase letter, one uppercase letter and one digit.
func validAPISecret(s string) bool {
	if len(s) < 8 || len(s) > 25 {
		return false
	}
	var lower, upper, digit bool
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= '0' && r <= '9':
			digit = true
		}
	}
	return lower && upper && digit
}
