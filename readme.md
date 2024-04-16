# MailStream

MailStream is a Go library that provides an efficient interface to interact with IMAP servers. It enables the streaming of mail updates in real-time. This library is a wrapper around the [github.com/emersion/go-imap/v2](https://github.com/emersion/go-imap) library, which provides the IMAP client implementation.

## Features

- Connect to IMAP servers using secure protocols.
- Subscribe to real-time mail updates.
- Efficient handling of concurrent mail streams.
- Fetching Mails using IMAP IDLE. - No steady polling.
- Mails parsed into structured objects seperating the html and text parts.

## Installation

To install MailStream, use the `go get` command:

```bash
go get github.com/robin-samuel/mailstream
```

This will retrieve the library from GitHub and install it in your Go workspace.

## Usage

Below are two basic examples of how to use the MailStream library: one for default usage and another for concurrent processing. Please look at the examples directory for more detailed explanations.

### Default Usage

The default example demonstrates how to set up a simple mail listening service that logs all incoming mails.

**File: `examples/default/main.go`**

```go
package main

import (
    "context"
    "fmt"
    "log"
    "github.com/robin-samuel/mailstream"
)

func main() {
    config := mailstream.Config{
        Host:     "imap.example.com",
        Port:     993,
        Email:    "mymail@example.com",
        Password: "password1234",
    }
    client, err := mailstream.New(config)
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    ch := make(chan *mailstream.Mail)
    client.Subscribe(ch)
    done := client.WaitForUpdates(context.Background())

    for {
        select {
        case mail := <-ch:
            fmt.Println(mail.Subject)
        case err := <-done:
            log.Fatal(err)
        }
    }
}
```

### Concurrent Usage

The concurrent example demonstrates how to handle multiple subscribers that listen for new mails concurrently.

**File: `examples/concurrent/main.go`**

```go
package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/robin-samuel/mailstream"
)

func main() {
	config := mailstream.Config{
		Host:     "imap.example.com",
		Port:     993,
		Email:    "mymail@example.com",
		Password: "password1234",
	}
	client, err := mailstream.New(config)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		ch := make(chan *mailstream.Mail)
		client.Subscribe(ch)
		defer client.Unsubscribe(ch)
		mail := <-ch
		fmt.Printf("Task 1 - Received mail: %s\n", mail.Subject)
	}()

	go func() {
		defer wg.Done()
		ch := make(chan *mailstream.Mail)
		client.Subscribe(ch)
		defer client.Unsubscribe(ch)
		mail := <-ch
		fmt.Printf("Task 2 - Received mail: %s\n", mail.Subject)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	client.WaitForUpdates(ctx)

	wg.Wait()
	cancel()
}

```
