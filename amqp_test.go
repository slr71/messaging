package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/cyverse-de/model/v10"
	amqp "github.com/rabbitmq/amqp091-go"
)

// ---------------------------------------------------------------------------
// Integration-test helpers (require RUN_INTEGRATION_TESTS + live RabbitMQ)
// ---------------------------------------------------------------------------

var client *Client

func GetClient(t *testing.T) *Client {
	var err error
	if client != nil {
		return client
	}
	client, err = NewClient(uri(), false)
	if err != nil {
		t.Error(err)
	}
	_ = client.SetupPublishing(exchange())
	go func() { client.Listen() }()
	return client
}

func shouldrun() bool {
	sh := os.Getenv("RUN_INTEGRATION_TESTS") != ""
	fmt.Printf("Running integration tests: %t\n", sh)
	return sh
}

func uri() string {
	uri := os.Getenv("INTEGRATION_TEST_AMQP_URI")
	if uri == "" {
		uri = "amqp://guest:guest@rabbit:5672/%2fde"
	}
	return uri
}

func exchange() string {
	return "de"
}

func exchangeType() string {
	return "topic"
}

// ---------------------------------------------------------------------------
// Unit-test helpers (fake AMQP server, frame helpers, silenceLoggers) live
// in fakeamqp_test.go.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Unit tests — constants and message types
// ---------------------------------------------------------------------------

func TestConstants(t *testing.T) {
	expected := 0
	actual := int(Launch)
	if actual != expected {
		t.Errorf("Launch was %d instead of %d", actual, expected)
	}
	expected = 1
	actual = int(Stop)
	if actual != expected {
		t.Errorf("Stop was %d instead of %d", actual, expected)
	}
	expected = 0
	actual = int(Success)
	if actual != expected {
		t.Errorf("Success was %d instead of %d", actual, expected)
	}
}

func TestNewStopRequest(t *testing.T) {
	actual := NewStopRequest()
	expected := &StopRequest{Version: 0}
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("NewStopRequest returned:\n%#v\n\tinstead of:\n%#v", actual, expected)
	}
}
func TestNewLaunchRequest(t *testing.T) {
	job := &model.Job{}
	actual := NewLaunchRequest(job)
	expected := &JobRequest{
		Version: 0,
		Job:     job,
		Command: Launch,
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("NewLaunchRequest returned:\n%#v\n\tinstead of:\n%#v", actual, expected)
	}
}

// ---------------------------------------------------------------------------
// Unit tests — key/queue name formatting
// ---------------------------------------------------------------------------

func TestTimeLimitRequestKey(t *testing.T) {
	invID := "test"
	actual := TimeLimitRequestKey(invID)
	expected := fmt.Sprintf("%s.%s", TimeLimitRequestsKey, invID)
	if actual != expected {
		t.Errorf("TimeLimitRequestKey returned %s instead of %s", actual, expected)
	}
}

func TestTimeLimitRequestQueueName(t *testing.T) {
	invID := "test"
	actual := TimeLimitRequestQueueName(invID)
	expected := fmt.Sprintf("road-runner-%s-tl-request", invID)
	if actual != expected {
		t.Errorf("TimeLimitRequestQueueName returned %s instead of %s", actual, expected)
	}
}

func TestTimeLimitResponsesKey(t *testing.T) {
	invID := "test"
	actual := TimeLimitResponsesKey(invID)
	expected := fmt.Sprintf("%s.%s", TimeLimitResponseKey, invID)
	if actual != expected {
		t.Errorf("TimeLimitResponsesKey returned %s instead of %s", actual, expected)
	}
}

func TestTimeLimitResponsesQueueName(t *testing.T) {
	invID := "test"
	actual := TimeLimitResponsesQueueName(invID)
	expected := fmt.Sprintf("road-runner-%s-tl-response", invID)
	if actual != expected {
		t.Errorf("TimeLimitResponsesQueueName returned %s instead of %s", actual, expected)
	}
}

func TestTimeLimitDeltaRequestKey(t *testing.T) {
	invID := "test"
	actual := TimeLimitDeltaRequestKey(invID)
	expected := fmt.Sprintf("%s.%s", TimeLimitDeltaKey, invID)
	if actual != expected {
		t.Errorf("TimeLimitDeltaRequestKey returned %s instead of %s", actual, expected)
	}
}

