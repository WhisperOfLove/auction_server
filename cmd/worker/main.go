package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"auction_server/internal/config"
	"auction_server/internal/events"
)

// Worker consumes Kafka events for notifications and heavy async work.
func main() {
	cfg := config.Load()
	reader := events.NewBidConsumer(cfg.KafkaBrokers)
	if reader == nil {
		log.Println("worker: set KAFKA_BROKERS to enable (exiting)")
		return
	}
	defer reader.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Println("worker: listening on auction.bid.placed")
	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("worker read: %v", err)
			continue
		}
		log.Printf("worker event: %s", string(msg.Value))
		// TODO: push FCM, SMS, email, analytics
	}
}
