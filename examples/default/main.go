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

	// Create a channel to receive mail updates by subscribing to the client
	ch := make(chan *mailstream.Mail)
	client.Subscribe(ch)

	// Tell the client to start listening for mail updates
	done := client.WaitForUpdates(context.Background())

	// We run forever, printing mail updates as they come
	for {
		select {
		case mail := <-ch:
			fmt.Println(mail.Subject)
		case err := <-done:
			log.Fatal(err)
		}
	}
}
