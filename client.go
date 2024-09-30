package mailstream

import (
	"context"
	"mime"
	"strconv"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"
)

type Client struct {
	client         *imapclient.Client
	expungeHandler chan uint32
	mailboxHandler chan imapclient.UnilateralDataMailbox
	fetchHandler   chan imapclient.FetchMessageData

	listener       []chan *Mail
	broadcast      chan *Mail
	addListener    chan chan *Mail
	removeListener chan (<-chan *Mail)
	numMessages    uint32
}

type Config struct {
	Host     string
	Port     int
	Email    string
	Password string
	Mailbox  string
}

func New(config Config) (*Client, error) {
	c := &Client{
		broadcast:      make(chan *Mail),
		addListener:    make(chan chan *Mail),
		removeListener: make(chan (<-chan *Mail)),
	}

	if config.Mailbox == "" {
		config.Mailbox = "INBOX"
	}

	address := config.Host + ":" + strconv.Itoa(config.Port)
	options := &imapclient.Options{
		WordDecoder: &mime.WordDecoder{CharsetReader: charset.Reader},
		UnilateralDataHandler: &imapclient.UnilateralDataHandler{
			Expunge: func(seqNum uint32) {
				if c.expungeHandler != nil {
					c.expungeHandler <- seqNum
				}
			},
			Mailbox: func(data *imapclient.UnilateralDataMailbox) {
				if c.mailboxHandler != nil {
					c.mailboxHandler <- *data
				}
			},
			Fetch: func(msg *imapclient.FetchMessageData) {
				if c.fetchHandler != nil {
					c.fetchHandler <- *msg
				}
			},
		},
	}

	var client *imapclient.Client
	var err error
	// Check if the server supports implicit TLS
	if client, err = imapclient.DialTLS(address, options); err != nil {
		// Check if the server supports STARTTLS
		if client, err = imapclient.DialStartTLS(address, options); err != nil {
			return nil, err
		}
	}

	// Login with the given username and password
	if err := client.Login(config.Email, config.Password).Wait(); err != nil {
		return nil, err
	}

	// Select the INBOX
	if mb, err := client.Select(config.Mailbox, nil).Wait(); err != nil {
		return nil, err
	} else {
		c.numMessages = mb.NumMessages
	}

	c.client = client
	go c.serve()
	return c, nil
}

func (c *Client) Close() error {
	return c.client.Close()
}

func (c *Client) serve() {
	defer func() {
		for _, listener := range c.listener {
			close(listener)
		}
	}()
	for {
		select {
		case listener := <-c.addListener:
			c.listener = append(c.listener, listener)
		case listener := <-c.removeListener:
			for i, l := range c.listener {
				if l == listener {
					c.listener = append(c.listener[:i], c.listener[i+1:]...)
					break
				}
			}
		case mail, ok := <-c.broadcast:
			if !ok {
				return
			}
			for _, listener := range c.listener {
				if listener != nil {
					listener <- mail
				}
			}
		}
	}
}

func (c *Client) Subscribe() <-chan *Mail {
	listener := make(chan *Mail, 10)
	c.addListener <- listener
	return listener
}

func (c *Client) Unsubscribe(ch <-chan *Mail) {
	c.removeListener <- ch
}

func (c *Client) GetUnseenMails() <-chan error {
	criteria := &imap.SearchCriteria{
		NotFlag: []imap.Flag{imap.FlagSeen},
	}
	return c.Search(criteria)
}

func (c *Client) Search(criteria *imap.SearchCriteria) <-chan error {
	done := make(chan error)

	go func() {
		data, err := c.client.Search(criteria, nil).Wait()
		if err != nil {
			done <- err
			return
		}

		var seq imap.SeqSet
		seq.AddNum(data.AllSeqNums()...)

		if seq != nil {
			if err := c.fetch(seq); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()

	return done
}

func (c *Client) WaitForUpdates(ctx context.Context) <-chan error {
	done := make(chan error)

	go func() {
		defer close(done)

		if c.mailboxHandler == nil {
			c.mailboxHandler = make(chan imapclient.UnilateralDataMailbox, 1000)
			defer close(c.mailboxHandler)
		}

		for {
			idle, err := c.client.Idle()
			if err != nil {
				done <- err
				return
			}

			select {
			case <-ctx.Done():
				return
			case md := <-c.mailboxHandler:
				if md.NumMessages == nil {
					continue
				}
				if err := idle.Close(); err != nil {
					done <- err
					return
				}

				if err := idle.Wait(); err != nil {
					done <- err
					return
				}

				if c.numMessages < *md.NumMessages {
					var seq imap.SeqSet
					seq.AddRange(c.numMessages+1, *md.NumMessages)
					if err := c.fetch(seq); err != nil {
						done <- err
						return
					}
				}
				c.numMessages = *md.NumMessages
			}
		}
	}()

	return done
}

func (c *Client) fetch(seq imap.SeqSet) error {
	nums, _ := seq.Nums()
	for _, num := range nums {
		var subSeq imap.SeqSet
		subSeq.AddNum(num)

		fetchCmd := c.client.Fetch(subSeq, &imap.FetchOptions{
			BodyStructure: &imap.FetchItemBodyStructure{Extended: false},
			Envelope:      true,
			Flags:         true,
			InternalDate:  true,
			RFC822Size:    true,
			UID:           true,
			BodySection:   []*imap.FetchItemBodySection{{}},
		})

		messages, err := fetchCmd.Collect()
		if err != nil {
			return err
		}

		for _, message := range messages {
			mail, err := buildMail(message)
			if err != nil {
				return err
			}
			c.broadcast <- mail
		}
	}

	return nil
}
