package graph

import (
	"context"
	"fmt"
	"time"
)

// Message represents an Outlook email message
type Message struct {
	ID                string      `json:"id"`
	Subject           string      `json:"subject"`
	BodyPreview       string      `json:"bodyPreview"`
	Body              *ItemBody   `json:"body,omitempty"`
	From              *Recipient  `json:"from,omitempty"`
	ToRecipients      []Recipient `json:"toRecipients,omitempty"`
	CcRecipients      []Recipient `json:"ccRecipients,omitempty"`
	BccRecipients     []Recipient `json:"bccRecipients,omitempty"`
	ReceivedDateTime  time.Time   `json:"receivedDateTime"`
	SentDateTime      time.Time   `json:"sentDateTime,omitempty"`
	HasAttachments    bool        `json:"hasAttachments"`
	Importance        string      `json:"importance"`
	IsRead            bool        `json:"isRead"`
	IsDraft           bool        `json:"isDraft"`
	ConversationID    string      `json:"conversationId,omitempty"`
	ParentFolderID    string      `json:"parentFolderId,omitempty"`
	WebLink           string      `json:"webLink,omitempty"`
	InternetMessageID string      `json:"internetMessageId,omitempty"`
}

// ItemBody represents message body content
type ItemBody struct {
	ContentType string `json:"contentType"` // "text" or "html"
	Content     string `json:"content"`
}

// Recipient represents an email recipient
type Recipient struct {
	EmailAddress EmailAddress `json:"emailAddress"`
}

// EmailAddress represents an email address
type EmailAddress struct {
	Name    string `json:"name,omitempty"`
	Address string `json:"address"`
}

// MailFolder represents a mail folder
type MailFolder struct {
	ID               string `json:"id"`
	DisplayName      string `json:"displayName"`
	ParentFolderID   string `json:"parentFolderId,omitempty"`
	ChildFolderCount int    `json:"childFolderCount"`
	UnreadItemCount  int    `json:"unreadItemCount"`
	TotalItemCount   int    `json:"totalItemCount"`
}

// SendMailRequest is the request body for sending mail
type SendMailRequest struct {
	Message         Message `json:"message"`
	SaveToSentItems bool    `json:"saveToSentItems"`
}

// ListMessages lists messages in a folder
func (c *Client) ListMessages(ctx context.Context, folder string, params *QueryParams) (*ListResponse[Message], error) {
	if folder == "" {
		folder = "inbox"
	}

	path := fmt.Sprintf("/me/mailFolders/%s/messages", folder)
	if params != nil {
		path += params.ToQuery()
	}

	var result ListResponse[Message]
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetMessage retrieves a single message by ID
func (c *Client) GetMessage(ctx context.Context, messageID string) (*Message, error) {
	path := fmt.Sprintf("/me/messages/%s", messageID)

	var result Message
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// SendMail sends an email message
func (c *Client) SendMail(ctx context.Context, to []string, cc []string, subject, body string, isHTML bool) error {
	contentType := "text"
	if isHTML {
		contentType = "html"
	}

	toRecipients := make([]Recipient, len(to))
	for i, addr := range to {
		toRecipients[i] = Recipient{EmailAddress: EmailAddress{Address: addr}}
	}

	ccRecipients := make([]Recipient, len(cc))
	for i, addr := range cc {
		ccRecipients[i] = Recipient{EmailAddress: EmailAddress{Address: addr}}
	}

	req := SendMailRequest{
		Message: Message{
			Subject:      subject,
			Body:         &ItemBody{ContentType: contentType, Content: body},
			ToRecipients: toRecipients,
			CcRecipients: ccRecipients,
		},
		SaveToSentItems: true,
	}

	return c.Post(ctx, "/me/sendMail", req, nil)
}

// replyToMessage is the shared helper for reply/replyAll.
func (c *Client) replyToMessage(ctx context.Context, messageID, action, comment string) error {
	path := fmt.Sprintf("/me/messages/%s/%s", messageID, action)
	return c.Post(ctx, path, map[string]string{"comment": comment}, nil)
}

// ReplyToMessage sends a reply to a message
func (c *Client) ReplyToMessage(ctx context.Context, messageID, comment string) error {
	return c.replyToMessage(ctx, messageID, "reply", comment)
}

// ReplyAllToMessage sends a reply-all to a message
func (c *Client) ReplyAllToMessage(ctx context.Context, messageID, comment string) error {
	return c.replyToMessage(ctx, messageID, "replyAll", comment)
}

// DeleteMessage deletes a message
func (c *Client) DeleteMessage(ctx context.Context, messageID string) error {
	path := fmt.Sprintf("/me/messages/%s", messageID)
	return c.Delete(ctx, path)
}

// MoveMessage moves a message to a different folder
func (c *Client) MoveMessage(ctx context.Context, messageID, destinationFolderID string) (*Message, error) {
	path := fmt.Sprintf("/me/messages/%s/move", messageID)
	body := map[string]string{"destinationId": destinationFolderID}

	var result Message
	if err := c.Post(ctx, path, body, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ListMailFolders lists mail folders
func (c *Client) ListMailFolders(ctx context.Context) (*ListResponse[MailFolder], error) {
	var result ListResponse[MailFolder]
	if err := c.Get(ctx, "/me/mailFolders", &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// SearchMessages searches messages using KQL
func (c *Client) SearchMessages(ctx context.Context, query string, top int) (*ListResponse[Message], error) {
	params := &QueryParams{
		Search: query,
		Top:    top,
	}

	path := "/me/messages" + params.ToQuery()

	var result ListResponse[Message]
	if err := c.Get(ctx, path, &result); err != nil {
		return nil, err
	}

	return &result, nil
}
