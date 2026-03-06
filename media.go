package main

import (
	"fmt"

	"github.com/slidebolt/sdk-types"
)

const (
	domainMediaCast = "media.cast"
	actionPlayURL   = "play_url"
	actionStop      = "stop"
)

type mediaCastCommand struct {
	Type        string `json:"type"`
	URL         string `json:"url,omitempty"`
	ContentType string `json:"content_type,omitempty"`
}

func (c mediaCastCommand) Validate() error {
	switch c.Type {
	case actionPlayURL:
		if c.URL == "" {
			return fmt.Errorf("play_url requires url")
		}
	case actionStop:
		return nil
	default:
		return fmt.Errorf("unsupported media.cast command: %s", c.Type)
	}
	return nil
}

func mediaCastDomainDescriptor() types.DomainDescriptor {
	return types.DomainDescriptor{
		Domain: domainMediaCast,
		Commands: []types.ActionDescriptor{
			{
				Action: actionPlayURL,
				Fields: []types.FieldDescriptor{
					{Name: "url", Type: "string", Required: true},
					{Name: "content_type", Type: "string", Required: false},
				},
			},
			{
				Action: actionStop,
			},
		},
		Events: []types.ActionDescriptor{
			{
				Action: "media_ack",
				Fields: []types.FieldDescriptor{
					{Name: "action", Type: "string", Required: true},
					{Name: "url", Type: "string", Required: false},
				},
			},
		},
	}
}