func TestStopRequestKey(t *testing.T) {
	invID := "test"
	actual := StopRequestKey(invID)
	expected := fmt.Sprintf("%s.%s", StopsKey, invID)
	if actual != expected {
		t.Errorf("StopRequestKey returned %s instead of %s", actual, expected)
	}
}

func TestTimeLimitDeltaQueueName(t *testing.T) {
	invID := "test"
	actual := TimeLimitDeltaQueueName(invID)
	expected := fmt.Sprintf("road-runner-%s-tl-delta", invID)
	if actual != expected {
		t.Errorf("TimeLimitDeltaQueueName returned %s instead of %s", actual, expected)
	}
}

func TestStopQueueName(t *testing.T) {
	invID := "test"
	actual := StopQueueName(invID)
	expected := fmt.Sprintf("road-runner-%s-stops-request", invID)
	if actual != expected {
		t.Errorf("StopQueueName returneed %s instead of %s", actual, expected)
	}
}

// ---------------------------------------------------------------------------
// Unit tests — reconnect logic (fake AMQP server, no broker needed)
// ---------------------------------------------------------------------------

func TestDoReconnect_UpdatesConnectionInPlace(t *testing.T) {
	silenceLoggers(t)

	uri, cleanup := startFakeAMQPServer(t)
	defer cleanup()

	client, err := NewClient(uri, true)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	originalConnection := client.connection

	// Drain the errors channel in the background so that
	// connection.Close() inside doReconnect doesn't block trying to
	// send on the NotifyClose channel. We keep a reference to the
	// current errors channel since doReconnect replaces it.
	errChan := client.errors
	go func() {
		for range errChan {
		}
	}()

	client.doReconnect()

	if client.connection == originalConnection {
		t.Fatal("doReconnect did not update the client's connection " +
			"field — the caller still holds the old, closed connection")
	}
	if client.connection == nil {
		t.Fatal("doReconnect set connection to nil")
	}

	// Drain the new errors channel for cleanup.
	go func() {
		for range client.errors {
		}
	}()
	client.Close()
}

func TestDoReconnect_PreservesChannels(t *testing.T) {
	silenceLoggers(t)

	uri, cleanup := startFakeAMQPServer(t)
	defer cleanup()

	client, err := NewClient(uri, true)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	origAggChan := client.aggregationChan
	origConsChan := client.consumersChan

	errChan := client.errors
	go func() {
		for range errChan {
		}
	}()

	client.doReconnect()

	if client.aggregationChan != origAggChan {
		t.Fatal("doReconnect replaced aggregationChan — " +
			"goroutines delivering to the old channel would be orphaned")
	}
	if client.consumersChan != origConsChan {
		t.Fatal("doReconnect replaced consumersChan — " +
			"AddConsumer calls on the old channel would be lost")
	}
	if client.connection == nil {
		t.Fatal("connection is nil after doReconnect")
	}

	go func() {
		for range client.errors {
		}
	}()
	client.Close()
}

// ---------------------------------------------------------------------------
// Unit tests — error channel forwarding
// ---------------------------------------------------------------------------

