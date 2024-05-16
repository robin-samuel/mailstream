package mailstream

import (
	"context"
	"mime"
	"strconv"
	"sync"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"
)

type Client struct {
	client         *imapclient.Client
	expungeHandler chan uint32
	mailboxHandler chan imapclient.UnilateralDataMailbox
	fetchHandler   chan imapclient.FetchMessageData

	lock        sync.Mutex
	subscribers []chan *Mail
	numMessages uint32
}

type Config struct {
	Host     string
	Port     int
	Email    string
	Password string
	Mailbox  string
}

func New(config Config) (*Client, error) {
	c := &Client{}

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
	return c, nil
}

func (c *Client) Close() error {
	return c.client.Close()
}

func (c *Client) Subscribe(ch chan *Mail) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.subscribers = append(c.subscribers, ch)
}

func (c *Client) Unsubscribe(ch chan *Mail) {
	c.lock.Lock()
	defer c.lock.Unlock()
	for i, subscribers := range c.subscribers {
		if subscribers == ch {
			c.subscribers = append(c.subscribers[:i], c.subscribers[i+1:]...)
			break
		}
	}
}

func (c *Client) GetUnseenMails() <-chan error {
	done := make(chan error)

	go func() {
		criteria := &imap.SearchCriteria{
			NotFlag: []imap.Flag{imap.FlagSeen},
		}
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
	fetchCmd := c.client.Fetch(seq, &imap.FetchOptions{
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
		c.lock.Lock()
		for _, ch := range c.subscribers {
			ch <- mail
		}
		c.lock.Unlock()
	}

	return nil
}
