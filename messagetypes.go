// Package messaging provides the logic and data structures that the services
// will need to communicate with each other over AMQP (as implemented
// by RabbitMQ).
package messaging

import "github.com/cyverse-de/model/v10"

// JobRequest is a generic request type for job related requests.
type JobRequest struct {
	Job     *model.Job
	Command Command
	Message string
	Version int
}

// NewLaunchRequest returns a *JobRequest that has been constructed to be a
// launch request for the provided job InvocationID.
func NewLaunchRequest(j *model.Job) *JobRequest {
	return &JobRequest{
		Job:     j,
		Command: Launch,
		Version: 0,
	}
}

// NewStopRequest returns a *JobRequest that has been constructed to be a
// stop request for a running job.
func NewStopRequest() *StopRequest {
	return &StopRequest{
		Version: 0,
	}
}

// StopRequest contains the information needed to stop a job
type StopRequest struct {
	Reason       string
	Username     string
	Version      int
	InvocationID string
}

// UpdateMessage contains the information needed to broadcast a change in state
// for a job.
type UpdateMessage struct {
	Job     *model.Job
	Version int
	State   JobState
	Message string
	SentOn  string // Should be the milliseconds since the epoch
	Sender  string // Should be the hostname of the box sending the message.
}

// TimeLimitRequest is the message that is sent to road-runner to get it to
// broadcast its current time limit.
type TimeLimitRequest struct {
	InvocationID string
}

// EmailRequest defines the structure of a request to be sent to iplant-email.
type EmailRequest struct {
	TemplateName        string         `json:"template"`
	TemplateValues      map[string]any `json:"values"`
	Subject             string         `json:"subject"`
	ToAddress           string         `json:"to"`
	CourtesyCopyAddress string         `json:"cc,omitempty"`
	FromAddress         string         `json:"from-addr,omitempty"`
	FromName            string         `json:"from-name,omitempty"`
}

// NotificationMessage defines the structure of a notification message sent to
// the Discovery Environment UI.
type NotificationMessage struct {
	Deleted       bool           `json:"deleted"`
	Email         bool           `json:"email"`
	EmailTemplate string         `json:"email_template"`
	Message       map[string]any `json:"message"`
	Payload       any            `json:"payload"`
	Seen          bool           `json:"seen"`
	Subject       string         `json:"subject"`
	Type          string         `json:"type"`
	User          string         `json:"user"`
}

// WrappedNotificationMessage defines a wrapper around a notification message
// sent to the Discovery Environment UI. The wrapper contains an unread message
// count in addition to the message itself.
type WrappedNotificationMessage struct {
	Total   int64                `json:"total"`
	Message *NotificationMessage `json:"message"`
}

// TimeLimitResponse is the message that is sent by road-runner in response
// to a TimeLimitRequest. It contains the current time limit from road-runner.
type TimeLimitResponse struct {
	InvocationID          string
	MillisecondsRemaining int64
}

// TimeLimitDelta is the message that is sent to get road-runner to change its
// time limit. The 'Delta' field contains a string in Go's Duration string
// format. More info on the format is available here:
// https://golang.org/pkg/time/#ParseDuration
type TimeLimitDelta struct {
	InvocationID string
	Delta        string
}
