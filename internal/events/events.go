package events

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

const (
	topicBidPlaced         = "auction.bid.placed"
	topicAuctionSubmitted  = "auction.submitted"
	topicModerationDecided = "auction.moderation.decided"
)

var (
	writersMu sync.Mutex
	writers   = map[string]*kafka.Writer{}
)

func InitKafka(brokers string) {
	brokers = strings.TrimSpace(brokers)
	if brokers == "" {
		log.Println("kafka: disabled (KAFKA_BROKERS empty)")
		return
	}
	for _, topic := range []string{topicBidPlaced, topicAuctionSubmitted, topicModerationDecided} {
		getWriter(brokers, topic)
	}
	log.Printf("kafka: producer ready (%s)", brokers)
}

func getWriter(brokers, topic string) *kafka.Writer {
	writersMu.Lock()
	defer writersMu.Unlock()
	if w, ok := writers[topic]; ok {
		return w
	}
	w := &kafka.Writer{
		Addr:         kafka.TCP(strings.Split(brokers, ",")...),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
		Async:        true,
	}
	writers[topic] = w
	return w
}

func Close() {
	writersMu.Lock()
	defer writersMu.Unlock()
	for _, w := range writers {
		_ = w.Close()
	}
	writers = map[string]*kafka.Writer{}
}

type BidPlacedEvent struct {
	AuctionID string `json:"auctionId"`
	UserID    string `json:"userId"`
	Price     int64  `json:"price"`
	AtMs      int64  `json:"atMs"`
}

type AuctionSubmittedEvent struct {
	AuctionID string `json:"auctionId"`
	OwnerID   string `json:"ownerId"`
	Title     string `json:"title"`
	AtMs      int64  `json:"atMs"`
}

type ModerationDecidedEvent struct {
	AuctionID string `json:"auctionId"`
	OwnerID   string `json:"ownerId"`
	Decision  string `json:"decision"`
	Reason    string `json:"reason,omitempty"`
	AtMs      int64  `json:"atMs"`
}

func publish(topic string, payload any) {
	writersMu.Lock()
	w := writers[topic]
	writersMu.Unlock()
	if w == nil {
		return
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := w.WriteMessages(ctx, kafka.Message{Value: raw}); err != nil {
		log.Printf("kafka publish %s: %v", topic, err)
	}
}

func PublishBidPlaced(ev BidPlacedEvent) {
	if ev.AtMs == 0 {
		ev.AtMs = time.Now().UnixMilli()
	}
	publish(topicBidPlaced, ev)
}

func PublishAuctionSubmitted(ev AuctionSubmittedEvent) {
	if ev.AtMs == 0 {
		ev.AtMs = time.Now().UnixMilli()
	}
	publish(topicAuctionSubmitted, ev)
}

func PublishModerationDecided(ev ModerationDecidedEvent) {
	if ev.AtMs == 0 {
		ev.AtMs = time.Now().UnixMilli()
	}
	publish(topicModerationDecided, ev)
}

func NewBidConsumer(brokers string) *kafka.Reader {
	return newConsumer(brokers, topicBidPlaced, "auction-worker-bids")
}

func NewModerationConsumer(brokers string) *kafka.Reader {
	return newConsumer(brokers, topicModerationDecided, "auction-worker-moderation")
}

func newConsumer(brokers, topic, groupID string) *kafka.Reader {
	brokers = strings.TrimSpace(brokers)
	if brokers == "" {
		return nil
	}
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers: strings.Split(brokers, ","),
		Topic:   topic,
		GroupID: groupID,
	})
}
