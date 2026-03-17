// Package messaging provides the logic and data structures that the services
// will need to communicate with each other over AMQP (as implemented
// by RabbitMQ).
package messaging

// Command tells the receiver of a JobRequest which action to perform
type Command int

// JobState defines a valid state for a job.
type JobState string

// StatusCode defines a valid exit code for a job.
type StatusCode int

const (
	//Launch tells the receiver of a JobRequest to launch the job
	Launch Command = iota

	//Stop tells the receiver of a JobRequest to stop a job
	Stop
)

const (
	//QueuedState is when a job is queued.
	QueuedState JobState = "Queued"

	//SubmittedState is when a job has been submitted.
	SubmittedState JobState = "Submitted"

	//RunningState is when a job is running.
	RunningState JobState = "Running"

	//ImpendingCancellationState is when a job is running but the current step is about
	//to reach its expiration time.
	ImpendingCancellationState JobState = "ImpendingCancellation"

	//SucceededState is when a job has successfully completed the required steps.
	SucceededState JobState = "Completed"

	//FailedState is when a job has failed. Duh.
	FailedState JobState = "Failed"
)

const (
	// Success is the exit code used when the required commands execute correctly.
	Success StatusCode = iota

	// StatusDockerPullFailed is the exit code when a 'docker pull' fails.
	StatusDockerPullFailed

	// StatusDockerCreateFailed is the exit code when a 'docker create' fails.
	StatusDockerCreateFailed

	// StatusInputFailed is the exit code when an input download fails.
	StatusInputFailed

	// StatusStepFailed is the exit code when a step in the job fails.
	StatusStepFailed

	// StatusOutputFailed is the exit code when the output upload fails.
	StatusOutputFailed

	// StatusKilled is the exit code when the job is killed.
	StatusKilled

	// StatusTimeLimit is the exit code when the job is killed due to the time
	// limit being reached.
	StatusTimeLimit

	// StatusBadDuration is the exit code when the job is killed because an
	// unparseable job duration was sent to it.
	StatusBadDuration
)

const (
	//ReindexAllKey is the routing/binding key for full reindex messages.
	ReindexAllKey = "index.all"

	//ReindexTemplatesKey is the routing/binding key for templates reindex messages.
	ReindexTemplatesKey = "index.templates"

	//IncrementalKey is the routing/binding key for incremental updates
	IncrementalKey = "metadata.update"

	//LaunchesKey is the routing/binding key for job launch request messages.
	LaunchesKey = "jobs.launches"

	//UpdatesKey is the routing/binding key for job update messages.
	UpdatesKey = "jobs.updates"

	//StopsKey is the routing/binding key for job stop request messages.
	StopsKey = "jobs.stops"

	//CommandsKey is the routing/binding key for job command messages.
	CommandsKey = "jobs.commands"

	// TimeLimitRequestsKey is the routing/binding key for the job time limit messages.
	TimeLimitRequestsKey = "jobs.timelimits.requests"

	//TimeLimitDeltaKey is the routing/binding key for the job time limit delta messages.
	TimeLimitDeltaKey = "jobs.timelimits.deltas"

	//TimeLimitResponseKey is the routing/binding key for the job time limit
	//response messages.
	TimeLimitResponseKey = "jobs.timelimits.responses"

	// EmailRequestPublishingKey is the routing/binding key for AMQP request messages
	// destined for iplant-email.
	EmailRequestPublishingKey = "email.requests"
)
