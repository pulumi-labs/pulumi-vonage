package provider

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// ---------------------------------------------------------------------------
// Verify v2 templates: https://api.nexmo.com/v2/verify/templates
// Shapes follow the Vonage Node SDK (packages/verify2). The SDK authenticates
// these calls with a JWT; the Vonage OpenAPI definition is reported to also allow
// Basic auth, which is what this provider sends (unverified against a live account).
// ---------------------------------------------------------------------------

type wireTemplate struct {
	TemplateID string `json:"template_id"`
	Name       string `json:"name"`
	IsDefault  bool   `json:"is_default"`
}

type wireTemplateList struct {
	Page       int `json:"page"`
	TotalPages int `json:"total_pages"`
	Embedded   struct {
		Templates []wireTemplate `json:"templates"`
	} `json:"_embedded"`
}

type wireFragment struct {
	TemplateFragmentID string `json:"template_fragment_id"`
	Channel            string `json:"channel"`
	Locale             string `json:"locale"`
	Text               string `json:"text"`
	DateUpdated        string `json:"date_updated"`
	DateCreated        string `json:"date_created"`
}

type wireFragmentList struct {
	Page       int `json:"page"`
	TotalPages int `json:"total_pages"`
	Embedded   struct {
		Fragments []wireFragment `json:"template_fragments"`
	} `json:"_embedded"`
}

func templatesPath(suffix string) string { return "/v2/verify/templates" + suffix }

func fragmentsPath(templateID, suffix string) string {
	return templatesPath("/"+url.PathEscape(templateID)) + "/template_fragments" + suffix
}

func (c *Client) CreateTemplate(ctx context.Context, name string) (*wireTemplate, error) {
	var out wireTemplate
	if err := c.apiJSON(ctx, http.MethodPost, templatesPath(""), nil, map[string]string{"name": name}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTemplate returns nil, nil when the template does not exist.
func (c *Client) GetTemplate(ctx context.Context, id string) (*wireTemplate, error) {
	var out wireTemplate
	err := c.apiJSON(ctx, http.MethodGet, templatesPath("/"+url.PathEscape(id)), nil, nil, &out)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PatchTemplate(ctx context.Context, id, name string, isDefault bool) (*wireTemplate, error) {
	var out wireTemplate
	body := map[string]any{"name": name, "is_default": isDefault}
	if err := c.apiJSON(ctx, http.MethodPatch, templatesPath("/"+url.PathEscape(id)), nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteTemplate deletes a template; a missing template is not an error.
func (c *Client) DeleteTemplate(ctx context.Context, id string) error {
	err := c.apiJSON(ctx, http.MethodDelete, templatesPath("/"+url.PathEscape(id)), nil, nil, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

// ListTemplates walks every page of /v2/verify/templates.
func (c *Client) ListTemplates(ctx context.Context) ([]wireTemplate, error) {
	var all []wireTemplate
	for page := 1; page <= 1000; page++ {
		q := url.Values{"page_size": {"100"}, "page": {strconv.Itoa(page)}}
		var out wireTemplateList
		if err := c.apiJSON(ctx, http.MethodGet, templatesPath(""), q, nil, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Embedded.Templates...)
		if out.TotalPages == 0 || page >= out.TotalPages {
			break
		}
	}
	return all, nil
}

func (c *Client) CreateFragment(ctx context.Context, templateID, channel, locale, text string) (*wireFragment, error) {
	var out wireFragment
	body := map[string]string{"channel": channel, "locale": locale, "text": text}
	if err := c.apiJSON(ctx, http.MethodPost, fragmentsPath(templateID, ""), nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetFragment returns nil, nil when the template or the fragment does not exist.
func (c *Client) GetFragment(ctx context.Context, templateID, id string) (*wireFragment, error) {
	var out wireFragment
	err := c.apiJSON(ctx, http.MethodGet, fragmentsPath(templateID, "/"+url.PathEscape(id)), nil, nil, &out)
	if isNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PatchFragment(ctx context.Context, templateID, id, text string) error {
	return c.apiJSON(ctx, http.MethodPatch, fragmentsPath(templateID, "/"+url.PathEscape(id)), nil, map[string]string{"text": text}, nil)
}

// DeleteFragment deletes a fragment; a missing fragment is not an error.
func (c *Client) DeleteFragment(ctx context.Context, templateID, id string) error {
	err := c.apiJSON(ctx, http.MethodDelete, fragmentsPath(templateID, "/"+url.PathEscape(id)), nil, nil, nil)
	if isNotFound(err) {
		return nil
	}
	return err
}

// ListFragments walks every page of a template's fragments.
func (c *Client) ListFragments(ctx context.Context, templateID string) ([]wireFragment, error) {
	var all []wireFragment
	for page := 1; page <= 1000; page++ {
		q := url.Values{"page_size": {"100"}, "page": {strconv.Itoa(page)}}
		var out wireFragmentList
		if err := c.apiJSON(ctx, http.MethodGet, fragmentsPath(templateID, ""), q, nil, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Embedded.Fragments...)
		if out.TotalPages == 0 || page >= out.TotalPages {
			break
		}
	}
	return all, nil
}
