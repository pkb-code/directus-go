package directus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

// Client keeps a connection to a Directus instance.
type Client struct {
	Collections        *ResourceClient[Collection]
	CustomTranslations *ResourceClient[CustomTranslation]
	Folders            *ResourceClient[Folder]
	Relations          *ResourceClient[RelationDefinition]
	Roles              *ResourceClient[Role]
	Users              *ResourceClient[User]
	Presets            *ResourceClient[Preset]
	Operations         *ResourceClient[Operation]
	Flows              *ResourceClient[Flow]
	Permissions        *ResourceClient[Permission]
	Dashboard          *ResourceClient[Dashboard]
	Panels             *ResourceClient[Panel]

	Fields *clientFields

	instance, token string
	logger          *slog.Logger
	bodyLogger      bool
}

// ClinetOption configures a client when creating it.
type ClientOption func(client *Client)

// WithLogger sets a custom logger for the sent requests and responses received from the server.
func WithLogger(logger *slog.Logger) ClientOption {
	return func(client *Client) {
		client.logger = logger
	}
}

// WithBodyLogger prints the request and response bodies to the logger.
func WithBodyLogger() ClientOption {
	return func(client *Client) {
		client.bodyLogger = true
	}
}

// NewClient creates a new connection to the Directus instance using the static token to authenticate.
func NewClient(instance string, token string, opts ...ClientOption) *Client {
	client := &Client{
		instance: strings.TrimRight(instance, "/"),
		token:    token,
		logger:   slog.New(slog.Default().Handler()),
	}
	for _, opt := range opts {
		opt(client)
	}

	client.Collections = NewResourceClient[Collection](client, "collections")
	client.CustomTranslations = NewResourceClient[CustomTranslation](client, "translations")
	client.Folders = NewResourceClient[Folder](client, "folders")
	client.Relations = NewResourceClient[RelationDefinition](client, "relations")
	client.Roles = NewResourceClient[Role](client, "roles")
	client.Users = NewResourceClient[User](client, "users")
	client.Presets = NewResourceClient[Preset](client, "presets")
	client.Operations = NewResourceClient[Operation](client, "operations")
	client.Flows = NewResourceClient[Flow](client, "flows")
	client.Permissions = NewResourceClient[Permission](client, "permissions")
	client.Dashboard = NewResourceClient[Dashboard](client, "dashboard")
	client.Panels = NewResourceClient[Panel](client, "panels")

	client.Fields = &clientFields{client: client}

	return client
}

func (client *Client) urlf(format string, a ...interface{}) string {
	return fmt.Sprintf("%s%s", client.instance, fmt.Sprintf(format, a...))
}

func (client *Client) sendRequest(req *http.Request, dest interface{}) error {
	client.logger.Debug("directus request", "method", req.Method, "url", req.URL.String())
	if client.bodyLogger && req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return fmt.Errorf("directus: cannot read request body: %v", err)
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		client.logger.Debug(string(body))
	}

	if req.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", client.token))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("directus: request failed: %w", err)
	}
	defer resp.Body.Close()

	client.logger.Debug("directus reply", "status", resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("directus: cannot read response body: %v", err)
	}
	if client.bodyLogger {
		client.logger.Debug(string(body))
	}

	switch {
	case resp.StatusCode == http.StatusOK:
		// Everything is fine.

	case req.Method == http.MethodDelete && resp.StatusCode == http.StatusNoContent:
		// Everything is fine.

	case resp.StatusCode == http.StatusBadRequest:
		var reply errorsReply
		if err := json.Unmarshal(body, &reply); err == nil && len(reply.Errors) > 0 {
			return reply.Errors[0]
		}

	case (req.Method == http.MethodPost || req.Method == http.MethodPatch) && resp.StatusCode == http.StatusNoContent:
		return ErrEmpty

	default:
		return &unexpectedStatusError{
			status: resp.StatusCode,
			url:    req.URL,
		}
	}

	if dest != nil && len(body) > 0 {
		if err := json.Unmarshal(body, dest); err != nil {
			return fmt.Errorf("directus: cannot decode response: %v", err)
		}
	}

	return nil
}

type errorsReply struct {
	Errors []Error `json:"errors"`
}
