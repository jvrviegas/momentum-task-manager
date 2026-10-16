package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrUnreachable reports that the server gave no answer of its own: the
// connection failed, an attempt timed out, or a gateway answered 502, 503,
// or 504. Errors that wrap it say which.
var ErrUnreachable = errors.New("cannot reach the Momentum server")

const (
	defaultAttempts       = 3
	defaultAttemptTimeout = 10 * time.Second
	defaultRetryDelay     = 500 * time.Millisecond
)

// Client calls a Momentum server for one signed-in device. It sends each
// mutation with a new idempotency key and, when the server gives no answer
// of its own (ErrUnreachable), tries a request twice more with the same key,
// after half a second and then a second. A failed request answers with an
// *Error.
type Client struct {
	baseURL        string
	token          string
	http           *http.Client
	attempts       int
	attemptTimeout time.Duration
	retryDelay     time.Duration
}

// NewClient returns a client for the server at baseURL, such as
// "https://momentum.example", that signs its requests with token, unless it
// is empty.
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"), token: token, http: http.DefaultClient,
		attempts: defaultAttempts, attemptTimeout: defaultAttemptTimeout, retryDelay: defaultRetryDelay,
	}
}

// Version answers RouteVersion.
func (c *Client) Version(ctx context.Context) (Version, error) {
	return send[Version](ctx, c, call{route: RouteVersion})
}

// Session answers RouteSession.
func (c *Client) Session(ctx context.Context) (Session, error) {
	return send[Session](ctx, c, call{route: RouteSession})
}

// StartDeviceSignIn answers RouteStartDeviceSignIn.
func (c *Client) StartDeviceSignIn(ctx context.Context) (DeviceSignIn, error) {
	return send[DeviceSignIn](ctx, c, call{route: RouteStartDeviceSignIn})
}

// PollDeviceSignIn answers RoutePollDeviceSignIn. Until the person signs in,
// it fails with an *Error whose code is one of RFC 8628's.
func (c *Client) PollDeviceSignIn(ctx context.Context, deviceCode string) (Token, error) {
	return send[Token](ctx, c, call{route: RoutePollDeviceSignIn, body: PollDeviceSignInRequest{DeviceCode: deviceCode}})
}

// SignOut revokes the client's token (RouteSignOut).
func (c *Client) SignOut(ctx context.Context) error {
	_, err := send[struct{}](ctx, c, call{route: RouteSignOut})
	return err
}

// SetTimeZone answers RouteSetTimeZone.
func (c *Client) SetTimeZone(ctx context.Context, name string) (TimeZone, error) {
	return send[TimeZone](ctx, c, call{route: RouteSetTimeZone, body: TimeZone{TimeZone: name}})
}

// ListTasks answers RouteListTasks. endedAfter applies to StatusCompleted
// only.
func (c *Client) ListTasks(ctx context.Context, status string, endedAfter time.Time) ([]Task, error) {
	query := url.Values{QueryStatus: {status}}
	if status == StatusCompleted {
		query.Set(QueryEndedAfter, endedAfter.UTC().Format(time.RFC3339))
	}
	list, err := send[TaskList](ctx, c, call{route: RouteListTasks, query: query})
	return list.Tasks, err
}

// CreateTask answers RouteCreateTask.
func (c *Client) CreateTask(ctx context.Context, input CreateTaskRequest) (Task, error) {
	return send[Task](ctx, c, call{route: RouteCreateTask, body: input, mutation: true})
}

// ModifyTask answers RouteModifyTask.
func (c *Client) ModifyTask(ctx context.Context, uuid string, input ModifyTaskRequest) (Task, error) {
	return send[Task](ctx, c, call{route: RouteModifyTask, uuid: uuid, body: input, mutation: true})
}

// DeleteTask answers RouteDeleteTask.
func (c *Client) DeleteTask(ctx context.Context, uuid string) (Task, error) {
	return send[Task](ctx, c, call{route: RouteDeleteTask, uuid: uuid, mutation: true})
}

// CompleteTask answers RouteCompleteTask.
func (c *Client) CompleteTask(ctx context.Context, uuid string) (Task, error) {
	return send[Task](ctx, c, call{route: RouteCompleteTask, uuid: uuid, mutation: true})
}

// StartTask answers RouteStartTask.
func (c *Client) StartTask(ctx context.Context, uuid string) (Task, error) {
	return send[Task](ctx, c, call{route: RouteStartTask, uuid: uuid, mutation: true})
}

