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

	// A concurrent task that listens for new mails.
	go func() {
		defer wg.Done()
		// Create a channel to receive mail updates by subscribing to the client.
		ch := make(chan *mailstream.Mail)
		client.Subscribe(ch)
		defer client.Unsubscribe(ch)
		// Wait for exactly one mail to be received.
		mail := <-ch
		fmt.Printf("Task 1 - Received mail: %s\n", mail.Subject)
	}()

	// Another concurrent task that listens for new mails.
	go func() {
		defer wg.Done()
		// Create a channel to receive mail updates by subscribing to the client.
		ch := make(chan *mailstream.Mail)
		client.Subscribe(ch)
		defer client.Unsubscribe(ch)
		// Wait for exactly one mail to be received.
		mail := <-ch
		fmt.Printf("Task 2 - Received mail: %s\n", mail.Subject)
	}()

	// Tell the client to fetch unseen mails. This will trigger the tasks above,
	// if there are any unseen mails. If there are no unseen mails, the tasks
	// will continue to listen for new mails.
	<-client.GetUnseenMails() // We can ignore/remove this if we don't care about mails in the inbox right now.

	// Tell the client to wait for updates. We create a context with a cancel
	// function so that we can stop waiting for updates after task 1 and task 2
	// have received a mail.
	ctx, cancel := context.WithCancel(context.Background())
	client.WaitForUpdates(ctx)

	// Wait for task 1 and task 2 to finish.
	wg.Wait()

	// Stop waiting for updates.
	cancel()
}