func TestForwardChannelErrors_AllChannelsNotify(t *testing.T) {
	silenceLoggers(t)

	uri, cleanup := startFakeAMQPServer(t)
	defer cleanup()

	client, err := NewClient(uri, false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ch1, err := client.connection.Channel()
	if err != nil {
		t.Fatalf("Channel 1: %v", err)
	}
	ch2, err := client.connection.Channel()
	if err != nil {
		t.Fatalf("Channel 2: %v", err)
	}

	client.forwardChannelErrors(ch1)
	client.forwardChannelErrors(ch2)

	// Inject a synthetic error into client.errors to verify that the
	// channel is still the same one set up in NewClient and has not
	// been overwritten by forwardChannelErrors.
	testErr := &amqp.Error{Code: 999, Reason: "synthetic"}
	go func() {
		client.errors <- testErr
	}()

	select {
	case e := <-client.errors:
		if e == nil || e.Code != 999 {
			t.Fatalf("received unexpected error: %v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("could not send/receive on client.errors — " +
			"it may have been replaced by forwardChannelErrors")
	}

	go func() {
		for range client.errors {
		}
	}()
	client.Close()
}

func TestErrorChannelNotOverwritten(t *testing.T) {
	silenceLoggers(t)

	uri, cleanup := startFakeAMQPServer(t)
	defer cleanup()

	client, err := NewClient(uri, false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	originalErrors := client.errors

	ch1, err := client.connection.Channel()
	if err != nil {
		t.Fatalf("Channel: %v", err)
	}

	client.forwardChannelErrors(ch1)

	if client.errors != originalErrors {
		t.Fatal("forwardChannelErrors replaced client.errors — " +
			"the Listen() loop would no longer receive errors from the " +
			"connection-level NotifyClose registration")
	}

	go func() {
		for range client.errors {
		}
	}()
	client.Close()
}

// ---------------------------------------------------------------------------
// Unit tests — Listen error handling
// ---------------------------------------------------------------------------

func TestListenWithError_ReturnsErrorInsteadOfExit(t *testing.T) {
	silenceLoggers(t)

	uri, cleanup := startFakeAMQPServer(t)
	defer cleanup()

	client, err := NewClient(uri, false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	// Send a synthetic error to simulate a connection drop.
	go func() {
		time.Sleep(50 * time.Millisecond)
		client.errors <- amqp.ErrClosed
	}()

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.ListenWithError()
	}()

	select {
	case listenErr := <-errCh:
		if listenErr == nil {
			t.Fatal("ListenWithError returned nil instead of an error")
		}
		if !errors.Is(listenErr, amqp.ErrClosed) {
			t.Fatalf("ListenWithError returned unexpected error: %v", listenErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ListenWithError did not return within 5 seconds — " +
			"it likely called os.Exit or is blocked forever")
	}
}

func TestListenWithError_ReturnsNilOnGracefulClose(t *testing.T) {
	silenceLoggers(t)

	uri, cleanup := startFakeAMQPServer(t)
	defer cleanup()

	client, err := NewClient(uri, false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// Close the errors channel after a short delay to simulate a
	// graceful shutdown (e.g. caller called Close()).
	go func() {
		time.Sleep(50 * time.Millisecond)
		close(client.errors)
	}()

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.ListenWithError()
	}()

	select {
	case listenErr := <-errCh:
		if listenErr != nil {
			t.Fatalf("ListenWithError returned %v, expected nil on graceful close", listenErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ListenWithError did not return within 5 seconds")
	}
}

func TestListenWithError_ReturnsNilOnNilError(t *testing.T) {
	silenceLoggers(t)

	uri, cleanup := startFakeAMQPServer(t)
	defer cleanup()

	client, err := NewClient(uri, false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	// Send a nil *amqp.Error to simulate the zero-value receive that
	// can happen when a NotifyClose channel is closed during shutdown.
	go func() {
		time.Sleep(50 * time.Millisecond)
		client.errors <- nil
	}()

	errCh := make(chan error, 1)
	go func() {
		errCh <- client.ListenWithError()
	}()

	select {
	case listenErr := <-errCh:
		if listenErr != nil {
			t.Fatalf("ListenWithError returned %v, expected nil for nil AMQP error", listenErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ListenWithError did not return within 5 seconds")
	}
}

// ---------------------------------------------------------------------------
// Integration tests (require RUN_INTEGRATION_TESTS + live RabbitMQ)
// ---------------------------------------------------------------------------

func TestNewClient(t *testing.T) {
	if !shouldrun() {
		return
	}
	actual, err := NewClient(uri(), false)
	if err != nil {
		t.Error(err)
	}
	defer actual.Close()
	expected := uri()
	if actual.uri != expected {
		t.Errorf("Client's uri was %s instead of %s", actual.uri, expected)
	}
}

func runPublishingTest(t *testing.T, queue, key string, publish func(*Client), check func([]byte)) {
	if !shouldrun() {
		return
	}

	actual := make([]byte, 0)
	coord := make(chan int)
	handler := func(_ context.Context, d amqp.Delivery) {
		_ = d.Ack(false)
		actual = d.Body
		coord <- 1
	}

	client := GetClient(t)
	client.AddConsumer(exchange(), exchangeType(), queue, key, handler, 0)

	publish(client)
	<-coord
	check(actual)
}

func TestClient(t *testing.T) {
	queue := "test_queue"
	key := "tests"
	expected := []byte("this is a test")

	publish := func(c *Client) {
		_ = c.Publish(key, expected)
	}

	check := func(actual []byte) {
		if string(actual) != string(expected) {
			t.Errorf("handler received %s instead of %s", actual, expected)
		}
	}

	runPublishingTest(t, queue, key, publish, check)
}

func TestSendTimeLimitRequest(t *testing.T) {
	queue := "test_queue1"
	key := TimeLimitRequestKey("test")

	publish := func(c *Client) {
		_ = client.SendTimeLimitRequest("test")
	}

	check := func(actual []byte) {
		req := &TimeLimitRequest{}
		err := json.Unmarshal(actual, req)
		if err != nil {
			t.Error(err)
		}
		if req.InvocationID != "test" {
			t.Errorf("TimeLimitRequest's InvocationID was %s instead of test", req.InvocationID)
		}
	}

	runPublishingTest(t, queue, key, publish, check)
}

func TestSendTimeLimitResponse(t *testing.T) {
	queue := "test_queue2"
	key := TimeLimitResponsesKey("test")

	publish := func(c *Client) {
		_ = client.SendTimeLimitResponse("test", 0)
	}

	check := func(actual []byte) {
		resp := &TimeLimitResponse{}
		err := json.Unmarshal(actual, resp)
		if err != nil {
			t.Error(err)
		}
		if resp.InvocationID != "test" {
			t.Errorf("TimeLimitRequest's InvocationID was %s instead of test", resp.InvocationID)
		}
	}

	runPublishingTest(t, queue, key, publish, check)
}

func TestSendTimeLimitDelta(t *testing.T) {
	queue := "test_queue3"
	key := TimeLimitDeltaRequestKey("test")

	publish := func(c *Client) {
		_ = client.SendTimeLimitDelta("test", "10s")
	}

	check := func(actual []byte) {
		delta := &TimeLimitDelta{}
		err := json.Unmarshal(actual, delta)
		if err != nil {
			t.Error(err)
		}
		if delta.InvocationID != "test" {
			t.Errorf("TimeLimitDelta's InvocationID was %s instead of test", delta.InvocationID)
		}
		if delta.Delta != "10s" {
			t.Errorf("TimeLimitDelta's Delta was %s instead of 10s", delta.Delta)
		}
	}

	runPublishingTest(t, queue, key, publish, check)
}

func TestSendStopRequest(t *testing.T) {
	queue := "test_queue4"
	invID := "test"
	key := StopRequestKey(invID)

	publish := func(c *Client) {
		_ = client.SendStopRequest(invID, "test_user", "this is a test")
	}

	check := func(actual []byte) {
		req := &StopRequest{}
		if err := json.Unmarshal(actual, req); err != nil {
			t.Error(err)
		}
		if req.Reason != "this is a test" {
			t.Errorf("Reason was '%s' instead of '%s'", req.Reason, "this is a test")
		}
		if req.InvocationID != invID {
			t.Errorf("InvocationID was %s instead of %s", req.InvocationID, invID)
		}
		if req.Username != "test_user" {
			t.Errorf("Username was %s instead of %s", req.Username, "test_user")
		}
	}

	runPublishingTest(t, queue, key, publish, check)
}

func TestPublishJobUpdate(t *testing.T) {
	queue := "test_job_update_queue"
	key := UpdatesKey
	job := &model.Job{}

	expected := &UpdateMessage{
		Job:     job,
		Version: 1,
		State:   RunningState,
		Message: "I have found the answer!",
		Sender:  "Deep Thought",
	}

	publish := func(c *Client) {
		_ = client.PublishJobUpdate(expected)
	}

	check := func(actualBytes []byte) {
		actual := &UpdateMessage{}
		if err := json.Unmarshal(actualBytes, actual); err != nil {
			t.Error(err)
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("acutal job update does not match expected job update: actual = %+v\n", actual)
		}
	}

	runPublishingTest(t, queue, key, publish, check)
}

func TestPublishEmailRequest(t *testing.T) {
	queue := "test_email_request_queue"
	key := EmailRequestPublishingKey

	expected := &EmailRequest{
		TemplateName:        "some_template",
		TemplateValues:      map[string]any{"foo": "bar"},
		Subject:             "Something crazy this way comes!",
		ToAddress:           "somebody@example.org",
		CourtesyCopyAddress: "somebody.else@example.org",
		FromAddress:         "somebody.different@example.org",
		FromName:            "Somebody Different",
	}

	publish := func(c *Client) {
		_ = client.PublishEmailRequest(expected)
	}

	check := func(actualBytes []byte) {
		actual := &EmailRequest{}
		if err := json.Unmarshal(actualBytes, actual); err != nil {
			t.Error(err)
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("actual email request does not match expected email request: actual = %+v\n", actual)
		}
	}

	runPublishingTest(t, queue, key, publish, check)
}

func TestPublishNotificationMessage(t *testing.T) {
	queue := "test_notification_queue"
	user := "nobody"
	key := fmt.Sprintf("notification.%s", user)

	expected := &WrappedNotificationMessage{
		Total: 42,
		Message: &NotificationMessage{
			Deleted:       false,
			Email:         true,
			EmailTemplate: "some_template",
			Message:       map[string]any{"foo": "bar"},
			Payload:       map[string]any{"baz": "quux"},
			Seen:          false,
			Subject:       "Something happened!!!",
			Type:          "idunno",
			User:          user,
		},
	}

	publish := func(c *Client) {
		_ = client.PublishNotificationMessage(expected)
	}

	check := func(actualBytes []byte) {
		actual := &WrappedNotificationMessage{}
		if err := json.Unmarshal(actualBytes, actual); err != nil {
			t.Error(err)
		}
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("actual notification does not match expected notification: actual = '%+v'\n", actual)
		}
	}

	runPublishingTest(t, queue, key, publish, check)
}

func TestCreateQueue(t *testing.T) {
	if !shouldrun() {
		return
	}
	client := GetClient(t)
	actual, err := client.CreateQueue("test_queue5", exchange(), "test_key5", true, false)
	if err != nil {
		t.Error(err)
	}
	if actual == nil {
		t.Error("channel is nil")
	}
	if _, err = actual.QueueDeclarePassive("test_queue5", true, false, false, false, nil); err != nil {
		t.Error(err)
	}
	if err = actual.Close(); err != nil {
		t.Error(err)
	}
}

func TestQueueExists(t *testing.T) {
	if !shouldrun() {
		return
	}
	client := GetClient(t)
	actual, err := client.CreateQueue("test_queue5", exchange(), "test_key5", true, false)
	if err != nil {
		t.Error(err)
	}
	if actual == nil {
		t.Error("channel is nil")
	}
	exists, err := client.QueueExists("test_queue5", true, false)
	if err != nil {
		t.Error(err)
	}
	if !exists {
		t.Error("Queue 'test_queue5' was not found")
	}
	if err = actual.Close(); err != nil {
		t.Error(err)
	}
}

func TestDeleteQueue(t *testing.T) {
	if !shouldrun() {
		return
	}
	client := GetClient(t)
	actual, err := client.CreateQueue("test_queue6", exchange(), "test_key5", true, false)
	if err != nil {
		t.Error(err)
	}
	if actual == nil {
		t.Error("channel is nil")
	}
	exists, err := client.QueueExists("test_queue6", true, false)
	if err != nil {
		t.Error(err)
	}
	if !exists {
		t.Error("Queue 'test_queue6' was not found")
	}

	actual, err = client.CreateQueue("test_queue7", exchange(), "test_key6", true, false)
	if err != nil {
		t.Error(err)
	}
	if actual == nil {
		t.Error("channel is nil")
	}
	exists, err = client.QueueExists("test_queue7", true, false)
	if err != nil {
		t.Error(err)
	}
	if !exists {
		t.Error("Queue 'test_queue7' was not found")
	}

	actual, err = client.CreateQueue("test_queue8", exchange(), "test_key7", true, false)
	if err != nil {
		t.Error(err)
	}
	if actual == nil {
		t.Error("channel is nil")
	}
	exists, err = client.QueueExists("test_queue8", true, false)
	if err != nil {
		t.Error(err)
	}
	if !exists {
		t.Error("Queue 'test_queue8' was not found")
	}

	if err = client.DeleteQueue("test_queue6"); err != nil {
		t.Error(err)
	}
	exists, err = client.QueueExists("test_queue6", true, false)
	if err != nil {
		t.Error(err)
	}
	if exists {
		t.Error("Queue 'test_queue6' was found")
	}

	if err = client.DeleteQueue("test_queue7"); err != nil {
		t.Error(err)
	}
	exists, err = client.QueueExists("test_queue7", true, false)
	if err != nil {
		t.Error(err)
	}
	if exists {
		t.Error("Queue 'test_queue7' was found")
	}

	if err = client.DeleteQueue("test_queue8"); err != nil {
		t.Error(err)
	}
	exists, err = client.QueueExists("test_queue8", true, false)
	if err != nil {
		t.Error(err)
	}
	if exists {
		t.Error("Queue 'test_queue8' was found")
	}

	if err = actual.Close(); err != nil {
		t.Error(err)
	}
}
