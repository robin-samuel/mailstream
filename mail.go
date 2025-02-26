package mailstream

import (
	"bytes"
	"errors"
	"io"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/mail"
)

type Mail struct {
	UID     uint32
	From    []imap.Address
	To      []imap.Address
	Subject string
	Date    time.Time

	Plain []byte
	HTML  []byte
}

func buildMail(message *imapclient.FetchMessageBuffer) (*Mail, error) {
	var body []byte
	for _, part := range message.BodySection {
		if part.Section.Specifier == imap.PartSpecifierNone {
			body = part.Bytes
			break
		}
	}
	mr, err := mail.CreateReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	m := &Mail{
		UID:     uint32(message.UID),
		From:    message.Envelope.From,
		To:      message.Envelope.To,
		Subject: message.Envelope.Subject,
		Date:    message.Envelope.Date,
	}

	for {
		p, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch h := p.Header.(type) {
		case *mail.InlineHeader:
			contentType, _, _ := h.ContentType()
			switch contentType {
			case "text/plain":
				m.Plain, _ = io.ReadAll(p.Body)
			case "text/html":
				m.HTML, _ = io.ReadAll(p.Body)
			}
		case *mail.AttachmentHeader:
		}
	}
	return m, nil
}
