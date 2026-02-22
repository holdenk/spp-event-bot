// Package matrix wraps the mautrix-go client for posting event messages to Matrix rooms.
package matrix

import (
	"context"
	"fmt"
	"html"
	"strings"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"

	"github.com/sparklingpinkpandas/spp-event-bot/feed"
)

// Client wraps a mautrix client for event posting.
type Client struct {
	mxClient *mautrix.Client
}

// NewClient creates a new Matrix client connected to the given homeserver.
// The userID can be empty; the bot will resolve its own identity if needed.
func NewClient(homeserver, accessToken string) (*Client, error) {
	client, err := mautrix.NewClient(homeserver, id.UserID(""), accessToken)
	if err != nil {
		return nil, fmt.Errorf("creating matrix client: %w", err)
	}

	return &Client{mxClient: client}, nil
}

// PostEvent formats a feed event as an HTML message and sends it to the given Matrix room.
func (c *Client) PostEvent(ctx context.Context, roomID string, ev feed.Event) error {
	plain, formatted := formatEvent(ev)

	content := &event.MessageEventContent{
		MsgType:       event.MsgText,
		Body:          plain,
		Format:        event.FormatHTML,
		FormattedBody: formatted,
	}

	_, err := c.mxClient.SendMessageEvent(ctx, id.RoomID(roomID), event.EventMessage, content)
	if err != nil {
		return fmt.Errorf("sending message to %s: %w", roomID, err)
	}

	return nil
}

// formatEvent builds plain-text and HTML representations of a feed event.
func formatEvent(ev feed.Event) (plain, htmlBody string) {
	var plainBuf, htmlBuf strings.Builder

	// Title
	if ev.Link != "" {
		htmlBuf.WriteString(fmt.Sprintf("<b><a href=\"%s\">%s</a></b>", html.EscapeString(ev.Link), html.EscapeString(ev.Title)))
		plainBuf.WriteString(fmt.Sprintf("%s (%s)", ev.Title, ev.Link))
	} else {
		htmlBuf.WriteString(fmt.Sprintf("<b>%s</b>", html.EscapeString(ev.Title)))
		plainBuf.WriteString(ev.Title)
	}

	// Date
	if !ev.Date.IsZero() {
		dateStr := ev.Date.Format("Mon, 02 Jan 2006 15:04 MST")
		htmlBuf.WriteString(fmt.Sprintf("<br/><b>Date:</b> %s", html.EscapeString(dateStr)))
		plainBuf.WriteString(fmt.Sprintf("\nDate: %s", dateStr))
	}

	// Location
	if ev.Location != "" {
		htmlBuf.WriteString(fmt.Sprintf("<br/><b>Location:</b> %s", html.EscapeString(ev.Location)))
		plainBuf.WriteString(fmt.Sprintf("\nLocation: %s", ev.Location))
	}

	// Description snippet (truncate to 500 chars for readability)
	if ev.Description != "" {
		desc := ev.Description
		if len(desc) > 500 {
			desc = desc[:497] + "..."
		}
		htmlBuf.WriteString(fmt.Sprintf("<br/><br/>%s", html.EscapeString(desc)))
		plainBuf.WriteString(fmt.Sprintf("\n\n%s", desc))
	}

	return plainBuf.String(), htmlBuf.String()
}
