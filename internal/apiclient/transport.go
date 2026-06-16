package apiclient

import (
	"context"
	"net/http"
	"strings"

	clientcodec "github.com/arcgolabs/clientx/codec"

	"github.com/lyonbrown4d/orch/pkg/oopsx"
)

func (c *Client) get(ctx context.Context, path string, out any) error {
	return c.request(ctx, http.MethodGet, path, out)
}

func (c *Client) delete(ctx context.Context, path string, out any) error {
	return c.request(ctx, http.MethodDelete, path, out)
}

func (c *Client) request(ctx context.Context, method, path string, out any) error {
	if c == nil || c.hc == nil {
		return oopsx.B("cli", "apiclient").Errorf("nil client")
	}
	resp, err := c.hc.Execute(ctx, c.hc.R(), method, path)
	if err != nil {
		return oopsx.B("cli", "apiclient").Wrapf(err, "%s %s", method, path)
	}
	if !resp.IsStatusSuccess() {
		msg := strings.TrimSpace(string(resp.Bytes()))
		return oopsx.B("cli", "apiclient").Errorf("%s %s: %s: %s", method, path, resp.Status(), msg)
	}
	if err := clientcodec.JSON.Unmarshal(resp.Bytes(), out); err != nil {
		return oopsx.B("cli", "apiclient").Wrapf(err, "%s %s response", method, path)
	}
	return nil
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	if c == nil || c.hc == nil {
		return oopsx.B("cli", "apiclient").Errorf("nil client")
	}
	raw, err := clientcodec.JSON.Marshal(body)
	if err != nil {
		return oopsx.B("cli", "apiclient").Wrapf(err, "POST %s body", path)
	}
	req := c.hc.R().
		SetHeader("Content-Type", "application/json").
		SetBody(raw)
	resp, err := c.hc.Execute(ctx, req, http.MethodPost, path)
	if err != nil {
		return oopsx.B("cli", "apiclient").Wrapf(err, "POST %s", path)
	}
	if !resp.IsStatusSuccess() {
		msg := strings.TrimSpace(string(resp.Bytes()))
		return oopsx.B("cli", "apiclient").Errorf("POST %s: %s: %s", path, resp.Status(), msg)
	}
	if err := clientcodec.JSON.Unmarshal(resp.Bytes(), out); err != nil {
		return oopsx.B("cli", "apiclient").Wrapf(err, "POST %s response", path)
	}
	return nil
}
