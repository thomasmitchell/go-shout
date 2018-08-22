package shout

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

//Client has functions that handle interactions with SHOUT!
type Client struct {
	//Target is the URL that this client will hit with requests
	Target string
	//HTTPClient is the net/http client that will be used to send requests.
	// If left nil, http.DefaultClient will be used instead
	HTTPClient *http.Client
}

//TopicState is an enumeration type that describes the state that a topic
// is in. Possible values are TopicWorking, TopicFixed, or TopicBroken
type TopicState int

const (
	//TopicWorking denotes that the topic has been OK for multiple updates
	TopicWorking TopicState = iota
	//TopicFixed denotes that the topic has transitioned to OK from not OK in the
	// most recent update
	TopicFixed
	//TopicBroken denotes that the topic is NotOK.
	TopicBroken
)

//StateOut is the output of a PostEvent call, containing information about the
// topic that the event was posted for.
type StateOut struct {
	//The topic name
	Name string
	//The current state of the topic
	State    TopicState
	Previous EventOut
	First    EventOut
	Last     EventOut
}

//EventIn is the input to PostEvent, and should contain information about the
// event to post to SHOUT!
type EventIn struct {
	//The topic name
	Topic string
	//A message about the event
	Message string
	//A URL relevent to the event
	Link string
	//The time that the event occurred
	OccurredAt time.Time
	//True if the event represents a "working" state. False if "broken"
	OK bool
}

//EventOut is an event construct contained in the response of PostEvent calls
type EventOut struct {
	//When the event was reported to have occurred
	OccurredAt time.Time
	//When the event report was recieved
	ReportedAt time.Time
	//The message given for the event
	Message string
	//The link given for the event
	Link string
	//Whether the event was reported as OK
	OK bool
}

func (c *Client) doRequest(method, path string, body []byte) (*http.Response, error) {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequest(method,
		fmt.Sprintf("%s%s", c.Target, path),
		bytes.NewReader(body),
	)

	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("SHOUT! returned non-2xx status code: %s", resp.Status)
	}

	return resp, nil
}

type eventRaw struct {
	OccurredAt int64  `json:"occurred_at"`
	ReportedAt int64  `json:"reported_at"`
	OK         bool   `json:"ok"`
	Message    string `json:"message"`
	Link       string `json:"link"`
}

type stateRaw struct {
	Name     string   `json:"name"`
	State    string   `json:"state"`
	Previous eventRaw `json:"previous"`
	First    eventRaw `json:"first"`
	Last     eventRaw `json:"last"`
}

//PostEvent sends the given event to SHOUT! to update the state of the topic.
// The event will send a message to notification backends configured by the
// rules of the SHOUT! backend if the state has changed
func (c *Client) PostEvent(e EventIn) (*StateOut, error) {
	jsonStruct := struct {
		Topic      string `json:"topic"`
		Message    string `json:"message"`
		Link       string `json:"link"`
		OccurredAt int64  `json:"occurred_at"`
		OK         bool   `json:"ok"`
	}{
		Topic:      e.Topic,
		OK:         e.OK,
		Message:    e.Message,
		Link:       e.Link,
		OccurredAt: e.OccurredAt.Unix(),
	}

	jBytes, _ := json.Marshal(&jsonStruct)

	resp, err := c.doRequest("POST", "/events", jBytes)
	if err != nil {
		return nil, err
	}

	jDec := json.NewDecoder(resp.Body)
	jsonAsStruct := stateRaw{}
	err = jDec.Decode(&jsonAsStruct)
	if err != nil {
		return nil, fmt.Errorf("Could not parse response as JSON: %s", err.Error())
	}

	return &StateOut{
		Name:     jsonAsStruct.Name,
		State:    parseState(jsonAsStruct.State),
		Previous: parseEvent(jsonAsStruct.Previous),
		First:    parseEvent(jsonAsStruct.First),
		Last:     parseEvent(jsonAsStruct.Last),
	}, nil
}

func parseState(state string) TopicState {
	var ret TopicState
	switch state {
	case "working":
		ret = TopicWorking
	case "fixed":
		ret = TopicFixed
	case "broken":
		ret = TopicBroken
	}
	return ret
}

func parseEvent(event eventRaw) EventOut {
	return EventOut{
		Message:    event.Message,
		Link:       event.Link,
		OK:         event.OK,
		OccurredAt: time.Unix(event.OccurredAt, 0),
		ReportedAt: time.Unix(event.ReportedAt, 0),
	}
}

//AnnouncementIn is the input to PostAnnouncement, containing information about
// the announcement event to send
type AnnouncementIn struct {
	//The name of the topic
	Topic string `json:"topic"`
	//The message to announce
	Message string `json:"message"`
	//A URL relevant to the announcement
	Link string `json:"link"`
}

//PostAnnouncement sends a message that goes to notification backends configured
// by the rules of the SHOUT! backend. This has no concept of a "working" or
// "broken" state, and so the message is always sent.
func (c *Client) PostAnnouncement(announcement AnnouncementIn) error {
	jBytes, _ := json.Marshal(&announcement)
	_, err := c.doRequest("POST", "/events", jBytes)
	return err
}