// StopTask answers RouteStopTask.
func (c *Client) StopTask(ctx context.Context, uuid string) (Task, error) {
	return send[Task](ctx, c, call{route: RouteStopTask, uuid: uuid, mutation: true})
}

// StopRecurrence answers RouteStopRecurrence.
func (c *Client) StopRecurrence(ctx context.Context, uuid string) (Task, error) {
	return send[Task](ctx, c, call{route: RouteStopRecurrence, uuid: uuid, mutation: true})
}

// Undo answers RouteUndo.
func (c *Client) Undo(ctx context.Context) (Task, error) {
	return send[Task](ctx, c, call{route: RouteUndo, mutation: true})
}

// Projects answers RouteListProjects.
func (c *Client) Projects(ctx context.Context) ([]string, error) {
	list, err := send[ProjectList](ctx, c, call{route: RouteListProjects})
	return list.Projects, err
}

// Tags answers RouteListTags.
func (c *Client) Tags(ctx context.Context) ([]string, error) {
	list, err := send[TagList](ctx, c, call{route: RouteListTags})
	return list.Tags, err
}

// PreviewProjectMigration answers RoutePreviewProjectMigration.
func (c *Client) PreviewProjectMigration(ctx context.Context, input ProjectMigration) (ProjectMigrationPreview, error) {
	return send[ProjectMigrationPreview](ctx, c, call{route: RoutePreviewProjectMigration, body: input})
}

// ApplyProjectMigration answers RouteApplyProjectMigration.
func (c *Client) ApplyProjectMigration(ctx context.Context, input ApplyProjectMigrationRequest) (ProjectMigrationResult, error) {
	return send[ProjectMigrationResult](ctx, c, call{route: RouteApplyProjectMigration, body: input, mutation: true})
}

// call is one request: a route, the task in its path, its query, and a body
// to send as JSON. A mutation carries an idempotency key.
type call struct {
	route    string
	uuid     string
	query    url.Values
	body     any
	mutation bool
}

func send[T any](ctx context.Context, c *Client, request call) (T, error) {
	var answer T
	err := c.do(ctx, request, &answer)
	return answer, err
}

// do sends request until the server answers it or the attempts run out, and
// decodes a successful answer into answer.
func (c *Client) do(ctx context.Context, request call, answer any) error {
	method, path, _ := strings.Cut(request.route, " ")
	target := c.baseURL + strings.Replace(path, "{"+PathUUID+"}", url.PathEscape(request.uuid), 1)
	if len(request.query) > 0 {
		target += "?" + request.query.Encode()
	}
	var body []byte
	if request.body != nil {
		var err error
		if body, err = json.Marshal(request.body); err != nil {
			return err
		}
	}
	var key string
	if request.mutation {
		key = rand.Text()
	}
	var err error
	for attempt := range c.attempts {
		if attempt > 0 {
			timer := time.NewTimer(c.retryDelay << (attempt - 1))
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
		if err = c.attempt(ctx, method, target, key, body, answer); !errors.Is(err, ErrUnreachable) || ctx.Err() != nil {
			return err
		}
	}
	return err
}

// attempt sends one request and reads its answer within the attempt timeout.
func (c *Client) attempt(ctx context.Context, method, target, key string, body []byte, answer any) error {
	attemptCtx, cancel := context.WithTimeout(ctx, c.attemptTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(attemptCtx, method, target, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		request.Header.Set(IdempotencyKeyHeader, key)
	}
	response, err := c.http.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return err
		}
		return fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	switch {
	case err != nil && ctx.Err() != nil:
		return err
	case err != nil:
		return fmt.Errorf("%w: %w", ErrUnreachable, err)
	case response.StatusCode == http.StatusBadGateway || response.StatusCode == http.StatusServiceUnavailable ||
		response.StatusCode == http.StatusGatewayTimeout:
		return fmt.Errorf("%w: the server answered %s", ErrUnreachable, response.Status)
	case response.StatusCode >= http.StatusMultipleChoices:
		var failure Error
		if json.Unmarshal(data, &failure) != nil || failure.Code == "" {
			return &Error{Code: CodeInternal, Message: "the server answered " + response.Status}
		}
		return &failure
	case response.StatusCode == http.StatusNoContent:
		return nil
	}
	if err := json.Unmarshal(data, answer); err != nil {
		return fmt.Errorf("decode the server's answer: %w", err)
	}
	return nil
}
